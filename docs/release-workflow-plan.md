# Release Workflow Implementation Plan (Living Document)

This document is a step-by-step plan for implementing the chosen approach:

- Option 4 (PR release-note block + aggregation + AI summarization)
- Plus selected improvements from other options
- release-drafter for automated GitHub Release drafts (Changesets dropped — not a good fit for a Go/Docker project)
- Without GoReleaser for now

Status legend:

- [ ] Not started
- [~] In progress
- [x] Done

---

## Area 1: PR Label Strategy and Automation

### Goal

Create a consistent, low-friction PR labeling system that improves:

- release note grouping,
- changelog quality,
- issue/PR discoverability,
- and future automation.

### Why this is first

Labels are the smallest change with the highest downstream impact:

- GitHub release notes can group by labels.
- Changeset and release aggregation pipelines can use labels for categorization.
- AI summarizers perform better with explicit metadata.

---

## 1.1 Decisions We Need To Make (Policy)

These are maintainer decisions and should be agreed before automation.

Current decision status (2026-04-18):

- [x] Keep the proposed taxonomy with one change: use type/performance instead of type/perf.
- [x] Keep impact labels and treat them as optional metadata (not required for merge).
- [x] Keep exactly one type label per PR.
- [x] Allow multiple area labels per PR.
- [x] Require at least one area label per PR.

### A) Label taxonomy

Recommended baseline:

- Type labels (exactly one required):
  - `type/feature`
  - `type/fix`
  - `type/docs`
  - `type/chore`
  - `type/refactor`
  - `type/performance`
  - `type/test`
- Area labels (one or more optional, but strongly recommended):
  - `area/server`
  - `area/agent`
  - `area/frontend`
  - `area/proto`
  - `area/docs`
  - `area/ci`
- Impact labels (optional, generous use encouraged):
  - `impact/breaking`
  - `impact/security`
  - `impact/ops`

### B) Required minimum labeling rules

Proposed policy:

1. Every PR must have exactly one `type/*` label.
2. Every PR must have at least one `area/*` label.
3. PRs may have multiple `area/*` labels when multiple parts of the monorepo are touched.
4. Impact labels are optional and can be added when they improve release context.
5. If `impact/breaking` is present, PR should include explicit migration notes in PR description.

### Type label hierarchy (how to choose exactly one)

Use the main outcome for users as the deciding factor.

1. If the core change introduces new user-visible capability, choose `type/feature`.
2. If the core change corrects wrong behavior, choose `type/fix`.
3. If the core change is speed or resource efficiency, choose `type/performance` (even if refactoring was required).
4. If the core change is internal structure/readability with no behavior change, choose `type/refactor`.
5. If the PR is primarily tests, choose `type/test`.
6. If the PR is primarily documentation/content, choose `type/docs`.
7. If the PR is maintenance/tooling/dependency/update work, choose `type/chore`.

Tie-breaker rule:

- Pick the label that best describes the primary reason the PR exists.
- Secondary work (docs/tests/refactors included with a feature/fix) should be reflected in PR description, not extra type labels.

Examples:

- New feature with docs and tests -> `type/feature`
- Refactor done specifically to reduce memory/CPU -> `type/performance`
- Refactor done for readability only -> `type/refactor`
- Docs-only PR -> `type/docs`

### C) Conflict rules

Deferred. The `exactly one type/*` enforcement already prevents all structural type conflicts. Explicit conflict checks will be added if the taxonomy grows or real-world conflicts are observed.

### D) Ownership

- Who can override labels when automation gets it wrong?
- Who owns label taxonomy updates?

---

## 1.2 What You Need To Do vs What Can Be Automated

### You (maintainer) need to do

1. Approve initial taxonomy and naming.
2. Approve required-label and conflict rules.
3. Decide if bots may auto-apply labels or only suggest labels.
4. Decide who owns periodic taxonomy cleanup.

### Automation can do

