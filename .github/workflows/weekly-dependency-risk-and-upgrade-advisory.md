---
description: |
  Weekly hybrid dependency advisory for Go (server/agent) and Vue frontend.
  Phase A deterministically collects dependency facts and uploads artifacts.
  Phase B performs agentic ranking and updates a single tracking issue.
on:
  schedule: weekly
  workflow_dispatch:
    inputs:
      suggest_only:
        description: "When true, produce suggestions only (no update PRs)."
        required: false
        type: boolean
        default: true
      include_trend_delta:
        description: "When true, compare this run against previous advisory summary."
        required: false
        type: boolean
        default: true
      max_recommendations:
        description: "Maximum ranked recommendations to include."
        required: false
        type: number
        default: 10
  pull_request:
    paths:
      - "**/go.mod"
      - "**/go.sum"
      - "frontend/package.json"
      - "frontend/package-lock.json"
      - "frontend/npm-shrinkwrap.json"
      - "frontend/pnpm-lock.yaml"
      - "frontend/yarn.lock"

permissions:
  contents: read
  issues: read
  pull-requests: read
  actions: read

network:
  allowed: [defaults, go, node]

runtimes:
  go:
    version: "1.26.2"

tools:
  github:
    toolsets: [default]
    lockdown: false
    min-integrity: none
  cache-memory: true

