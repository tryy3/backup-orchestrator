# Maintainer Guidelines

This document is maintainer-facing guidance for repository workflow decisions and quality guardrails.

It is intended to grow as the project evolves (release workflow, CI policy, automation conventions, and AI-assisted process).

Guidance in this file is generally preferred practice, not CI-enforced policy, unless explicitly stated.

---

## Current Focus: Squash Merge Commit Subjects

This section describes how to shape squash merge commit subjects for clean history and better release notes.

Scope:

- Applies when merging PRs into `main` with squash merge.
- Guidance only. It is intentionally not enforced by CI.

## Why this exists

- Squash merge creates one commit per PR on `main`.
- That commit subject becomes high-signal release and history metadata.
- A light convention improves readability without adding contributor friction.

## Recommended squash commit subject format

Preferred pattern:

```text
<type>(<area>): <imperative summary>
```

Examples:

- `feat(agent): add backup scheduling support`
- `fix(server): prevent startup crash when config is missing`
- `perf(agent): reduce memory usage during snapshot listing`
- `docs(readme): clarify local dev setup`
- `chore(ci): update github-actions non-major dependencies`

Notes:

- `feat|fix|perf|refactor|docs|test|chore` are recommended prefixes.
- Use one primary area when possible (`server`, `agent`, `frontend`, `proto`, `ci`, `docs`).
- Keep subject under 72 characters when practical.
- Use imperative wording and avoid trailing periods.

## Mapping from PR labels

Use PR labels as the source of truth and map to the squash subject prefix:

- `type/feature` -> `feat`
- `type/fix` -> `fix`
- `type/performance` -> `perf`
- `type/refactor` -> `refactor`
- `type/docs` -> `docs`
- `type/test` -> `test`
- `type/chore` -> `chore`

Area mapping suggestion:

- `area/server` -> `server`
- `area/agent` -> `agent`
- `area/frontend` -> `frontend`
- `area/proto` -> `proto`
- `area/ci` -> `ci`
- `area/docs` -> `docs`

If multiple area labels are present, pick the primary one for the subject and keep cross-area detail in the PR body.

## Merge-time checklist for maintainers

1. Confirm labels are correct (`type/*` exactly one, `area/*` at least one).
2. Adjust PR title if needed so squash subject matches preferred format.
3. Ensure release-note block is present and accurate (`NONE` if internal only).
4. If `impact/breaking` is set, verify migration notes are explicit.

## Non-goals

- Do not require contributors to use Conventional Commit prefixes in normal branch commits.
- Do not block merges on subject format.
- Do not require strict scopes for every tiny typo-only change.

## Quick fallback

If a PR is tiny and speed matters, a plain imperative title is acceptable:

- `Fix typo in local setup docs`

Consistency is preferred, but low-friction shipping remains the priority.

---

## Cutting a Release

### Overview

A draft GitHub Release is maintained automatically by release-drafter as PRs are merged to `main`. When you are ready to ship a version, the process is:

1. Open [Releases](../../releases) and click **Edit** on the draft release.
2. Run the release assembly pipeline to enrich the draft with per-PR `release-note` block content and an AI-generated summary (see "Enriching the draft" below).
3. Review and adjust the release notes (see structure below).
4. Decide the version number using SemVer (release-drafter suggests one — see bump rules).
5. Set the tag in the **Tag version** field (e.g. `v1.2.0`). GitHub will create the tag on `main` when you publish.
6. Click **Publish release**.
7. `build-push.yml` triggers automatically on the new `v*` tag and pushes `:v1.2.0` and `:1.2` Docker images to ghcr.io.

### SemVer bump rules

Choose the version bump based on the highest-impact label present among all merged PRs since the last release:

| Highest-impact label | Bump |
|---|---|
| `impact/breaking` | **major** (x.0.0) |
| `type/feature` | **minor** (0.x.0) |
| anything else | **patch** (0.0.x) |

All three components (server, agent, frontend) share the same version tag.

### Release note structure

release-drafter groups PRs into categories in this order:

1. ⚠️ **Breaking Changes** — `impact/breaking` — always expand and add migration instructions
2. 🔒 **Security** — `impact/security`
3. ✨ **Features** — `type/feature`
4. 🐛 **Bug Fixes** — `type/fix`
5. ⚡ **Performance** — `type/performance`
6. 🔧 **Maintenance** — `type/chore`, `type/refactor`, `type/test`, `type/docs`

Categories with no PRs are hidden automatically. The draft also includes a full-changelog link at the bottom.

### Enriching the draft

The rolling draft from release-drafter contains only PR titles. CI rebuilds structured notes from PR `release-note` blocks automatically after Release Drafter runs. For the final AI-polished pass before publishing, use the `enrich-release-notes` skill locally.

**Via GitHub Actions (structured notes only):**

1. Go to Actions → **Refresh Release Draft** → Run workflow (or wait for the automatic run after Release Drafter).
2. Leave `from_tag` blank to auto-detect the last release tag.
3. Optionally set `draft_tag` when multiple drafts exist or auto-detection should target a specific version.
4. Optionally set `release_id` when GitHub exposes the draft as `untagged-*` and tag lookup is unreliable.
5. Re-open the draft after the run completes — it will be updated with structured `release-note` content.

**Locally (AI-polished notes via skill):**

1. Run `python3 scripts/collect-release-prs.py v0.X.0` (or `--release-id <id>`).
2. Invoke the `enrich-release-notes` skill to generate summary, highlights, and breaking-change sections.
3. Update the draft with `gh release edit <tag> --notes-file <file>` using the resolved `tag_name` from `python3 scripts/draft_release.py --version v0.X.0 --json`.

For a quick local structured draft without the skill:

```bash
just release-notes-polished          # structured notes + AI summary paragraph
just release-notes-polished from-tag=v1.0.0  # explicit range
```

The AI step calls GitHub Models (`gpt-4o`) and is only allowed to summarise what is in the input.

### Adjusting the draft before publishing

- **Edit or expand entries** — the auto-generated text comes from the PR `release-note` block; add more context if a change is significant.
- **Edit the AI summary** — the summary paragraph injected by ai-polish.py is a starting point; adjust tone or correct inaccuracies before publishing.
- **Exclude noise** — add `skip-changelog` to trivial PRs (e.g. minor Renovate bumps) and re-run the enrichment pipeline to remove them.

### Docker image tags after publish

| Event | Tags pushed |
|---|---|
| Push to `main` | `:latest`, `:main`, `:sha-XXXX` |
| Publish release `v1.2.0` | `:v1.2.0`, `:1.2`, `:latest`, `:sha-XXXX` |