1. Apply labels by file path (GitHub Labeler).
2. Fail PR checks when required labels are missing.
3. Warn when conflicting labels coexist.
4. Suggest likely labels using AI (optional phase).

---

## 1.3 Implementation Steps

### Step 1: Create/normalize labels in repository settings

- [x] Add agreed labels and descriptions.
- [x] Pick colors consistently by group (type/area/impact).

### Step 2: Add path-based auto-labeling

- [x] Add labeler configuration file (for example `.github/labeler.yml`).
- [x] Map directory patterns to `area/*` labels.

Suggested initial mapping:

- `server/**` -> `area/server`
- `agent/**` -> `area/agent`
- `frontend/**` -> `area/frontend`
- `proto/**` -> `area/proto`
- `docs/**` -> `area/docs`
- `.github/**` -> `area/ci`

### Step 3: Enforce required type labels

- [x] Add a PR check that ensures exactly one `type/*` label.
- [x] Add user-friendly error output (what is missing and how to fix it).
- [x] Enforce at least one `area/*` label.

### Step 4: Add conflict checks

- [x] Deferred — covered by `exactly one type/*` enforcement. Revisit if taxonomy grows or real conflicts appear.

### Step 5: Update templates and contributor docs

- [x] Update PR template checklist to mention required labels.
- [x] Add short contributor section in README or docs.
- [x] Add a concise type-label hierarchy reference (copy from this section).

### Step 6: Optional AI label suggestion workflow

- [ ] Start with comment-only suggestions.
- [ ] Evaluate false positives for 2-4 weeks.
- [ ] Decide whether to enable auto-apply on safe categories.

---

## 1.4 GitHub-native features to use now

Recommended immediate tooling (low risk):

1. `actions/labeler` for path-based area labeling. Implemented in `.github/workflows/pr-labeler.yml`.
2. A simple workflow check (script/action) for required `type/*` labels. Implemented in `.github/workflows/pr-label-check.yml`.
3. Branch protection requiring label-check pass before merge.

Optional later:

1. AI label suggestions from PR diff/description.
2. Auto-label from title prefix (for example `docs:` -> `type/docs`).

---

## 1.5 Acceptance Criteria for Area 1

Area 1 is complete when:

- [x] 100% of merged PRs have exactly one `type/*` label.
- [x] At least 90% of merged PRs have one or more correct `area/*` labels.
- [x] Label-check workflow is required by branch protection.
- [x] Contributors can discover rules quickly from template/docs.

---

## 1.6 Open Questions

- ~~Should dependency-update bots map to `type/chore` automatically?~~ Resolved: Renovate is configured with `type/chore` + `area/ci` as default labels. Maintainers can relabel individual PRs if a change warrants more attention.
- Should some repository-wide metadata-only PRs default to `area/ci`, or should they always be labeled manually? — Deferred, low priority; Renovate handling covers most cases.

---

## Immediate Next Action

Recommended next step:

1. Do both in one PR (policy + enforcement together):
  - [x] Add `.github/labeler.yml` with area mappings.
  - [x] Add a PR workflow check for exactly one `type/*` label.
  - [x] Update PR template/checklist to point to this hierarchy.

Next recommended step:

1. Move on to Area 2 (PR and commit naming policy).

---

## Area 2: PR and Commit Naming Policy

### Goal

Establish a naming convention that produces clean, meaningful merge history and release notes without blocking contributors who make small or informal commits during development.

### Why this matters

- The PR title becomes the merge commit message on squash merge — it is the one message that persists in `git log --first-parent` and in release notes.
- Individual commits during development are often exploratory or incremental. Enforcing format there adds friction for little gain.
- A clear, well-structured PR title is the single highest-leverage convention to establish.

---

## 2.1 Decisions We Need To Make (Policy)

Current decision status (2026-04-18):