steps:
  - name: Prepare report directories and schema
    run: |
      set -euo pipefail
      mkdir -p reports/raw reports/readable reports/previous tmp/workflow-tools
      cat > reports/raw/schema-version.json <<'JSON'
      {
        "schema_version": "1.0.0",
        "workflow": "weekly-dependency-risk-and-upgrade-advisory",
        "generated_at": "${{ github.run_id }}"
      }
      JSON
      : > reports/raw/.index.ndjson

  - name: Seed cache-memory sentinel
    run: |
      set -euo pipefail
      mkdir -p /tmp/gh-aw/cache-memory
      if [ ! -f /tmp/gh-aw/cache-memory/cache-memory-sentinel.json ]; then
        cat > /tmp/gh-aw/cache-memory/cache-memory-sentinel.json <<'JSON'
      {
        "purpose": "Ensure cache-memory artifact upload has a visible file even before the repo accumulates state."
      }
      JSON
      fi

  - name: Set up buf
    uses: bufbuild/buf-action@v1.4.0
    with:
      setup_only: true

  - name: Install protoc plugins
    run: |
      go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
      go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.1

  - name: Generate proto
    run: cd proto && buf generate

  - name: Collect deterministic dependency facts
    run: |
      set +e

      ROOT_DIR="$(pwd)"
      INDEX_FILE="${ROOT_DIR}/reports/raw/.index.ndjson"

      mark_ok() {
        file="$1"
        kind="$2"
        ecosystem="$3"
        printf '{"file":"%s","status":"ok","kind":"%s","ecosystem":"%s"}\n' "$file" "$kind" "$ecosystem" >> "$INDEX_FILE"
      }

      mark_missing() {
        file="$1"
        kind="$2"
        ecosystem="$3"
        reason="$4"
        printf '{"file":"%s","status":"missing","kind":"%s","ecosystem":"%s","reason":%s}\n' "$file" "$kind" "$ecosystem" "$(printf '%s' "$reason" | jq -Rs .)" >> "$INDEX_FILE"
      }

      run_json() {
        cmd="$1"
        out="$2"
        kind="$3"
        ecosystem="$4"
        eval "$cmd" > "$out" 2>"${ROOT_DIR}/tmp/workflow-tools/stderr.txt"
        rc=$?
        if [ $rc -eq 0 ]; then
          mark_ok "$out" "$kind" "$ecosystem"
        else
          cat > "$out" <<JSON
      {
        "error": "command_failed",
        "exit_code": $rc,
        "command": $(printf '%s' "$cmd" | jq -Rs .),
        "stderr": $(cat "${ROOT_DIR}/tmp/workflow-tools/stderr.txt" | jq -Rs .)
      }
      JSON
          mark_missing "$out" "$kind" "$ecosystem" "$(cat "${ROOT_DIR}/tmp/workflow-tools/stderr.txt")"
        fi
      }

      run_json_allow_rc() {
        cmd="$1"
        out="$2"
        kind="$3"
        ecosystem="$4"
        allowed_rc_csv="$5"
        eval "$cmd" > "$out" 2>"${ROOT_DIR}/tmp/workflow-tools/stderr.txt"
        rc=$?
        case ",${allowed_rc_csv}," in
          *",${rc},"*) allowed=1 ;;
          *) allowed=0 ;;
        esac
        if [ $rc -eq 0 ] || [ "$allowed" -eq 1 ]; then
          mark_ok "$out" "$kind" "$ecosystem"
        else
          cat > "$out" <<JSON
      {
        "error": "command_failed",
        "exit_code": $rc,
        "command": $(printf '%s' "$cmd" | jq -Rs .),
        "stderr": $(cat "${ROOT_DIR}/tmp/workflow-tools/stderr.txt" | jq -Rs .)
      }
      JSON
          mark_missing "$out" "$kind" "$ecosystem" "$(cat "${ROOT_DIR}/tmp/workflow-tools/stderr.txt")"
        fi
      }

      run_text() {
        cmd="$1"
        out="$2"
        kind="$3"
        ecosystem="$4"
        eval "$cmd" > "$out" 2>"${ROOT_DIR}/tmp/workflow-tools/stderr.txt"
        rc=$?
        if [ $rc -eq 0 ]; then
          mark_ok "$out" "$kind" "$ecosystem"
        else
          cat > "$out" <<TXT
      # command_failed
      # exit_code: $rc
      # command: $cmd
      # stderr:
      $(cat "${ROOT_DIR}/tmp/workflow-tools/stderr.txt")
      TXT
          mark_missing "$out" "$kind" "$ecosystem" "$(cat tmp/workflow-tools/stderr.txt)"
        fi
      }

      run_mod_why_json() {
        module_dir="$1"
        out="$2"
        ecosystem="$3"
        kind="$4"
        pushd "$module_dir" >/dev/null || return 1
        mods=$(go list -m all 2>/dev/null)
        if [ $? -ne 0 ]; then
          popd >/dev/null || true
          cat > "$out" <<'JSON'
      {"error":"go_list_failed"}
      JSON
          mark_missing "$out" "$kind" "$ecosystem" "go list -m all failed"
          return 0
        fi

        {
          echo '['
          first=1
          printf '%s\n' "$mods" | tail -n +2 | while IFS= read -r mod; do
            why_out=$(go mod why -m "$mod" 2>&1)
            if [ $first -eq 0 ]; then
              echo ','
            fi
            first=0
            printf '{"module":%s,"why":%s}' "$(printf '%s' "$mod" | jq -Rs .)" "$(printf '%s' "$why_out" | jq -Rs .)"
          done
          echo
          echo ']'
        } > "$out"

        popd >/dev/null || true
        mark_ok "$out" "$kind" "$ecosystem"
      }

      run_go_license_report() {
        module_dir="$1"
        out="$2"
        ecosystem="$3"
        kind="$4"

        cat > tmp/workflow-tools/go-licenses-template.tmpl <<'TPL'
      {{- range . -}}
      {"package":{{ printf "%q" .Name }},"license":{{ printf "%q" .LicenseName }},"url":{{ printf "%q" .LicenseURL }}}
      {{"\n" -}}
      {{- end -}}
      TPL

        pushd "$module_dir" >/dev/null || return 1
        go run github.com/google/go-licenses@latest report ./... --template ../tmp/workflow-tools/go-licenses-template.tmpl > "../$out" 2>"${ROOT_DIR}/tmp/workflow-tools/stderr.txt"
        rc=$?
        popd >/dev/null || true

        if [ $rc -eq 0 ]; then
          mark_ok "$out" "$kind" "$ecosystem"
        else
          cat > "$out" <<JSON
      {
        "error": "go_licenses_failed",
        "stderr": $(cat "${ROOT_DIR}/tmp/workflow-tools/stderr.txt" | jq -Rs .)
      }
      JSON
          mark_missing "$out" "$kind" "$ecosystem" "$(cat "${ROOT_DIR}/tmp/workflow-tools/stderr.txt")"
        fi
      }

      run_go_fact_set() {
        module_dir="$1"
        prefix="$2"

        pushd "$module_dir" >/dev/null || return 1
        run_json "go list -m -u -json all" "../reports/raw/${prefix}-modules.json" "update_surface" "go"
        run_text "go mod graph" "../reports/raw/${prefix}-mod-graph.txt" "mod_graph" "go"
        popd >/dev/null || true

        run_mod_why_json "$module_dir" "reports/raw/${prefix}-mod-why.json" "go" "mod_why"

        pushd "$module_dir" >/dev/null || return 1
        run_json_allow_rc "go run golang.org/x/vuln/cmd/govulncheck@latest -json ./..." "../reports/raw/${prefix}-vulns.json" "vulnerability" "go" "3"
        popd >/dev/null || true

        run_go_license_report "$module_dir" "reports/raw/${prefix}-licenses.json" "go" "license"
      }

      run_go_fact_set server go-server
      run_go_fact_set agent go-agent

      pushd frontend >/dev/null || exit 0
      npm ci --ignore-scripts > ../tmp/workflow-tools/frontend-npm-ci.txt 2>&1
      if [ $? -ne 0 ]; then
        mark_missing "reports/raw/npm-outdated.json" "update_surface" "npm" "npm ci failed"
        mark_missing "reports/raw/npm-audit.json" "vulnerability" "npm" "npm ci failed"
        mark_missing "reports/raw/npm-licenses.json" "license" "npm" "npm ci failed"
        mark_missing "reports/raw/frontend-unused-deps.json" "unused_deps" "npm" "npm ci failed"
        mark_missing "reports/raw/frontend-import-frequency.json" "usage_surface" "npm" "npm ci failed"
      else
        run_json_allow_rc "npm outdated --json" "../reports/raw/npm-outdated.json" "update_surface" "npm" "1"
        run_json_allow_rc "npm audit --json" "../reports/raw/npm-audit.json" "vulnerability" "npm" "1"
        run_json_allow_rc "npx --yes license-checker --json" "../reports/raw/npm-licenses.json" "license" "npm" "1"
        [ -s ../reports/raw/npm-outdated.json ] || echo '{}' > ../reports/raw/npm-outdated.json
        [ -s ../reports/raw/npm-audit.json ] || echo '{}' > ../reports/raw/npm-audit.json
        [ -s ../reports/raw/npm-licenses.json ] || echo '{}' > ../reports/raw/npm-licenses.json

        npx --yes knip --reporter json > ../reports/raw/frontend-unused-deps.json 2>"${ROOT_DIR}/tmp/workflow-tools/unused-deps.err"
        knip_rc=$?
        if [ $knip_rc -eq 0 ] || [ $knip_rc -eq 1 ]; then
          mark_ok "reports/raw/frontend-unused-deps.json" "unused_deps" "npm"
        else
          npx --yes depcheck --json > ../reports/raw/frontend-unused-deps.json 2>>"${ROOT_DIR}/tmp/workflow-tools/unused-deps.err"
          depcheck_rc=$?
          if [ $depcheck_rc -eq 0 ] || [ $depcheck_rc -eq 1 ]; then
            mark_ok "reports/raw/frontend-unused-deps.json" "unused_deps" "npm"
          else
            cat > ../reports/raw/frontend-unused-deps.json <<JSON
      {
        "error": "unused_dependency_scan_failed",
        "stderr": $(cat "${ROOT_DIR}/tmp/workflow-tools/unused-deps.err" | jq -Rs .)
      }
      JSON
            mark_missing "reports/raw/frontend-unused-deps.json" "unused_deps" "npm" "$(cat "${ROOT_DIR}/tmp/workflow-tools/unused-deps.err")"
          fi
        fi

        node <<'NODE' > ../reports/raw/frontend-import-frequency.json
      const fs = require("fs");
      const path = require("path");

      const pkg = JSON.parse(fs.readFileSync("package.json", "utf8"));
      const deps = new Set([
        ...Object.keys(pkg.dependencies || {}),
        ...Object.keys(pkg.devDependencies || {})
      ]);

      const root = path.join(process.cwd(), "src");
      const importRegex = /from\s+['"]([^'"]+)['"]|import\s+['"]([^'"]+)['"]|require\(\s*['"]([^'"]+)['"]\s*\)/g;
      const fileRegex = /\.(ts|tsx|js|jsx|vue)$/;

      const counts = new Map();
      const filesByDep = new Map();

      const toPackageName = (spec) => {
        if (!spec || spec.startsWith(".") || spec.startsWith("/")) return null;
        const parts = spec.split("/");
        if (spec.startsWith("@") && parts.length >= 2) return `${parts[0]}/${parts[1]}`;
        return parts[0];
      };

      const walk = (dir) => {
        if (!fs.existsSync(dir)) return;
        for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
          const full = path.join(dir, entry.name);
          if (entry.isDirectory()) {
            walk(full);
            continue;
          }
          if (!fileRegex.test(full)) continue;

          const rel = path.relative(process.cwd(), full).replaceAll("\\\\", "/");
          const content = fs.readFileSync(full, "utf8");
          let m;
          while ((m = importRegex.exec(content)) !== null) {
            const spec = m[1] || m[2] || m[3];
            const dep = toPackageName(spec);
            if (!dep || !deps.has(dep)) continue;
            counts.set(dep, (counts.get(dep) || 0) + 1);
            if (!filesByDep.has(dep)) filesByDep.set(dep, new Set());
            filesByDep.get(dep).add(rel);
          }
        }
      };

      walk(root);

      const rows = Array.from(deps)
        .map((dep) => {
          const files = Array.from(filesByDep.get(dep) || []).sort();
          return {
            dependency: dep,
            import_count: counts.get(dep) || 0,
            importing_file_count: files.length,
            files
          };
        })
        .sort((a, b) => b.importing_file_count - a.importing_file_count || a.dependency.localeCompare(b.dependency));

      process.stdout.write(JSON.stringify({ generated_at: new Date().toISOString(), dependencies: rows }, null, 2));
      NODE
        if [ $? -eq 0 ]; then
          mark_ok "reports/raw/frontend-import-frequency.json" "usage_surface" "npm"
        else
          cat > ../reports/raw/frontend-import-frequency.json <<'JSON'
      {"error":"import_frequency_failed"}
      JSON
          mark_missing "reports/raw/frontend-import-frequency.json" "usage_surface" "npm" "import frequency analysis failed"
        fi
      fi
      popd >/dev/null || true

      jq -s --arg run_id "${{ github.run_id }}" --arg trigger "${{ github.event_name }}" '
        {
          generated_at: now | todate,
          github_run_id: $run_id,
          trigger: $trigger,
          entries: .
        }
      ' reports/raw/.index.ndjson > reports/raw/report-index.json

      cat > reports/readable/fact-collection-summary.md <<'MD'
      ### Fact Collection Summary

      This run collected deterministic dependency evidence for Go modules and frontend npm packages.

      <details><summary>Collection status by file</summary>

      MD
      jq -r '.entries[] | "- " + .file + ": " + .status + (if .reason then " (" + .reason + ")" else "" end)' reports/raw/report-index.json >> reports/readable/fact-collection-summary.md
      cat >> reports/readable/fact-collection-summary.md <<'MD'

      </details>
      MD

      cat > reports/readable/license-risk-summary.md <<'MD'
      ### License Risk Summary

      License evidence was collected from:
      - Go modules in server and agent (`go-licenses` template JSON lines)
      - Frontend npm packages (`license-checker` JSON)

      This summary is evidence-oriented. Final compatibility decisions are made in Phase B with explicit reference to issue #124 policy context.
      MD

      cat > reports/readable/usage-asymmetry-summary.md <<'MD'
      ### Usage Asymmetry Summary

      Usage asymmetry indicators are based on:
      - Frontend import frequency per dependency
      - Unused dependency scan results (knip or depcheck fallback)

      Dependencies with high footprint and low usage are candidates for remove/replace recommendations in Phase B.
      MD

  - name: Attempt to load previous advisory summary artifact
    if: ${{ github.event_name != 'pull_request' }}
    env:
      GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}
      INCLUDE_TREND_DELTA: ${{ github.event.inputs.include_trend_delta || 'true' }}
    run: |
      set +e
      if [ "$INCLUDE_TREND_DELTA" != "true" ]; then
        exit 0
      fi

      PREV_RUN_ID=$(gh run list --workflow "Weekly Dependency Risk and Upgrade Advisory" --status success --limit 2 --json databaseId -q '.[1].databaseId')
      if [ -z "$PREV_RUN_ID" ]; then
        exit 0
      fi

      gh run download "$PREV_RUN_ID" --name dependency-facts-raw --dir reports/previous >/tmp/workflow-tools/previous-download.log 2>&1 || true
      FOUND=$(find reports/previous -type f -name advisory-summary.json | head -n 1)
      if [ -n "$FOUND" ]; then
        cp "$FOUND" reports/raw/previous-advisory-summary.json
      fi

  - name: Upload raw evidence artifact
    uses: actions/upload-artifact@v7
    with:
      name: dependency-facts-raw
      path: reports/raw
      retention-days: 90
      if-no-files-found: warn

  - name: Upload readable evidence artifact
    uses: actions/upload-artifact@v7
    with:
      name: dependency-facts-readable
      path: reports/readable
      retention-days: 90
      if-no-files-found: warn

