---
name: enrich-release-notes
description: >-
  Generate enriched release notes for a given version by collecting PR data from a GitHub release draft
  and producing a human-friendly summary with highlights and breaking change warnings.
  Use this skill whenever the user asks to generate, enrich, summarize, or draft release notes for a version,
  or asks "what's in this release", "summarize the release", "write release notes for v0.X.0", etc.
  Also use when the user mentions release notes, changelog generation, or version summaries.
  The user must provide a version number (e.g. v0.4.0) — if they haven't, ask for one before proceeding.
---

# Enrich Release Notes

Generate polished, user-facing release notes from the raw PR data collected for a GitHub release draft.

## Prerequisites

- `gh` CLI must be authenticated (used by the data collection script).
- Python 3 must be available.
- The repository must have a Release Drafter draft for the requested version.

## Workflow

### Step 1: Get the version number

The user **must** provide a version number (e.g. `v0.4.0`). If they haven't, ask:

> Which version should I generate release notes for? (e.g. `v0.4.0`)

Do not auto-detect a draft release. The data collection script requires an explicit version and will fail if that release does not exist or is not a draft.

### Step 2: Collect PR data

Run the collection script to fetch or refresh the PR data JSON:

```bash
python3 scripts/collect-release-prs.py <version>
```

This writes (or updates) `docs/version-drafts/<version>-pr-data.json`. The script validates that `<version>` is an existing draft release. It uses caching — it only fetches PRs that are new or changed since the last run.

If the JSON file already exists and the user hasn't asked to refresh, you can skip re-running the script and read the existing file directly.

### Step 3: Read and analyze the JSON

Read the resulting JSON file. The structure is:

```json
{
  "release": {
    "tag": "v0.5.0",
    "name": "v0.5.0",
    "isDraft": true,
    "body": "## What's Changed\n..."
  },
  "prs": [
    {
      "number": 157,
      "title": "feat: redesign agent report outbox ...",
      "url": "https://github.com/tryy3/backup-orchestrator/pull/157",
      "labels": ["type/feature", "area/agent", "area/server"],
      "mergedAt": "2026-04-10T...",
      "state": "MERGED",
      "releaseNote": "Added configurable agent outbox delivery settings...",
      "breakingChanges": null,
      "whatThisPrDoes": "Before this PR: ...\nAfter this PR: ...",
      "body": "<full PR body>"
    }
  ]
}
```

Key fields to pay attention to, in priority order:

1. **`releaseNote`** — The author's own summary of the user-facing change, extracted from the `` ```release-note `` fenced block in the PR template. This is the most curated signal. Give it significant weight. A `null` value means the author wrote `NONE` or left it empty, indicating a non-user-facing change.
2. **`breakingChanges`** — Extracted from the `### Breaking changes` section. Non-null means the author documented a breaking change.
3. **`labels`** — Look for `impact/breaking` (breaking change), `type/feature`, `type/bugfix`, `type/chore`, `type/docs`, `type/refactor`, `type/performance`, and `area/*` labels to categorize.
4. **`whatThisPrDoes`** — The before/after description from the PR template. Useful for understanding context when `releaseNote` is null.
5. **`title`** — The PR title, often in conventional commit format (`feat:`, `fix:`, `chore:`, etc.).
6. **`body`** — The full PR description. Use as a fallback for context, but prefer the extracted fields above.

### Step 4: Generate the enriched release notes

Produce markdown output with these sections:

#### 4a. Summary paragraph

Write 3-6 sentences describing the overall theme and direction of the release. What is this version about? What area of the project saw the most activity? This should read like a newsletter intro — informative and concise.

To figure out the theme, look at the distribution of labels and the nature of the PRs:
- If most PRs are `type/feature` with `area/agent` labels, the theme is agent improvements.
- If most are `type/chore` with `area/ci`, the theme is CI/infrastructure hardening.
- A mix might be "maintenance and infrastructure improvements with a notable new feature."

#### 4b. Highlights

List the most notable changes. These are PRs that have:
- A non-null `releaseNote` field (the author considered them user-facing).
- `type/feature` or `type/bugfix` labels.
- Significant scope (touching multiple areas, or representing a meaningful behavior change).

For each highlight, write a short bullet point. Use the `releaseNote` content as the primary source, supplemented by `whatThisPrDoes` for context. Link to the PR.

Dependency bumps (Renovate/Dependabot PRs) and pure CI/workflow changes are generally **not** highlights unless they have a user-visible effect. Group them into a brief "also includes" note if there are many.

#### 4c. Breaking Changes

This section is **only** included if there are actual breaking changes. Check for:
- PRs where `breakingChanges` is non-null.
- PRs with the `impact/breaking` label.
- Any mention of "action required" in the `releaseNote` field.

For each breaking change:
- Describe **what changed** clearly.
- Describe **what the user needs to do** (migration steps, config changes, etc.).
- Link to the PR for details.

If there are no breaking changes, omit this section entirely rather than writing "None."

#### 4d. Full changelog

Include a concise categorized list of all PRs, grouped by type label:
- Features
- Bug Fixes
- Maintenance / Chores
- Documentation
- Dependencies

Each entry is just the PR title linked to the PR URL. This mirrors the Release Drafter format but is cleaned up.

### Step 5: Present the output

Show the generated release notes to the user in the chat. The markdown should be ready to paste into the GitHub release draft body.

Remind the user to review and adjust the summary before publishing — the AI summary is a starting point, not a final version.

## Output Template

Here is the structure to follow:

```markdown
## v0.X.0 Release Notes

### Summary

[3-6 sentence overview of the release theme and direction.]

### Highlights

- **[Short title]** — [Description based on releaseNote field.] ([#NNN](url))
- **[Short title]** — [Description.] ([#NNN](url))

### Breaking Changes

> **Action required**: [description of what changed and what users must do.]
>
> See [#NNN](url) for details.

### What's Changed

#### Features
- [PR title] ([#NNN](url))

#### Bug Fixes
- [PR title] ([#NNN](url))

#### Maintenance
- [PR title] ([#NNN](url))

#### Dependencies
- [PR title] ([#NNN](url))

**Full Changelog**: [prev-tag]...[this-tag]
```

## Tips

- Don't invent information. If a PR description is sparse, describe what you can see from the title and labels, but don't fabricate details.
- Renovate/Dependabot PRs can be grouped into a single "Updated X dependencies" line in the Dependencies section to reduce noise.
- If the release is mostly maintenance/CI work, say so honestly in the summary rather than inflating the significance.
- Keep the tone professional but approachable. This is read by users deciding whether to upgrade.