- [x] Guidance-only approach. No CI check will block merge based on PR title or commit message format.
- [x] Focus on PR title quality (it becomes the merge commit).
- [x] Individual commits during development may be informal.
- [x] Use squash-only merge strategy for PRs into `main`.
- [x] Maintainers are encouraged (not required) to use Conventional Commit-style prefixes for squash commit subjects.

### A) PR title format

Recommended format (guidance, not enforced):

```
<imperative summary of what the PR does>
```

Rules of thumb:
- Start with an imperative verb: `Add`, `Fix`, `Remove`, `Update`, `Refactor`, `Improve`, `Support`
- Keep under 72 characters
- No trailing period
- Be specific enough to understand without reading the PR body

Examples:
- `Add backup scheduling support to agent`
- `Fix crash when server is unreachable during startup`
- `Improve restic snapshot listing performance`
- `Update golangci-lint to v2.1`

Avoid:
- `WIP: ...` (resolve before merging)
- `Fix bug` (too vague)
- `changes` (meaningless)
- Conventional Commits prefix format (`feat:`, `fix:`) — not required, but acceptable if preferred

### B) Merge commit format

With squash merge enabled, GitHub uses the PR title as the merge commit subject and PR description (first paragraph) as the body. This means:

1. A well-written PR title → clean one-liner in `git log`
2. A well-written PR description → context in `git show`

No additional merge commit template is needed beyond getting the PR title right.

### C) Individual commit guidance

No format enforced. Guidance only:

- Short commits during feature work are fine
- A rebase/squash before PR is welcome but not required
- If a commit is meaningful standalone (e.g. a fix worth referencing), write a real message for it

### D) Conventional Commits

Not adopted as a requirement. Contributors who prefer `feat:` / `fix:` style may use it. It will not be parsed or relied on by any tooling.

For maintainers at merge time, a lightweight prefix convention for squash commit subjects is documented in `docs/maintainer-guidelines.md` to improve main-branch history and release readability without adding CI enforcement.

---

## 2.2 What You Need To Do vs What Can Be Automated

### You (maintainer) need to do

1. Keep PR title guidance examples up to date as the project evolves.
2. Decide whether to add optional non-blocking title hints now or after observing real PR patterns.

### Automation can do

1. Nothing required for guidance-only. Optional: a linter that posts a non-blocking comment when a PR title looks too short or starts with `WIP`.

---

## 2.3 Implementation Steps

### Step 1: Document the PR title convention

- [x] Add PR naming guidance to `CONTRIBUTING.md` (PR title section).
- [x] Add example good/bad titles.
- [x] Add note that PR title becomes merge commit.

### Step 2: Enforce squash merge in repository settings

- [x] Enable "Allow squash merging" and disable "Allow merge commits" and "Allow rebase merging" in GitHub repository settings (or via `gh` CLI).
- [x] This ensures PR title always becomes the commit message.

### Step 3: Update PR template

- [x] Add a short reminder in the PR template header about title quality (one line, non-blocking).
- [x] Add reference to maintainer squash subject guidance.

### Step 4: Optional non-blocking title lint

- [x] Add a workflow that posts a comment (but does not fail) when PR title is fewer than 10 characters or starts with `WIP`, `draft`, `temp`, or `fix:` without a subject.
- [ ] Evaluate after 1 month of real PRs.

---

## 2.4 Acceptance Criteria for Area 2

Area 2 is complete when:

- [x] CONTRIBUTING.md has a clear PR title guidance section with examples.
- [x] Squash merge is the only merge strategy enabled.
- [x] PR template includes a title quality reminder.
- [x] Maintainer-facing squash subject guidance is documented.

---

## 2.5 Open Questions

- ~~Should rebase merging also be disabled, or kept as an option for maintainers?~~ Resolved: rebase and merge-commit strategies are disabled; squash merge is the only allowed PR merge strategy.
- ~~Should the optional title lint workflow be added now or after observing a few real PRs?~~ Resolved: added now as non-blocking hints via `.github/workflows/pr-title-hint.yml`.

---

## Immediate Next Steps for Area 2

