# Release Workflow Options for Backup Orchestrator

## Context

This project currently has:

- A PR template with a dedicated release-note section: [.github/pull_request_template.md](../.github/pull_request_template.md)
- CI and image build workflows, including tag-aware image tagging: [.github/workflows/ci.yml](../.github/workflows/ci.yml), [.github/workflows/build-push.yml](../.github/workflows/build-push.yml)
- No current GoReleaser configuration file

The goal is to support:

- Multiple parallel PRs/issues being merged over time
- Reliable release changelog generation at release time
- Good user-facing summaries linked to relevant PRs/issues
- Low-friction handling for minor changes (for example docs-only updates)

---

## Option 1: GitHub Native Release Notes + Labels

### How it works

1. Use labels consistently on PRs (for example: feature, bug, docs, chore, breaking).
2. When cutting a release in GitHub, use auto-generate release notes.
3. GitHub compiles merged PRs and contributors, usually grouped by labels.
4. Maintainer edits the generated draft and publishes.

### What to standardize

- A label taxonomy (for example: area/server, area/agent, area/frontend + type/feature, type/fix, type/docs).
- PR title conventions so generated text is readable.
- A lightweight policy for when to manually improve generated notes.

### Pros

- Lowest setup effort.
- Familiar workflow for most contributors.
- Strong native linking to PRs and issues.

### Cons

- Output quality depends on PR title/description quality.
- User-facing narrative often still needs manual editing.
- Version bump decisions are still manual.

### Best fit

Best first step when you want immediate improvements with minimal process changes.

---

## Option 2: Conventional Commits + Release Please

### How it works

1. Enforce commit/PR type semantics (feat, fix, docs, chore, refactor, perf).
2. Release Please continuously analyzes merged commits.
3. It opens a release PR containing:
   - Proposed version bump
   - Changelog entries
   - Release metadata updates
4. Merging the release PR creates tag + release.

### What to standardize

- Commit or squash-merge title format (Conventional Commits).
- Mapping between commit types and changelog categories.
- Policy for breaking changes (for example via ! or explicit footer).

### Pros

- Strong automation for semver and releases.
- Scales well with many parallel PRs.
- Reduces release-day manual work significantly.

### Cons

- Requires discipline around commit semantics.
- Generated changelogs can be too mechanical without extra curation.
- May require contributor education and CI checks.

### Best fit

Best for teams that want standardized, mostly automated release management.

---

## Option 3: Changesets (PR-authored release fragments)

### How it works

1. Meaningful PRs add a small markdown changeset file describing user-facing change + bump intent.
2. Changeset tooling aggregates pending notes across merged work.
3. At release, tool opens/updates a release PR with:
   - Combined changelog text
   - Version bumps
4. Minor internal changes can be skipped or marked as no-release-note based on policy.

### What to standardize

- When a changeset is required (for example: all user-facing changes).
- How to classify package/component impact in a monorepo.
- Review guidelines for note quality.

### Pros

- High-quality release notes because text is authored close to the change.
- Good fit for many concurrent PRs.
- Clear intent from contributors on release impact.

### Cons

- Adds contributor overhead on each substantial PR.
- Slightly less common in pure Go projects compared to JS ecosystems.
- Needs policy discipline to avoid missing fragments.

### Best fit

Best when note quality and maintainability of release narratives matter most.

---

## Option 4: PR Release-Note Block + Aggregator + AI Summarization

### How it works

1. Keep the existing PR template release-note block as the canonical source.
2. Add a script/workflow that collects release-note blocks from merged PRs since last tag.
3. Build a draft release body grouped by categories (feature/fix/docs/internal).
4. Feed aggregated notes + PR metadata + linked issues to AI for a polished summary.
5. Maintainer reviews and publishes release.

### What to standardize

- Clear rules for release-note block content:
  - NONE for non-user-facing changes
  - Required concise, user-facing text for meaningful changes
- Optional PR labels that help grouping.
- A release script command (for example via just) to prepare draft notes.

### Pros

- Minimal disruption because your template already includes release-note section.
- Supports quick simple PRs and larger feature work in the same flow.
- AI can improve readability while preserving traceability to source PRs.

### Cons

- Requires building and maintaining aggregation logic.
- If PRs omit or poorly fill release-note blocks, quality drops.
- Still needs human review before publish.

### Best fit

Best near-term fit for this repository because it builds on existing conventions instead of replacing them.

---

## Option 5: GoReleaser Changelog (with or without custom notes)

### How it works

1. Introduce a .goreleaser.yaml config.
2. Use GoReleaser changelog configuration to collect commits since last tag.
3. Group by commit patterns (for example feat/fix/docs).
4. Generate release notes and publish artifacts in one release pipeline.

### What to standardize

- Commit message patterns or conventional commit adoption.
- Changelog grouping rules in .goreleaser.yaml.
- Artifact publishing and signing strategy (if needed later).

### Pros

- Strong integrated release automation for Go binaries/artifacts.
- Popular and well-documented in Go ecosystem.
- Good long-term foundation if you want full release orchestration in one tool.

### Cons

- Introduces larger migration/setup effort than options above.
- Commit-only changelog is often less polished unless complemented with human or AI curation.
- Needs careful rollout with existing build/push workflows.

### Best fit

Best if you want end-to-end release automation and are ready for moderate migration effort.

---

## Comparison Summary

| Option | Setup Effort | Ongoing Overhead | Changelog Quality | Automation Level | AI-Friendly |
|---|---|---|---|---|---|
| GitHub Native + Labels | Low | Low | Medium | Medium | Medium |
| Conventional + Release Please | Medium | Medium | Medium-High | High | High |
| Changesets | Medium | Medium-High | High | High | High |
| PR Block + Aggregator + AI | Medium | Medium | High (with review) | Medium-High | Very High |
| GoReleaser Changelog | Medium-High | Medium | Medium | High | Medium-High |

