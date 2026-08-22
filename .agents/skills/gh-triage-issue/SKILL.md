---
name: gh-triage-issue
description: >-
  Triage a GitHub issue: apply correct type/area/impact labels, improve weak titles,
  detect duplicates, and post a structured triage comment when action is needed.
  Use when the user invokes /gh-triage-issue, asks to triage an issue, fix issue labels,
  improve an issue title, or check issue quality. Also use after manually creating an issue
  that may be missing area or impact labels.
---

# GitHub Issue Triage

Triage one or more GitHub issues so they are labeled correctly, clearly titled, and actionable.

## Policy sources

Follow these files exactly — do not invent policy:

- `CONTRIBUTING.md`
- `docs/workflow.md`
- `docs/maintainer-guidelines.md`
- `AGENTS.md`

## Prerequisites

- `gh` CLI authenticated for this repository.
- Disable pager in non-interactive runs:

```bash
export GH_PAGER=cat
export PAGER=cat
```

## Scope

- **Single issue** (default): user provides issue number, URL, or "this issue" from context.
- **Multiple issues**: user asks to triage all open issues — process each conservatively; skip issues that are already in good shape.

## Label policy

Use **existing repository labels only**. Never create new labels.

### Type labels (exactly one when possible)

| Label | When |
|-------|------|
| `type/feature` | Feature request, enhancement |
| `type/fix` | Bug, defect, regression |
| `type/docs` | Documentation-only change request |
| `type/chore` | Tooling, housekeeping, non-user-facing maintenance |
| `type/refactor` | Internal restructuring without behavior change |
| `type/performance` | Performance improvement |
| `type/test` | Test coverage or test infrastructure |

### Area labels (one or more only when truly cross-cutting)

| Label | When |
|-------|------|
| `area/server` | Server API, gRPC server, SQLite, REST |
| `area/agent` | Agent daemon, scheduler, restic executor |
| `area/frontend` | Vue SPA, UI, Pinia stores |
| `area/proto` | Protobuf schemas, code generation |
| `area/docs` | Documentation, AGENTS.md, contributor docs |
| `area/ci` | GitHub Actions, Docker build, CI tooling |

### Impact labels (optional, when applicable)

| Label | When |
|-------|------|
| `impact/breaking` | Breaking change or existing setup stops working |
| `impact/security` | Security vulnerability or hardening |
| `impact/ops` | Deployment, operations, or production concern |

### Other labels

| Label | When |
|-------|------|
| `discussion`, `question`, `help wanted` | Usage help or general discussion — recommend Discussions per `docs/workflow.md` |
| `duplicate` | Only when confidence is **high** — never auto-close |

### Component → area mapping (from issue templates)

When the issue body includes a **Component** answer, map it:

| Template value | Label |
|----------------|-------|
| Server | `area/server` |
| Agent | `area/agent` |
| Frontend | `area/frontend` |
| Proto / gRPC | `area/proto` |
| Documentation | `area/docs` |
| CI / Build | `area/ci` |
| Other / Unknown | Infer from title/body; skip if unclear |

### Impact → label mapping (from issue templates)

| Template value | Label |
|----------------|-------|
| Breaking change (existing setup stops working) | `impact/breaking` |
| Security vulnerability | `impact/security` |
| Operations / deployment concern | `impact/ops` |

## Workflow

### Step 1: Load the issue

```bash
gh issue view <number> --json number,title,body,labels,state,url
```

### Step 2: Assess quality

**Title** — improve only when clearly weak (e.g. "Bug", "Help", "Not working"). Good titles include symptom + affected component.

**Body** — for bugs, check for: reproduction steps, expected vs actual, version, logs.
For features, check for: problem statement, desired outcome.

**Duplicates** — search open issues:

```bash
gh issue list --state open --limit 50 --json number,title
```

Compare title, symptom, and component overlap. Report confidence: `low`, `medium`, `high`. Only suggest `duplicate` label at **high** confidence.

**Frontend UX/a11y** — for `area/frontend` issues involving UX or accessibility, note missing WCAG-relevant details (contrast, keyboard nav, focus, labels, touch targets, screen readers, viewport).

### Step 3: Apply changes

Minimize churn — do not remove correct labels only to re-add equivalent ones.

```bash
# Update title when needed
gh issue edit <number> --title "<improved title>"

# Add labels
gh issue edit <number> --add-label "area/server" --add-label "impact/security"

# Remove incorrect labels (only when clearly wrong)
gh issue edit <number> --remove-label "type/chore"
```

### Step 4: Comment only when action is needed

Post **at most one** triage comment per issue per run. Comment when at least one of:

- labels were added or removed,
- title was improved,
- quality gaps remain (clarifying questions needed),
- duplicate candidates found,
- issue should move to Discussions.

If labels are correct and quality is good, **do not comment**.

Use this structure (sections in order):

```markdown
### Triage Summary
<short judgment of type, scope, readiness>

### Label Actions Taken (with rationale)
<each add/remove and why>

### Quality Gaps Found
<missing info blocking prioritization — or "None">

### Clarifying Questions
<max 5 questions, only if needed>

### Potential Duplicates
<candidate links + confidence — or "None found">

### Recommended Status Next Step
<e.g. ready-for-refinement, needs-repro, awaiting-reporter, or move to Discussions>
```

Write comment body to a temp file, then post:

```bash
comment_file="$(mktemp /tmp/gh-triage-comment-XXXXXX.md)"
# write body to $comment_file
gh issue comment <number> --body-file "$comment_file"
rm -f "$comment_file"
```

## Operational rules

- Be conservative. Avoid noise and label churn.
- Never fabricate evidence or policy.
- Never auto-close issues.
- Report a concise summary to the user: issue URL, labels changed, title changed, whether a comment was posted.

## Related skills

- `gh-create-issue` — create new issues with correct labels from the start.
- `gh-relevance-check` — evaluate whether a stale issue or PR is still relevant.