1. Evaluate non-blocking title hint usefulness after 1 month of real PR data.
2. Move on to Area 3 (Changesets policy).

---

## Area 3: Release Drafting and Versioning

### Goal

Automatically draft GitHub Releases as PRs are merged, so that when a maintainer decides to cut a release they have a ready-to-review draft rather than a blank page. Each release clearly shows breaking changes at the top, summarises what changed, and links to all contributing PRs.

### Why this approach

- **release-drafter** reads merged PR labels and titles, groups them by category, and keeps a rolling draft release up to date automatically.
- Changesets was considered and dropped — it is designed for npm monorepo versioning and requires contributors to run tooling in their branch. The overhead is not justified for a Go + Docker project where no packages are published to a registry.
- Docker images are the delivery artefact, not npm packages.
- A single shared version across server, agent, and frontend is the chosen policy for now.

---

## 3.1 Decisions (Policy)

Current decision status (2026-04-18):

- [x] Use release-drafter to generate and maintain a rolling GitHub Release draft.
- [x] Drop Changesets.
- [x] Follow SemVer. Bump is decided by the highest-impact label among merged PRs since the last release:
  - `impact/breaking` → major
  - `type/feature` → minor
  - anything else → patch
- [x] Release is triggered manually: a maintainer reviews the draft, adjusts it if needed, sets the version tag, and publishes.
- [x] No CHANGELOG.md committed to the repo. The GitHub Release page is the source of truth.
- [x] All three components (server, agent, frontend) share a single version tag.
- [x] Docker images tagged `:latest` on every push to `main`; tagged `:vX.Y.Z` and `:X.Y` on every semver tag.

### Release note structure

The generated draft will render in this order:

1. ⚠️ Breaking Changes — PRs with `impact/breaking`
2. 🔒 Security — PRs with `impact/security`
3. ✨ Features — PRs with `type/feature`
4. 🐛 Bug Fixes — PRs with `type/fix`
5. ⚡ Performance — PRs with `type/performance`
6. 🔧 Maintenance — PRs with `type/chore`, `type/refactor`, `type/test`, `type/docs`
7. Full Changelog link (`PREVIOUS_TAG...vX.Y.Z`)

Categories with no PRs are omitted automatically.

### Excluding a PR from release notes

Add the `skip-changelog` label to a PR to omit it from the draft entirely. Useful for typo fixes, trivial Renovate bumps, or CI-only changes where the PR title would add noise.

---

## 3.2 Release Workflow (How to Cut a Release)

```
PRs merged to main
  → release-drafter updates draft release automatically
  → build-push.yml pushes :latest + :sha-XXXX images to ghcr.io

Maintainer is ready to release:
  1. Open Releases → Edit the draft
  2. Review and edit the release notes if needed
  3. Set the version tag (e.g. v1.2.0) using release-drafter's suggestion
  4. Publish the release
  → GitHub creates the tag on main
  → build-push.yml triggers on the new v* tag
  → Pushes :v1.2.0 and :1.2 images to ghcr.io
```

---

## 3.3 Implementation Steps

### Step 1: Add release-drafter config and workflow

- [x] Create `.github/release-drafter.yml` with category template and version-resolver.
- [x] Create `.github/workflows/release-drafter.yml`.
  - Runs on push to `main` (updates draft with newly merged PRs).
  - Runs on PR label changes (keeps suggested version current while PR is open).

### Step 2: Update Docker image tagging

- [x] Add `type=raw,value=latest,enable={{is_default_branch}}` to all three builds in `build-push.yml`.
  - Previously, only `type=ref,event=branch` was present, which produced a `:main` tag but not `:latest`.

### Step 3: Add `skip-changelog` label to repository