---

## Recommended path for this repository

### Phase 1 (quick win)

- Tighten PR release-note expectations in [.github/pull_request_template.md](../.github/pull_request_template.md)
- Add and enforce PR labels for grouping
- Use GitHub generated notes + manual edit at release

### Phase 2 (high value, low disruption)

- Implement Option 4 aggregation from PR release-note blocks
- Add a just recipe to generate a release draft from last tag to HEAD
- Add AI-assisted summarization step for final release body

### Phase 3 (optional evolution)

- Evaluate either Release Please or GoReleaser depending on whether you prioritize:
  - PR-driven semver automation (Release Please), or
  - Full artifact-centric release orchestration (GoReleaser)

---

## Deep-dive areas for Option 4 + Changesets

This section captures areas that likely need focused follow-up design and implementation work.

### 1) PR label strategy and automation

Why it matters:

- Labels become the backbone for grouping and release summaries.

What to design:

- A clear label taxonomy (for example type/*, area/*, impact/*).
- Rules for required labels per PR.
- Rules for conflicts (for example type/docs should not coexist with type/feature unless intentional).

Automation options:

- GitHub native:
  - Auto-label by changed paths using actions/labeler.
  - Enforce required labels with a PR check.
- AI assisted:
  - Comment-only recommendation bot that suggests labels.
  - Optional auto-apply mode gated by maintainer review.

### 2) PR and commit naming policy

Why it matters:

- Consistent titles improve changelog quality and searchability.

What to design:

- PR title convention (for example feat(server): add streaming retries).
- Commit convention for squash merges.
- Breaking-change markers and guidance.

Automation options:

- PR title lint in CI (for example semantic-pull-request).
- Commit lint if merge strategy relies on commit history.
- Branch protection requiring the policy check to pass.

### 3) Changesets policy (source of detailed release blocks)

Why it matters:

- Changesets can carry richer, structured, user-facing notes than raw commits.

What to design:

- When a changeset is mandatory versus optional.
- Template for writing clear user-facing notes.
- Mapping from changeset types to release sections.
- Handling of docs/chore/internal-only changes.

Automation options:

- CI check: fail PR when user-facing changes have no changeset.
- PR bot comment with actionable guidance when missing.
- AI workflow: generate draft changeset text and request author confirmation.

### 4) Source-of-truth hierarchy for release notes

Why it matters:

- You will have multiple potential note sources (PR release-note block, labels, changesets).

What to design:

- Priority order, for example:
  1. Changeset text
  2. PR release-note block
  3. PR title fallback
- Rules for deduplication and conflict resolution.
- Rules for aggregating linked issues and related PRs.

### 5) Release assembly pipeline

Why it matters:

- Option 4 relies on deterministic collection and formatting before AI polishing.

What to design:

- Input window (from last tag to HEAD, or release branch only).
- Grouping format (features, fixes, docs, internal).
- Inclusion/exclusion rules (reverts, release chores, dependency bumps).

Automation options:

- A reproducible script invoked from just.
- A workflow_dispatch GitHub action that generates a draft release body artifact.
- Human approval step before publish.

### 6) AI role boundaries and safety

Why it matters:

- AI should polish and summarize, but not silently invent facts.

What to design:

- Inputs AI is allowed to use (changesets, PR descriptions, merged diff summaries).
- Required output format and section structure.
- A verification checklist for maintainers.

Automation options:

- Generate both:
  - traceable source list (links), and
  - polished summary
- Add a no-hallucination policy: every claim must map to a source PR/issue.

### 7) Templates and contributor UX

Why it matters:

- Good templates reduce inconsistency and release-day cleanup.

What to design:

- PR template wording for release-note and changeset expectations.
- Issue templates that capture release-relevant metadata (component, severity, impact).
- Quick examples of good and bad release text.

Automation options:

- PR checklist item confirming release metadata completeness.
- Saved replies or bots that guide contributors when fields are missing.

### 8) Governance, cadence, and ownership

Why it matters:

- Process quality drops quickly without clear ownership.

What to design:

- Who owns release draft generation.
- Who approves final notes.
- Release cadence and cut criteria.
- Hotfix/backport release-note handling.

### 9) Measurement and continuous improvement

Why it matters:

- You need feedback loops to know if the process is actually better.

What to measure:

- Percent of merged PRs with complete labels.
- Percent of user-facing PRs with changesets.
- Release prep time before/after rollout.
- Number of post-release note corrections.

---

## Suggested near-term implementation sequence (without GoReleaser)

1. Finalize label taxonomy and add label automation.
2. Enforce PR title convention and add a clear policy doc.
3. Introduce Changesets with a small contributor guide.
4. Add CI checks for missing changesets and required labels.
5. Build a release-draft generator that aggregates from changesets first.
6. Add AI summarization as a final polishing step with source traceability.
7. Run 2-3 releases and review metrics before deciding on GoReleaser adoption.

---

## Practical guardrails regardless of option

- Keep a strict distinction between user-facing and internal changes.
- Always include links to key PRs and issues in published release notes.
- Require explicit handling of breaking changes.
- Keep a maintainer review checkpoint before publishing releases.

---

## Related repository issues

- https://github.com/tryy3/backup-orchestrator/issues/94
- https://github.com/tryy3/backup-orchestrator/issues/125

---

## Execution tracking

- Living implementation plan: [docs/release-workflow-plan.md](release-workflow-plan.md)
