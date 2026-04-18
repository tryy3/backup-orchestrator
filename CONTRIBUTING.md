# Contributing to Backup Orchestrator

Thanks for contributing.

This is still an early-stage project and, at the moment, mostly maintained by the repository owner. The goal of this guide is to keep contribution rules simple, predictable, and easy to automate.

This file is the contributor-facing reference for:

- pull request expectations,
- labels and release metadata,
- basic development workflow,
- and rules that can later be reused in AI prompts, skills, or repository automation.

For ongoing maintainer planning and rollout details, see [docs/release-workflow-plan.md](docs/release-workflow-plan.md).

## Before You Start

Before opening a PR:

1. Check existing issues and docs to avoid duplicate work.
2. Keep changes focused on one main purpose.
3. Prefer small, reviewable PRs over large mixed changes.

If you are making a larger change, design change, or release-process change, open an issue first when practical.

## Development Setup

Project setup and commands are described in [README.md](README.md). For the full walkthrough — Nix dev shell, git hooks, running the stack locally, and the complete PR → merge → release lifecycle — see [docs/workflow.md](docs/workflow.md).

Common commands:

```bash
just build
just test
just fmt
just vet
just lint
just proto-gen
```

## Pull Request Guidelines

PRs should be easy to review and easy to summarize in a release.

### General expectations

1. Keep the PR description clear and complete.
2. Explain what changed and why.
3. Mention tradeoffs and alternatives when relevant.
4. Include issue links when applicable.
5. Add or update tests when behavior changes.
6. Update docs when user-facing behavior changes.

### PR labels

Each PR must have:

1. Exactly one `type/*` label.
2. At least one `area/*` label.
3. Optional `impact/*` labels when they improve release context.

Current label groups:

- Type:
  - `type/feature`
  - `type/fix`
  - `type/docs`
  - `type/chore`
  - `type/refactor`
  - `type/performance`
  - `type/test`
- Area:
  - `area/server`
  - `area/agent`
  - `area/frontend`
  - `area/proto`
  - `area/docs`
  - `area/ci`
- Impact:
  - `impact/breaking`
  - `impact/security`
  - `impact/ops`

Area labels may be combined. Type labels may not.

If a PR touches multiple parts of the monorepo, add multiple `area/*` labels.

### How to choose the type label

Pick the label that best matches the primary reason the PR exists.

1. Use `type/feature` for new user-visible capabilities.
2. Use `type/fix` for bug fixes or incorrect behavior corrections.
3. Use `type/performance` when the main outcome is better speed or lower resource usage.
4. Use `type/refactor` for internal restructuring without intended behavior change.
5. Use `type/test` for test-focused PRs.
6. Use `type/docs` for documentation-focused PRs.
7. Use `type/chore` for maintenance, tooling, automation, or dependency work.

Examples:

- A new feature that also adds docs and tests is still `type/feature`.
- A refactor whose main goal is performance is `type/performance`.
- A readability cleanup with no behavior change is `type/refactor`.

### PR title guidance

PR titles should be short, specific, and action-oriented.

Preferred style (guidance only):

```text
Add backup scheduling support to agent
Fix crash when server is unreachable during startup
Improve restic snapshot listing performance
Update golangci-lint to v2.1
```

Rules of thumb:

1. Start with an imperative verb: `Add`, `Fix`, `Remove`, `Update`, `Refactor`, `Improve`, `Support`.
2. Keep under 72 characters.
3. No trailing period.
4. Be specific enough to understand without opening the diff.

Avoid vague titles such as `Fix bug`, `changes`, or `WIP: ...`.

Why this matters: PRs are merged with squash-only, so the PR title becomes the merge commit subject on `main`.

Conventional Commit prefixes (`feat:`, `fix:`) are optional and allowed, but not required.

## Release Notes

The PR template includes a `release-note` block.

Use it like this:

1. Write a short user-facing summary when the PR changes behavior, UX, or capabilities.
2. Write `NONE` when the change is internal only, such as tooling, refactoring, CI, or maintenance work.
3. If users must take action after upgrade, make that explicit.

The goal is to make release drafting easier and more accurate.

The `release-note` block is the primary source for release notes. When a release is assembled, `scripts/release-notes.py` extracts these blocks and groups them by label category. The quality of this field directly affects release note quality.

## Code and Review Expectations

When contributing code:

1. Keep changes minimal and focused.
2. Preserve existing project structure and conventions.
3. Avoid unrelated refactors in the same PR unless they are necessary.
4. Prefer root-cause fixes over surface-level patches.

Reviewers will primarily look for:

1. Behavior regressions.
2. Missing tests.
3. Unclear release impact.
4. Changes that are larger or broader than necessary.

## Issues

Use the issue templates when opening bugs, feature requests, or questions.

Helpful issue reports usually include:

1. The affected component (`agent`, `server`, `frontend`, `proto`, `docs`, `CI`).
2. The version, commit SHA, branch, or image tag.
3. Reproduction steps.
4. Relevant logs or command output.

## AI-Assisted Contributions

AI-generated changes should follow the same rules as manual contributions.

If you use AI tools, make sure the resulting PR still has:

1. A correct `type/*` label.
2. Appropriate `area/*` labels.
3. A useful PR description.
4. A correct `release-note` block.
5. Human review before merge.

## Questions

If a contribution rule is unclear, prefer the simpler interpretation and document the assumption in the PR.

As the project evolves, this file should stay short and practical. Deeper maintainer rollout details belong in [docs/release-workflow-plan.md](docs/release-workflow-plan.md).