- [x] Create a `skip-changelog` label (colour: grey, description: \"Exclude from release notes\").

### Step 4: Document the release process for maintainers

- [x] Add a \"Cutting a Release\" section to `docs/maintainer-guidelines.md`.
  - Link to Releases page.
  - Step-by-step: review draft → edit notes → set tag → publish.
  - When to choose major / minor / patch.
  - Note that publishing the release triggers the Docker image tag push.

---

## 3.4 Acceptance Criteria for Area 3

Area 3 is complete when:

- [x] Every PR merged to `main` is automatically reflected in the rolling draft release.
- [x] Breaking changes and security items appear first in the draft.
- [x] Draft shows a version suggestion based on merged PR labels.
- [x] Docker `:latest` is pushed on every merge to `main`.
- [x] Docker `:vX.Y.Z` and `:X.Y` are pushed when a semver tag is created.
- [x] `skip-changelog` label exists in the repository.
- [x] Maintainer release guide is written in `docs/maintainer-guidelines.md`.

---

## 3.5 Open Questions

- Should Renovate dependency PRs get `skip-changelog` automatically to reduce noise in patch releases? — To evaluate after first few releases.
- Should a `release` GitHub environment be set up to gate the Docker `:vX.Y.Z` push behind a manual approval step? — Deferred, low priority for now.

---

## Immediate Next Steps for Area 3

1. ~~Create `skip-changelog` label in the repository.~~ Done.
2. ~~Write maintainer release guide in `docs/maintainer-guidelines.md`.~~ Done.

---

## Area 5: Release Assembly Pipeline

### Goal

Produce a richer release draft than release-drafter alone can provide. release-drafter captures PR titles; this pipeline captures the full `release-note` block from each PR body, groups entries by category, and outputs structured markdown ready to paste into or replace the draft.

### Why this is separate from release-drafter

release-drafter runs automatically and is the live rolling draft. The release assembly pipeline is run **once per release** by the maintainer when they are about to publish. It enriches the draft with the extended per-PR text that contributors wrote in the `release-note` block.

---

## 5.1 Decisions (Policy)

Current decision status (2026-04-18):

- [x] Source script lives at `scripts/release-notes.py`. It calls `gh` CLI and requires no build tooling beyond Python 3 and an authenticated `gh`.
- [x] The script reads the `release-note` block from each merged PR body. Falls back to PR title when no block is present. Skips `NONE` entries and `skip-changelog` PRs.
- [x] Categories and order match the release-drafter config: breaking → security → feature → fix → performance → maintenance.
- [x] The script is invokable locally via `just release-notes` and remotely via a `workflow_dispatch` action.
- [x] The workflow finds the live draft release and updates its body with the generated notes.

### Source-of-truth priority

1. `release-note` block in PR body (most detailed, user-authored)
2. PR title (fallback when no block is present)
3. PR is omitted if it has `skip-changelog` or the block contains `NONE`

---

## 5.2 Implementation Steps

### Step 1: Write the release-notes script

- [x] Create `scripts/release-notes.py`.
  - Accepts `--from-tag TAG` (default: last git tag auto-detected via `git describe`).
  - Accepts `--output FILE` (default: stdout).
  - Fetches merged PRs since the tag date using `gh pr list`.
  - Extracts `release-note` blocks with regex.
  - Groups into categories, renders structured markdown.

### Step 2: Add just recipe

- [x] Add `release-notes` recipe to justfile.
  - `just release-notes` — auto-detect last tag.
  - `just release-notes from-tag=v1.0.0` — explicit start tag.

### Step 3: Add workflow_dispatch workflow

- [x] Create `.github/workflows/refresh-release-draft.yml`.
  - Manual trigger with optional `from_tag` input.
  - Runs the script, then uses `gh release edit` to update the draft body.
  - Fails with a clear message if no draft release exists yet.

---

## 5.3 Typical Usage

**Before publishing a release (local):**
```bash
just release-notes          # prints draft to terminal
just release-notes > /tmp/notes.md  # save to file for editing
```

**Before publishing a release (via GitHub UI):**
1. Go to Actions → Refresh Release Draft → Run workflow.
2. Open Releases → Edit draft to review and adjust.
3. When satisfied, set the version tag and publish.

**Specifying a range manually:**
```bash
just release-notes from-tag=v1.0.0
```

---

## 5.4 Acceptance Criteria for Area 5

Area 5 is complete when:

- [x] `scripts/release-notes.py` collects PR release-note blocks and groups them by category.
- [x] `just release-notes` runs the script with auto-detected tag range.
- [x] `workflow_dispatch` workflow updates the draft release body.
- [x] PRs with `skip-changelog` or `NONE` release notes are excluded.
- [x] Breaking changes appear first, maintenance last.

---

## 5.5 Open Questions

- Should the workflow post a summary comment on the run with the generated notes for easy copy-paste? — Low priority, the draft update is sufficient.
- Should the script support `--to-ref` to generate notes for a specific historical range? — Can be added if needed.

---

## Immediate Next Steps for Area 5

1. Move on to Area 6 (AI role boundaries and polish step).

---

## Area 6: AI Role Boundaries and Polish Step

### Goal

Add a human-readable summary paragraph at the top of each release draft — written by AI, grounded only in the structured notes already generated by Area 5. The AI enriches readability without replacing the traceable PR list.

### Why this approach

- Release notes generated from PR titles and blocks are accurate but can read as a flat list. A short narrative paragraph helps users quickly decide "does this release affect me?".
- Grounding the AI strictly in the structured input prevents hallucination (no invented features or fixes).
- GitHub Models API (`gpt-4o`) is accessible via `GITHUB_TOKEN` — no additional secrets or API keys required.

---

## 6.1 Decisions (Policy)

Current decision status (2026-04-18):

- [x] AI provider: GitHub Models API (`gpt-4o`) using `GITHUB_TOKEN`. No extra secret needed.
- [x] AI output: a 3–6 sentence summary paragraph, prepended under the `## What's Changed` heading. The full categorized PR list is preserved unchanged below it.
- [x] AI constraints: system prompt explicitly forbids the model from mentioning anything not present in the input. Breaking changes and security items must be noted if present.
- [x] The AI step is **optional** — the workflow exposes an `ai_polish` boolean input (default: true). Running without AI produces the same structured draft as Area 5 alone.
- [x] The script falls back to `gh auth token` when `GITHUB_TOKEN` is not set in the environment, making local use frictionless.

### No-hallucination guarantee (best-effort)

The system prompt instructs the model to only describe changes that are explicitly listed in the input. The maintainer must still review the summary before publishing — the AI summary is a starting point, not a trusted source.

---

## 6.2 Implementation Steps

### Step 1: Write the AI polish script

- [x] Create `scripts/ai-polish.py`.
  - Reads notes from stdin or `--input FILE`.
  - Calls `https://models.inference.ai.azure.com/chat/completions` with `gpt-4o`.
  - Injects the summary under `## What's Changed`, preserving the rest of the document.
  - Falls back to `gh auth token` locally when `GITHUB_TOKEN` is not set.

### Step 2: Add just recipe

- [x] Add `release-notes-polished` recipe to justfile.
  - Chains `release-notes.py` → `ai-polish.py` in one command.
  - `just release-notes-polished` — auto-detect last tag.
  - `just release-notes-polished from-tag=v1.0.0` — explicit start tag.

### Step 3: Update workflow

- [x] Add `ai_polish` boolean input (default: `true`) to `refresh-release-draft.yml`.
  - When enabled: runs `ai-polish.py` after `release-notes.py`, before updating the draft.
  - When disabled: skips the AI step entirely and pushes the raw structured notes.
  - Added `actions/setup-python@v5` step (GitHub-hosted runners don't have Python pre-installed for all distros).

---

## 6.3 Typical Usage

**Full pipeline locally (structured + AI summary):**
```bash
just release-notes-polished          # prints to stdout
just release-notes-polished > /tmp/polished.md   # save for editing
```

**Structured notes only (no AI):**
```bash
just release-notes
```

**Via GitHub Actions:**
1. Actions → Refresh Release Draft → Run workflow.
2. `ai_polish` checkbox is pre-ticked — uncheck to skip AI.
3. Open Releases → Edit draft to review.

---

## 6.4 Acceptance Criteria for Area 6

Area 6 is complete when:

- [x] `scripts/ai-polish.py` injects a summary paragraph without modifying the PR list.
- [x] The summary only describes changes present in the input (system prompt enforces this).
- [x] `just release-notes-polished` produces a polished draft in one command.
- [x] The workflow `ai_polish` input can disable the step when raw output is preferred.
- [x] No additional GitHub secrets required — uses `GITHUB_TOKEN`.

---

## 6.5 Open Questions

- Should the AI also suggest a one-line release title (currently derived from the version tag only)? — Deferred, nice-to-have.
- Should there be a review/diff step in the workflow that shows the AI changes before updating the draft? — Deferred; the maintainer reviews in the Releases UI.

---

## Immediate Next Steps for Area 6

All implementation steps complete. No further actions required for this area.

Next: Areas 7–9 are lower priority. Area 7 (issue templates) can be tackled if contributor UX becomes a friction point.

---

## Area 7: Templates and Contributor UX

### Goal

Ensure issue templates capture enough metadata (component, impact) that issues can be linked to the correct labels and feed into the release pipeline naturally. Also disable blank issues so all reports land in a structured form.

### What already existed

The four issue templates (bug, feature, question, other) were already in place and well-structured. The gaps relative to the release workflow were:
- No `labels` set on bug/feature templates — `type/*` labels weren't applied automatically on issue creation.
- No component dropdown — reporters couldn't indicate which area was affected.
- No impact field on bug reports — `impact/breaking` or `impact/security` signals were lost.
- No `config.yml` — blank issues were allowed, and there was no Discussions link.

---

## 7.1 Decisions (Policy)

Current decision status (2026-04-18):

- [x] Bug report template auto-applies `type/fix` label on creation.
- [x] Feature request template auto-applies `type/feature` label on creation.
- [x] Both templates include a required **Component** multi-select dropdown (Server, Agent, Frontend, Proto, Docs, CI, Other).
- [x] Bug report includes an optional **Impact** multi-select (breaking change, security, ops, none). This is informational — maintainers apply `impact/*` labels based on the selection.
- [x] `config.yml` added: blank issues disabled, Discussions link surfaced.

### Note on label application from issue templates

GitHub applies the `labels` field from issue templates automatically at creation time. The component and impact dropdowns are **informational** — a maintainer still needs to manually apply the corresponding `area/*` and `impact/*` labels to the issue. This is acceptable; the dropdown answers make that decision obvious.

---

## 7.2 Implementation Steps

### Step 1: Update bug report template

- [x] Add `labels: ["type/fix"]` to auto-apply type label.
- [x] Add **Component** multi-select dropdown (required).
- [x] Add **Impact** multi-select dropdown (optional).

### Step 2: Update feature request template

- [x] Add `labels: ["type/feature"]` to auto-apply type label.
- [x] Add **Component** multi-select dropdown (required).

### Step 3: Add issue template config

- [x] Create `.github/ISSUE_TEMPLATE/config.yml`.
  - `blank_issues_enabled: false` — forces use of templates.
  - Discussions contact link — directs Q&A traffic away from issues.

---

## 7.3 Acceptance Criteria for Area 7

Area 7 is complete when:

- [x] Bug issues automatically get `type/fix` label on creation.
- [x] Feature issues automatically get `type/feature` label on creation.
- [x] Both templates ask for the affected component.
- [x] Bug template asks for impact type.
- [x] Blank issues are disabled.

---

## Immediate Next Steps

Areas 8 (Governance/cadence) and 9 (Measurement) are skipped — too early and low priority for current project stage.

The release workflow implementation is complete. All active areas (1–7) are done.
