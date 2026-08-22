---
name: gh-relevance-check
description: >-
  Evaluate whether a GitHub issue or pull request is still relevant, actionable, and worth keeping open.
  Use when the user invokes /gh-relevance-check, asks if an issue or PR is stale or outdated,
  or wants a summary of relevance assessments across open issues and PRs.
---

# GitHub Relevance Check

Determine whether issues or pull requests are still relevant to the current codebase.

## Prerequisites

- `gh` CLI authenticated.
- Read access to the repository.

```bash
export GH_PAGER=cat
export PAGER=cat
```

## Mode A: Check one item (default)

Use when the user provides an issue or PR number/URL, or says "is this still relevant?"

### Step 1: Load the item

```bash
# Issue
gh issue view <number> --json number,title,body,state,labels,comments,url

# Pull request
gh pr view <number> --json number,title,body,state,labels,comments,url,commits,files
```

### Step 2: Investigate

- Read title, body, and all comments.
- Check whether referenced files, packages, or APIs still exist in the codebase.
- Search recent merged PRs and commits for related fixes or implementations.
- Look for duplicate or superseding issues/PRs.

```bash
gh pr list --state merged --limit 30 --search "<keywords>"
gh issue list --state all --limit 20 --search "<keywords>"
```

### Step 3: Post analysis comment

Post **one** comment on the issue or PR:

```markdown
**Relevance Assessment: [Still Relevant | Likely Outdated | Needs Discussion]**

- **Summary**: <1-2 sentence verdict>
- **Evidence**:
  - <concrete finding with commit, PR, or file reference>
  - <another finding>
- **Recommendation**: <one of the following>
  - ✅ **Keep open** — still valid and actionable.
  - 🗄️ **Consider closing** — appears resolved or no longer applicable. <why>
  - 💬 **Needs maintainer input** — mixed signals; human should decide.
```

```bash
body_file="$(mktemp /tmp/gh-relevance-XXXXXX.md)"
# write body
gh issue comment <number> --body-file "$body_file"   # for issues
# or
gh pr comment <number> --body-file "$body_file"      # for PRs
rm -f "$body_file"
```

Do **not** close, label, or edit the item unless the user explicitly asks after seeing the assessment.

## Mode B: Summary report (optional)

Use when the user asks for a relevance summary across the repo (replaces the old `relevance-summary` workflow).

### Step 1: Find evaluated items

List open issues and PRs. For each, read comments and look for a prior **Relevance Assessment** comment containing:

- `**Relevance Assessment:**` followed by `Still Relevant`, `Likely Outdated`, or `Needs Discussion`
- A **Recommendation** with ✅ Keep open, 🗄️ Consider closing, or 💬 Needs maintainer input

### Step 2: Present summary

Output a markdown table in chat (do not create a GitHub issue unless the user asks):

```markdown
### Relevance Check Summary

**Generated:** YYYY-MM-DD

| # | Type | Title | Assessment | Recommendation |
|---|------|-------|------------|----------------|
| [#N](url) | Issue/PR | Brief title | ... | ... |

### Statistics
- Total evaluated: N
- Still Relevant: N
- Likely Outdated: N
- Needs Discussion: N
```

Sort by assessment: **Likely Outdated** first, then **Needs Discussion**, then **Still Relevant**.

If no items have been evaluated, say so clearly.

### Step 3: Create tracking issue (only if requested)

If the user wants a persistent summary issue:

```bash
gh issue create --title "[Relevance Summary] YYYY-MM-DD" --label report --body-file /tmp/relevance-summary.md
```

## Guidelines

- Be concise, factual, and cite specific commits, PRs, files, or code.
- Do not make repository changes beyond posting the assessment comment (unless creating a summary issue on request).
- Report results to the user with the item URL and verdict.

## Related skills

- `gh-triage-issue` — label and quality improvements for new or manual issues.
