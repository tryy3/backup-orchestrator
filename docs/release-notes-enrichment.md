# Release Notes Enrichment

Design document for enriching Release Drafter output with AI-generated summaries and breaking change highlights.

## Background

Release Drafter (`.github/workflows/release-drafter.yml`) already runs on every push to `main` and produces a well-structured draft release grouped by label category (Features, Bug Fixes, Maintenance, etc.). This is the source of truth for *which* PRs belong to the next release.

The enrichment layer adds:

1. **AI-generated summary** — a short paragraph describing the overall theme of the release (e.g. "major frontend redesign", "new backup scheduling engine").
2. **Breaking changes section** — extracted from PR descriptions that mention breaking changes or carry the `impact/breaking` label.
3. **Per-PR release notes** — the `release-note` fenced block from each PR description.

## Architecture

```
Release Drafter draft
        │
        ▼
┌─────────────────────┐
│ collect-release-prs  │  Script 1: fetch draft → parse PR refs → fetch PR bodies → JSON
│   (Python, gh CLI)   │
└────────┬────────────┘
         │  pr-data.json
         ▼
┌─────────────────────┐
│ SKILL.md / AI agent  │  Reads JSON, produces enriched release notes
│   (Copilot skill)    │
└─────────────────────┘
         │
         ▼
   Enriched markdown
   (summary + breaking changes + per-PR notes)
```

### Step 1: Data Collection (`scripts/collect-release-prs.py`)

Input: a version string (e.g. `v0.4.0`) for an existing draft release.

Process:

1. Run `gh release view <version> --json body,tagName,name,isDraft` to get the release body.
2. Parse the body for PR references (`#123` patterns).
3. For each PR number, run `gh pr view <number> --json number,title,body,labels,url,mergedAt,state` to get the full description.
4. Extract from each PR body:
   - The `release-note` fenced block content.
   - The "Breaking changes" section content.
   - The "What this PR does" section content.
5. Write a JSON file with all collected data.

Output JSON structure:

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
      "releaseNote": "Added configurable agent outbox delivery settings...",
      "breakingChanges": null,
      "whatThisPrDoes": "Before this PR: ...\nAfter this PR: ...",
      "body": "<full PR body>"
    }
  ]
}
```

### Step 2: AI Summary Generation (SKILL.md)

A Copilot skill (`/.github/skills/enrich-release-notes/SKILL.md`) that:

1. Reads the JSON file produced by step 1.
2. Generates:
   - **Summary paragraph** — 3-6 sentences describing the overall release theme.
   - **Breaking Changes** — dedicated section if any PR has breaking changes. Each breaking change should describe what changed and what action the user needs to take.
   - **Highlights** — notable features or fixes worth calling out.
3. Outputs enriched markdown that can be pasted into the release draft body.

## PR Description Convention

PRs follow the template in `.github/pull_request_template.md`. Key sections the scripts look for:

- `### What this PR does` — before/after description.
- `### Breaking changes` — migration notes when applicable.
- `` ```release-note `` — short user-facing summary (or `NONE` for internal changes).

## Usage

```bash
# Collect PR data from a specific release
python3 scripts/collect-release-prs.py v0.4.0

# Output to a specific file
python3 scripts/collect-release-prs.py v0.4.0 --output pr-data.json
```

Then invoke the Copilot skill to generate enriched notes from the JSON.

## Relationship to Existing Scripts

- `scripts/release-notes.py` — collects PRs by searching git history since a tag. Used by the `refresh-release-draft` agentic workflow. The new `collect-release-prs.py` takes a different approach: it starts from the Release Drafter draft itself, which is already the curated list of PRs for the release.
- `scripts/ai-polish.py` — calls GitHub Models API for a summary paragraph. The SKILL.md approach replaces this with Copilot-driven summarization that has access to full PR context (not just the categorized list).

Both existing scripts remain usable independently. The new workflow is an alternative path that produces richer output.
