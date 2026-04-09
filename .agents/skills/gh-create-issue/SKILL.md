---
name: gh-create-issue
description: Use when user wants to create a GitHub issue for the current repository. Must read and follow the repository's issue template format.
---

# GitHub Create Issue

Use this skill when the user requests to create an issue. Must follow the repository's issue template format.

## Workflow

### Step 1: Determine Template Type

Analyze the user's request to determine the issue type:
- If the user describes a problem, error, crash, or something not working -> Bug Report
- If the user requests a new feature, enhancement, or additional support -> Feature Request
- If the user is asking a question or needs help with something -> Questions & Discussion
- Otherwise -> Others

**If unclear**, ask the user which template to use. Do not default to "Others" on your own.

### Step 2: Read the Selected Template

1. Read the corresponding template file from `.github/ISSUE_TEMPLATE/` directory.
2. Identify required fields (`validations.required: true`), title prefix (`title`), and labels (`labels`, if present).

### Step 3: Collect Information

Based on the selected template, ask the user for required information only. Follow the template's required fields and option constraints (for example, Platform and Priority choices).

### Step 4: Build and Preview Issue Content

Write the issue body using the `create_file` tool to a fixed path such as `/tmp/gh-issue-body.md`. Do NOT use `mktemp` + heredoc — the variable is lost between terminal calls and the heredoc echoes content noisily in interactive shells.

- Use the exact title prefix from the selected template.
- Fill content following the template body structure and section order.
- Apply labels exactly as defined by the template.
- Keep all labels when there are multiple labels.
- If template has no labels, do not add custom labels.

Preview the file content in chat and ask for confirmation before creating. **Skip this step if the user explicitly indicates no preview/confirmation is needed** (for example, automation workflows).

### Step 5: Create Issue

Use `gh issue create` command to create the issue. Run the create and cleanup together in a single terminal call so the file path is not lost between invocations:

```bash
gh issue create --title "<title_with_template_prefix>" --body-file /tmp/gh-issue-body.md && rm -f /tmp/gh-issue-body.md
```

If the selected template includes labels, append one `--label` per label:

```bash
gh issue create --title "<title_with_template_prefix>" --body-file /tmp/gh-issue-body.md --label "<label_1_from_template>" --label "<label_2_from_template>" && rm -f /tmp/gh-issue-body.md
```

If the selected template has no labels, do not pass `--label`.

Use the `--web` flag to open the creation page in browser when complex formatting is needed:

```bash
gh issue create --web
```

## Notes

- Must read template files under `.github/ISSUE_TEMPLATE/` to ensure following the correct format.
- Treat template files as the only source of truth. Do not hardcode title prefixes or labels in this skill.
- Title must be clear and concise, avoid vague terms like "a suggestion" or "stuck".
- Provide as much detail as possible to help developers understand and resolve the issue.
- If user doesn't specify a template type, ask them to choose one first.