safe-outputs:
  mentions: false
  allowed-github-references: ["#124"]
  create-issue:
    max: 1
    labels: [report, dependencies]
  update-issue:
    max: 1
    target: "*"
  add-comment:
    max: 1
---

# Weekly Dependency Risk and Upgrade Advisory

Produce a weekly dependency advisory for this monorepo based only on deterministic evidence generated under `reports/`.

## Scope and constraints

- Treat `CONTRIBUTING.md`, `docs/workflow.md`, and `docs/maintainer-guidelines.md` as policy source-of-truth.
- Give explicit attention to license-risk decisions tied to issue #124.
- Default behavior is suggest-only. Do not open update PRs and do not mutate dependencies.
- Do not fabricate missing data. If facts are missing, mark them as partial and continue.
- Read only generated files in `reports/raw` and `reports/readable` for analysis and ranking.

## Inputs

- `suggest_only`: `${{ github.event.inputs.suggest_only || 'true' }}`
- `include_trend_delta`: `${{ github.event.inputs.include_trend_delta || 'true' }}`
- `max_recommendations`: `${{ github.event.inputs.max_recommendations || 10 }}`

## Required machine outputs

Create or update these machine-readable files:

1. `reports/raw/advisory-summary.json`

Use this schema:

```json
{
  "generated_at": "ISO-8601",
  "suggest_only": true,
  "include_trend_delta": true,
  "max_recommendations": 10,
  "data_completeness": {
    "complete": false,
    "missing_sections": ["example"]
  },
  "ranking_model_weights": {
    "security_exploitability": 0,
    "license_policy_risk": 0,
    "staleness_major_lag": 0,
    "usage_asymmetry": 0,
    "operational_criticality": 0,
    "upgrade_complexity": 0
  },
  "recommendations": [
    {
      "dependency_name": "",
      "ecosystem": "go|npm",
      "current_version": "",
      "target_version_if_any": "",
      "recommendation_type": "upgrade|replace|pin|monitor|remove",
      "confidence_score": 0,
      "rationale": "",
      "priority_score": 0,
      "blast_radius": {
        "importing_file_count": 0,
        "affected_internal_packages_or_modules": [],
        "runtime_critical_path": "startup|api|scheduler|backup_execution|frontend_runtime",
        "estimated_effort": "small|medium|large",
        "regression_risk": "low|medium|high"
      },
      "issue_124_license_relevance": "yes|no"
    }
  ],
  "trend_delta": {
    "enabled": true,
    "source": "previous-artifact|none",
    "new_risks": [],
    "resolved_risks": [],
    "risk_score_movement": []
  },
  "materially_unchanged": false
}
```

