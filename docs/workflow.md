# Contributor and Release Workflow

This document walks through the full lifecycle of a change in this repository: from opening an issue, through development and pull request, to a published release. It is the single reference to read before contributing or cutting a release.

---

## Table of Contents

1. [Reporting an Issue](#1-reporting-an-issue)
2. [Starting Development](#2-starting-development)
3. [Opening a Pull Request](#3-opening-a-pull-request)
4. [Review and Merge](#4-review-and-merge)
5. [Cutting a Release](#5-cutting-a-release)
6. [Reference Quick-Card](#6-reference-quick-card)

---

## 1. Reporting an Issue

### Choose the right template

Go to **Issues → New Issue** and select the matching template:

| Template | When to use |
|---|---|
| 🐛 Bug Report | Something is broken or behaving incorrectly |
| 💡 Feature Request | You want a new capability |
| ❓ Questions & Discussion | Help, usage questions |
| 🤔 Other | Doesn't fit the above |

> For general discussion or questions, prefer [Discussions](https://github.com/tryy3/backup-orchestrator/discussions) over an issue.

### What the template will ask for

**Bug reports** ask for: platform, version, affected component(s), impact type (breaking / security / none), reproduction steps, expected vs. actual behaviour, and log output.

**Feature requests** ask for: platform, version, affected component(s), the problem being solved, the desired solution, and alternatives considered.

### After submission

- The template auto-applies `type/fix` (bugs) or `type/feature` (features).
- A maintainer will apply the appropriate `area/*` label (server, agent, frontend, …) and any `impact/*` label based on the Component and Impact answers.
- No action needed from the reporter beyond answering follow-up questions.

### Reference

- [CONTRIBUTING.md](../CONTRIBUTING.md)

---

## 2. Starting Development

### Before you write any code

1. **Check for an existing issue.** If one doesn't exist, open one first (see §1) so the work is tracked.
2. **Check the roadmap** — [ROADMAP.md](../ROADMAP.md) — to see if the area is already planned or in progress.

### Set up the dev environment

The project uses [Nix flakes](../flake.nix) with direnv for a fully reproducible shell:

```bash
# First-time setup
direnv allow          # activates the Nix dev shell automatically on cd

# Or manually enter the shell
nix develop
```

Tools available in the shell: `go`, `node`, `just`, `gh`, `buf`, `golangci-lint`, `lefthook`, `air`, `restic`, `python3`, and more.

### Install git hooks

```bash
lefthook install      # registers pre-commit hooks (gofmt + go vet on staged files)
```

### Run the stack locally

```bash
just dev-server       # Go server with hot reload (air)
just dev-agent        # Go agent with hot reload (air)
just dev-frontend     # Vite dev server with HMR
```

All three together in Zellij tabs: `just dev`.

### Run tests before committing

```bash
just test             # server + agent + frontend
just test-server      # Go server only (race detector)
just test-agent       # Go agent only (race detector)
just test-frontend    # Vitest
```

### Reference

- [CLAUDE.md](../CLAUDE.md) — project conventions, package structure, key design decisions
- [docs/architecture-overview.md](architecture-overview.md)
- [docs/local-dev-setup.md](local-dev-setup.md)

---

## 3. Opening a Pull Request

### Branch naming

No formal convention enforced. Use something descriptive:

```
fix/agent-crash-on-reconnect
feat/backup-scheduling
chore/update-golangci-lint
```

### Push and create the PR

```bash
git push -u origin <your-branch>
gh pr create --base main --head <your-branch> --title "<title>" \
  --label "type/feature" --label "area/agent"
```

Or open via the GitHub UI after pushing.

### PR title

The PR title becomes the squash merge commit subject — it is the one message that persists in `git log` and release notes. Make it count:

| ✅ Good | ❌ Avoid |
|---|---|
| `Add backup scheduling support to agent` | `WIP: scheduling` |
| `Fix crash when server is unreachable during startup` | `Fix bug` |
| `Improve restic snapshot listing performance` | `changes` |

Rules of thumb: imperative verb, under 72 characters, specific enough to understand without reading the body.

### Labels (required — enforced by CI)

Every PR must have:

- **Exactly one** `type/*` label
- **At least one** `area/*` label

| If the PR primarily… | Use |
|---|---|
| Adds a new user-visible capability | `type/feature` |
| Corrects wrong behaviour | `type/fix` |
| Improves speed / resource use | `type/performance` |
| Restructures code with no behaviour change | `type/refactor` |
| Adds or changes tests | `type/test` |
| Updates docs only | `type/docs` |
| Maintenance, dependencies, tooling | `type/chore` |

Area labels: `area/server`, `area/agent`, `area/frontend`, `area/proto`, `area/docs`, `area/ci`. Use multiple if the PR touches multiple areas.

Optional impact labels: `impact/breaking`, `impact/security`, `impact/ops`.

> The `PR Label Check` workflow enforces this. PRs without the correct labels cannot be merged.

### Release note block

At the bottom of the PR template there is a `release-note` block:

````markdown
```release-note
<describe the user-facing change here>
```
````

**Fill this in for every user-facing change.** Write `NONE` for CI, internal refactors, maintenance, or test-only PRs.

This text is extracted by `scripts/release-notes.py` when assembling the release draft. The quality of this field directly determines the quality of release notes.

Examples:

```
# Feature
Add scheduled backup support to the agent. Schedules are configured via the
server and pushed to agents over the gRPC stream.

# Fix
Fix agent crash on startup when the server is unreachable and no local config
exists. The agent now falls back to a safe idle state and retries.

# Not user-facing
NONE
```

### Checklist

The PR template includes a checklist. Key items:

- [ ] Description explains what and why, not just what
- [ ] Self-reviewed the diff before requesting review
- [ ] `type/*` and `area/*` labels are set
- [ ] Release note is filled or marked `NONE`

### Reference

- [CONTRIBUTING.md](../CONTRIBUTING.md) — PR title guidance, full label hierarchy
- [docs/maintainer-guidelines.md](maintainer-guidelines.md) — squash commit subject convention (maintainer-facing)

---

## 4. Review and Merge

_This section is primarily for maintainers._

### Reviewing a PR

1. Check that labels are correct (`PR Label Check` must be green before merge is possible).
2. Verify the release note block is filled accurately.
3. If `impact/breaking` is set, ensure explicit migration notes are in the PR body.
4. Review the squash commit subject GitHub will generate (shown in the merge dialog). Adjust the PR title if needed so the squash subject follows the preferred format:

   ```
   feat(agent): add backup scheduling support
   fix(server): prevent startup crash when config is missing
   ```

   Reference: [docs/maintainer-guidelines.md](maintainer-guidelines.md#recommended-squash-commit-subject-format)

### Merging

**Squash merge is the only allowed strategy.** The PR title becomes the commit subject on `main`.

After merging:
- `build-push.yml` triggers automatically and pushes `:latest` + `:sha-XXXX` Docker images to ghcr.io.
- `release-drafter.yml` updates the rolling draft release with the new PR.

---

## 5. Cutting a Release

_Maintainer only._

### When to release

There is no fixed cadence. Release when there is enough user-facing change to justify communicating it. Check the rolling draft release (see below) to assess what has accumulated since the last release.

### Step 1 — Check the rolling draft

The release-drafter action keeps a draft release up to date automatically after every merge to `main`. It groups PRs by label (breaking → security → features → fixes → performance → maintenance) and suggests a version bump based on the highest-impact label present.

Open [Releases](https://github.com/tryy3/backup-orchestrator/releases) and click **Edit** on the draft.

### Step 2 — Enrich the draft with PR release-note blocks

The rolling draft contains only PR titles. To get the richer `release-note` block content from each PR, run the release assembly pipeline. Choose one:

**Via GitHub Actions (recommended):**

1. Go to Actions → **Refresh Release Draft** → Run workflow.
2. Leave `from_tag` blank to auto-detect the last release tag.
3. Leave `ai_polish` ticked (default) to add a generated summary paragraph.
4. Wait for the run to complete, then re-open the draft — it will have been updated.

**Locally:**

```bash
# Structured notes only
just release-notes

# Structured notes + AI summary paragraph (requires gh auth login)
just release-notes-polished

# Explicit range
just release-notes from-tag=v1.0.0
just release-notes-polished from-tag=v1.0.0
```

The AI step calls GitHub Models (`gpt-4o`) via your `GITHUB_TOKEN`. It is only allowed to summarise what is in the input — it cannot invent features or fixes.

### Step 3 — Review and edit the draft

Things to check:

- The AI summary paragraph is accurate and reads naturally. Edit it if needed.
- Every breaking change has an explanation and migration steps.
- Any trivial PRs that add noise can be removed (or add `skip-changelog` label to their PR retroactively and re-run the pipeline).
- The version suggestion from release-drafter reflects the right bump:

  | Highest-impact label in this batch | Version bump |
  |---|---|
  | `impact/breaking` | **major** (x.0.0) |
  | `type/feature` | **minor** (0.x.0) |
  | anything else | **patch** (0.0.x) |

### Step 4 — Publish

1. Set the **Tag version** field to the chosen version (e.g. `v1.2.0`).
2. Confirm **Target** is `main`.
3. Click **Publish release**.

GitHub creates the tag on `main`. `build-push.yml` triggers on the new `v*` tag and pushes:

| Image | Tags pushed |
|---|---|
| `ghcr.io/tryy3/backup-orchestrator-server` | `:v1.2.0`, `:1.2`, `:latest`, `:sha-XXXX` |
| `ghcr.io/tryy3/backup-orchestrator-agent` | `:v1.2.0`, `:1.2`, `:latest`, `:sha-XXXX` |
| `ghcr.io/tryy3/backup-orchestrator-agent-db` | `:v1.2.0`, `:1.2`, `:latest`, `:sha-XXXX` |

All three components share the same version tag.

### Reference

- [docs/maintainer-guidelines.md](maintainer-guidelines.md#cutting-a-release)

---

## 6. Reference Quick-Card

### Commands

```bash
# Dev
just dev-server              # server with hot reload
just dev-agent               # agent with hot reload
just dev-frontend            # Vite dev server

# Test
just test                    # all tests
just test-server             # Go server (race detector)
just test-agent              # Go agent (race detector)

# Quality
just fmt                     # format all Go files
just vet                     # go vet all Go files
just lint                    # golangci-lint

# Release notes
just release-notes           # structured draft from last tag
just release-notes-polished  # structured draft + AI summary
just release-notes from-tag=v1.0.0  # explicit range
```

### Key files

| File | Purpose |
|---|---|
| [CONTRIBUTING.md](../CONTRIBUTING.md) | Contributor guide: PR titles, label rules |
| [docs/maintainer-guidelines.md](maintainer-guidelines.md) | Maintainer guide: squash subjects, release process |
| [.github/pull_request_template.md](../.github/pull_request_template.md) | PR template |
| [.github/ISSUE_TEMPLATE/](../.github/ISSUE_TEMPLATE/) | Issue templates |
| [.github/release-drafter.yml](../.github/release-drafter.yml) | Release draft categories and version-resolver config |
| [scripts/release-notes.py](../scripts/release-notes.py) | Release note assembly script |
| [scripts/ai-polish.py](../scripts/ai-polish.py) | AI summary polish script |
| [justfile](../justfile) | All task runner recipes |
| [ROADMAP.md](../ROADMAP.md) | Planned work |

### Label taxonomy

```
type/feature      type/fix        type/performance
type/refactor     type/test       type/docs         type/chore

area/server       area/agent      area/frontend
area/proto        area/docs       area/ci

impact/breaking   impact/security   impact/ops

skip-changelog    (omit PR from release notes)
```

### Workflows

| Workflow | Trigger | What it does |
|---|---|---|
| `ci.yml` | push / PR | Tests, lint, build |
| `pr-labeler.yml` | PR opened/updated | Auto-applies `area/*` by file path |
| `pr-label-check.yml` | PR opened/updated | Enforces one `type/*` + one `area/*` (**required**) |
| `pr-title-hint.yml` | PR opened/updated | Non-blocking comment if title looks weak |
| `release-drafter.yml` | push to main / PR label change | Updates rolling draft release |
| `refresh-release-draft.yml` | manual | Enriches draft with release-note blocks + AI summary |
| `build-push.yml` | push to main / semver tag | Pushes Docker images to ghcr.io |
