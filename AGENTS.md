# AGENTS.md

## Project Overview

Backup Orchestrator is a monorepo for a restic-based backup platform:

- Server: Go service exposing REST (Chi), gRPC, SQLite, and embedded frontend assets.
- Agent: Go daemon running on backup hosts, executes restic/rclone, schedules jobs, and streams status over gRPC.
- Frontend: Vue 3 + TypeScript + Vite + Pinia SPA.
- Proto: Shared protobuf contracts generated into both Go modules.

Primary design constraints:

- Agents connect outbound to server (server is the only inbound endpoint).
- Agent must keep operating on last-known config when server is unavailable.
- No ORM; use raw SQL through `database/sql`.
- SQLite driver is `modernc.org/sqlite` (pure Go, no CGO).

## Monorepo Layout

- `server/`: Go module for API, gRPC server, config push, DB.
- `agent/`: Go module for scheduler, restic executor, reporting.
- `frontend/`: Vue app.
- `proto/`: `.proto` schemas and buf config.
- `docs/`: architecture, API, schema, workflow docs.
- `docker/`: Dockerfiles and compose setup.

When editing code, scope changes to one module unless cross-module updates are required.

## Required Tooling

- Go 1.26.x (see `server/go.mod`, `agent/go.mod`).
- Node.js 22+.
- npm.
- just (task runner).
- buf (for proto generation/lint).
- golangci-lint (for lint recipes).
- lefthook (pre-commit hooks).

Recommended local workflow is Nix + direnv:

```bash
direnv allow
nix develop
```

## Setup Commands

From repository root:

```bash
# frontend deps
cd frontend && npm ci

# install git hooks
cd .. && lefthook install

# inspect available tasks
just --list
```

Local defaults are in `.env.dev`. Override with `.env.dev.local`.

## Development Workflow

Use root `just` recipes whenever possible:

```bash
# run each service independently
just dev-server
just dev-agent
just dev-frontend

# or run all in zellij tabs
just dev

# stop dev processes if needed
just dev-stop
```

Notes:

- `just dev-server` sets `BACKUP_DB_PATH` to `tmp/server.db`.
- `just dev-agent` sets `BACKUP_DATA_DIR` to `tmp/agent-data`.
- Frontend dev server proxies `/api` to server on localhost:8080.

## Build Instructions

```bash
# full build (frontend + server + agent)
just build

# targeted builds
just build-frontend
just build-server
just build-server-only
just build-agent
```

Build outputs:

- Binaries: `bin/server`, `bin/agent`.
- Frontend dist: `frontend/dist` and copied to `server/internal/frontend/dist`.

Version metadata (`Version`, `Commit`, `BuildDate`) is injected via ldflags in `justfile`.

## Testing Instructions

Run from repo root:

```bash
# all tests
just test

# per module
just test-server
just test-agent
just test-frontend

# coverage helpers (Go modules)
just test-cover
just test-cover-server
just test-cover-agent
```

Direct module-level commands:

```bash
cd server && go test -race ./...
cd agent && go test -race ./...
cd frontend && npm test
```

Focused testing examples:

```bash
# run one Go test function
cd server && go test ./... -run TestName

# run one frontend test by name
cd frontend && npx vitest run -t "test name"
```

Test conventions:

- Go tests are colocated as `*_test.go`.
- Frontend uses Vitest (`frontend/src/**` and `frontend/src/test/**`).
- Add or update tests for behavior changes.

## Code Style and Conventions

### Go

- Format with `gofmt` (use `just fmt`).
- Vet with `go vet` (use `just vet`).
- Lint with `golangci-lint` (use `just lint`).
- Keep packages under `internal/` unless explicitly public.
- Return wrapped errors (`fmt.Errorf("context: %w", err)`), do not panic in normal flow.
- Use `github.com/google/uuid` for entity IDs.
- Keep SQL explicit; avoid introducing ORMs.

### Frontend

- Use Vue 3 SFCs with `<script setup lang="ts">`.
- Keep API calls in `frontend/src/api/client.ts` (no direct fetch in components).
- Keep shared API types in `frontend/src/types/api.ts`.
- Use Pinia stores in `frontend/src/stores/`.
- Styling is Tailwind-based.

### Proto

- Regenerate and lint on schema changes:

```bash
just proto-gen
just proto-lint
just proto-breaking
```

## CI and Required Checks

Relevant workflows are in `.github/workflows/`:

- `ci.yml`: tests, fmt/vet, lint (PR), proto checks (PR), build.
- `pr-label-check.yml`: enforces PR labels.
- `release-drafter.yml`: maintains rolling draft release.
- `build-push.yml`: builds and pushes images on main and tags.

Before opening a PR, at minimum run:

```bash
just test
just fmt
just vet
just lint
```

If proto changed, also run:

```bash
just proto-gen
just proto-lint
just proto-breaking
```

## Pull Request Rules

Follow `CONTRIBUTING.md` and `.github/pull_request_template.md`.

Required label policy (enforced by CI):

- Exactly one `type/*` label.
- At least one `area/*` label.
- Optional `impact/*` labels when relevant.

Release notes:

- Fill the `release-note` fenced block in the PR template.
- Use `NONE` for non-user-facing work.

PR title guidance:

- Use short, imperative, specific titles.
- Keep around 72 chars or less when possible.
- Squash merge is used; PR title becomes merge commit subject.

## Security and Operations Notes

- Do not commit secrets.
- Ensure `BACKUP_ENCRYPTION_KEY` handling remains compatible with current server behavior.
- Be careful with command execution paths and hook/script inputs.
- Avoid logging sensitive repository credentials.
- For production changes, consider impact on:
  - agent offline operation,
  - reconnect/report replay,
  - backward compatibility of proto and DB behavior.

## Docker and Release Operations

```bash
just docker-build
just docker-push
```

Release note generation:

```bash
just release-notes
just release-notes-polished
```

The workflow `refresh-release-draft.yml` can refresh the draft release body from PR `release-note` blocks.

## Debugging and Troubleshooting

- Check the task graph first: `just --list`.
- If generated code is stale, run `just proto-gen`.
- If server build fails due to missing embedded assets, run `just build-frontend` (or create `server/internal/frontend/dist` stub as CI does).
- If hooks fail locally, reinstall: `lefthook install`.
- Keep fixes minimal and localized; avoid opportunistic refactors in bugfix PRs.

## Agent Working Agreement for This Repo

When acting as an automated coding agent in this repository:

1. Prefer root `just` recipes over ad-hoc commands.
2. Keep changes focused; avoid touching unrelated modules.
3. Update tests and docs with behavior changes.
4. Preserve established architecture decisions listed in docs.
5. Run relevant tests/linters before finalizing.
6. Include migration notes when introducing breaking behavior.

## Reference Docs

- `README.md`
- `CLAUDE.md`
- `CONTRIBUTING.md`
- `docs/workflow.md`
- `docs/architecture-overview.md`
- `docs/grpc-api.md`
- `docs/database-schema.md`
- `docs/maintainer-guidelines.md`