## Ranking model

Use explicit weighted factors and include their numeric weights in `advisory-summary.json`:

- Security severity and exploitability
- License risk and policy incompatibility
- Staleness (major-version lag)
- Usage asymmetry (large footprint, low usage surface)
- Operational criticality (startup, API path, scheduler, backup execution, frontend runtime)
- Upgrade complexity (breaking API likelihood and touched files/packages)

## Phase B process

1. Parse `reports/raw/report-index.json` first and identify missing sections.
2. Parse vulnerability, update-surface, license, unused-dependency, and import-frequency data.
3. Build a scored recommendation list and cap to `max_recommendations`.
4. Ensure each recommendation includes all required blast radius fields.
5. If `include_trend_delta` is true and `reports/raw/previous-advisory-summary.json` exists, compute:
   - new risks
   - resolved risks
   - risk-score movement per dependency
6. Compute `materially_unchanged` by comparing this run's top recommendations and priority scores against previous summary.

## Tracking issue behavior

Maintain exactly one issue titled:

`Dependency Risk and Upgrade Advisory`

Behavior rules:

1. Search for an open issue with the exact title.
2. If found: update that issue body using `update-issue`.
3. If not found: create it using `create-issue` with that exact title.
4. Do not churn labels on update.
5. If `materially_unchanged` is true, do not add a new comment. Update only timestamp/state in the issue body.

