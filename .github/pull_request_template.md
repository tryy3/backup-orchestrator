<!-- Template from https://github.com/kubevirt/kubevirt/blob/main/.github/PULL_REQUEST_TEMPLATE.md?-->

### What this PR does

PR title reminder (non-blocking): use a short, imperative, specific title. With squash merge, this title becomes the merge commit subject.

Optional maintainer hint for squash subject:

Suggested squash subject: <!-- e.g. feat(agent): add backup scheduling support -->

Reference: [docs/maintainer-guidelines.md](docs/maintainer-guidelines.md)

Before this PR:

After this PR:

<!-- (optional, in `fixes #<issue number>(, fixes #<issue_number>, ...)` format, will close the issue(s) when PR gets merged)*: -->

Fixes #

### Why we need it and why it was done in this way

The following tradeoffs were made:

The following alternatives were considered:

Links to places where the discussion took place: <!-- optional: slack, other GH issue, mailinglist, ... -->

### Breaking changes

<!-- optional -->

If this PR introduces breaking changes, please describe the changes and the impact on users.

### Special notes for your reviewer

<!-- optional -->

### Labels

Please ensure this PR has:

- exactly one `type/*` label
- one or more `area/*` labels when relevant
- optional `impact/*` labels when they help release context

Type label guidance:

- choose the label that best matches the main reason this PR exists
- if a feature PR also includes docs/tests/refactors, keep `type/feature`
- if the main goal is efficiency, prefer `type/performance` over `type/refactor`

Reference: [docs/release-workflow-plan.md](docs/release-workflow-plan.md)

### Checklist

This checklist is not enforcing, but it's a reminder of items that could be relevant to every PR.
Approvers are expected to review this list.

- [ ] PR: The PR description is expressive enough and will help future contributors
- [ ] Code: [Write code that humans can understand](https://en.wikiquote.org/wiki/Martin_Fowler#code-for-humans) and [Keep it simple](https://en.wikipedia.org/wiki/KISS_principle)
- [ ] Refactor: You have [left the code cleaner than you found it (Boy Scout Rule)](https://learning.oreilly.com/library/view/97-things-every/9780596809515/ch08.html)
- [ ] Upgrade: Impact of this change on upgrade flows was considered and addressed if required
- [ ] Documentation: A [user-guide update](https://docs.cherry-ai.com) was considered and is present (link) or not required. Check this only when the PR introduces or changes a user-facing feature or behavior.
- [ ] Self-review: I have reviewed my own code (e.g., via [`/gh-pr-review`](/.claude/skills/gh-pr-review/SKILL.md), `gh pr diff`, or GitHub UI) before requesting review from others
- [ ] Labels: This PR has exactly one `type/*` label and appropriate `area/*` labels

### Release note

<!--  Write your release note:
1. Enter your extended release note in the below block. If the PR requires additional action from users switching to the new release, include the string "action required".
2. If no release note is required, just write "NONE".
3. Only include user-facing changes (new features, bug fixes visible to users, UI changes, behavior changes). For CI, maintenance, internal refactoring, build tooling, or other non-user-facing work, write "NONE".
-->

```release-note

```