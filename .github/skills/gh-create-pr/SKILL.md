---
name: gh-create-pr
description: Create or update GitHub pull requests using the repository-required workflow and template compliance. Use when asked to create/open/update a PR so the assistant reads `.github/pull_request_template.md`, fills every template section, preserves markdown structure exactly, and marks missing data as N/A or None instead of skipping sections.
---

# GitHub PR Creation

## Workflow

1. Read `.github/pull_request_template.md` before drafting the PR body.
2. Collect PR context from the current branch (base/head, scope, linked issues, testing status, breaking changes, release note content).
3. Check if the current branch has been pushed to remote. If not, push it first:
   - Default remote is `origin`, but ask the user if they want to use a different remote.
   ```bash
   git push -u <remote> <head-branch>
   ```
4. Determine the base branch:
   - Default base is `main`. Ask the user to confirm if a different base is needed.
   - Check available remotes with `git remote -v` if unclear.
5. Create a temp file and write the PR body:
   - Use `pr_body_file="$(mktemp /tmp/gh-pr-body-XXXXXX).md"`
   - Fill content using the template structure exactly (keep section order, headings, checkbox formatting).
   - If not applicable, write `N/A` or `None`.
6. Preview the temp file content. **Show the file path** (e.g., `/tmp/gh-pr-body-XXXXXX.md`) and ask for explicit confirmation before creating. **Skip this step if the user explicitly indicates no preview/confirmation is needed** (for example, automation workflows).
7. After confirmation, create the PR:
   ```bash
   gh pr create --base <base> --head <head> --title "<title>" --body-file "$pr_body_file"
   ```
8. Clean up the temp file: `rm -f "$pr_body_file"`
9. Report the created PR URL and summarize title/base/head and any required follow-up.

## Constraints

- Never skip template sections.
- Never rewrite the template format.
- Keep content concise and specific to the current change set.
- PR title and body must be written in English.
- Never create the PR before showing the full final body to the user, unless they explicitly waive the preview or confirmation.
- Never rely on command permission prompts as PR body preview.
- **Labels are required by branch protection.** Every PR must have:
  - Exactly one `type/*` label (`type/feature`, `type/fix`, `type/docs`, `type/chore`, `type/refactor`, `type/performance`, `type/test`)
  - At least one `area/*` label (`area/server`, `area/agent`, `area/frontend`, `area/proto`, `area/docs`, `area/ci`)
  - Optional `impact/*` labels (`impact/breaking`, `impact/security`, `impact/ops`) when relevant
  - Use `--label` for each label when calling `gh pr create`
- **Release note & Documentation checkbox** — both are driven by whether the change is **user-facing**. Use the table below:

  | Change type | `type/*` label | Release note | Docs `[x]` |
  |---|---|---|---|
  | New user-facing feature | `type/feature` | Describe the change | ✅ |
  | Bug fix visible to users | `type/fix` | Describe the fix | ✅ if behavior changed |
  | Behavior change / breaking | `type/fix` or `type/feature` + `impact/breaking` | Describe + `action required` | ✅ |
  | Security fix | any + `impact/security` | Describe the fix | ✅ if usage changed |
  | Performance improvement | `type/performance` | Describe if user-visible | ❌ usually |
  | Internal refactoring | `type/refactor` | `NONE` | ❌ |
  | Tests only | `type/test` | `NONE` | ❌ |
  | Docs only | `type/docs` | `NONE` (or brief note) | ✅ |
  | CI / build tooling | `type/chore` + `area/ci` | `NONE` | ❌ |
  | Dependency bump | `type/chore` | `NONE` | ❌ |

## Command Pattern

```bash
# read template
cat .github/pull_request_template.md

# show this full Markdown body in chat first
pr_body_file="$(mktemp /tmp/gh-pr-body-XXXXXX).md"
cat > "$pr_body_file" <<'EOF'
...filled template body...
EOF

# run only after explicit user confirmation
# always include at least one --label type/* and one --label area/*
gh pr create --base <base> --head <head> --title "<title>" --body-file "$pr_body_file" \
  --label "type/feature" --label "area/server"
rm -f "$pr_body_file"
```