## Advisory issue body format

Use GitHub-flavored markdown. Start section headers at `###`.

Required sections:

- `### Executive summary`
- `### Top ranked risks`
- `### License risk section (issue #124 context)`
- `### Low-usage high-footprint opportunities`
- `### Upgrade blockers`
- `### Suggested next actions`
- `### Data completeness and partial-failure notes`
- `### Artifact links`

For `### Artifact links`, include:

- `Raw evidence artifact`: `dependency-facts-raw`
- `Readable summaries artifact`: `dependency-facts-readable`
- Run URL: `https://github.com/${{ github.repository }}/actions/runs/${{ github.run_id }}`

Use `<details><summary>...</summary>` for verbose tables and raw excerpts.

## Optional issue drafts

Include up to 3 ready-to-open issue drafts for highest-priority recommendations as markdown blocks under a dedicated subsection in `### Suggested next actions`.

## No-action handling

If no meaningful action is required, post a concise no-action-needed report with evidence and call `noop` if no issue create/update is needed.

## Safety rules

- Never create dependency update PRs in this workflow.
- Never run automatic dependency modifications.
- Never fabricate policy or facts.
- Keep recommendations evidence-linked to files in `reports/`.

## Usage

- Edit this markdown body to refine ranking heuristics or report wording.
- If you edit frontmatter (triggers, permissions, tools, safe-outputs, network, steps), recompile with:
  - `gh aw compile --strict weekly-dependency-risk-and-upgrade-advisory`
