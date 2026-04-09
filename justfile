# Backup Orchestrator — Task Runner
# https://github.com/casey/just

# Docker — GitHub Container Registry
# Auto-detected from git remote; override with: just docker-push github-repo=youruser/yourrepo
github-repo := `git remote get-url origin 2>/dev/null | sed -E 's|.*github\.com[:/]||;s|\.git$||' | tr '[:upper:]' '[:lower:]'`
registry    := "ghcr.io"
image-tag   := "latest"

# Version metadata — injected at build time.
# For tagged releases: VERSION=v1.2.3; for dev builds: VERSION=dev
version    := `git describe --tags --exact-match 2>/dev/null || echo "dev"`
commit     := `git rev-parse --short HEAD 2>/dev/null || echo "unknown"`
build-date := `date -u +%Y-%m-%dT%H:%M:%SZ`

server-pkg := "github.com/tryy3/backup-orchestrator/server/internal/version"
agent-pkg  := "github.com/tryy3/backup-orchestrator/agent/internal/version"

# List available recipes
default:
    @just --list

# ── Tests ─────────────────────────────────────────────────────────────────────

# Run server tests with race detector
test-server:
    cd server && go test -race ./...

# Run agent tests with race detector
test-agent:
    cd agent && go test -race ./...

# Run frontend tests
test-frontend:
    cd frontend && npm test

# Run all tests
test: test-server test-agent test-frontend

# Run server tests with coverage report
test-cover-server:
    cd server && go test -race -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html

# Run agent tests with coverage report
test-cover-agent:
    cd agent && go test -race -coverprofile=coverage.out ./... && go tool cover -html=coverage.out -o coverage.html

# Run all tests with coverage
test-cover: test-cover-server test-cover-agent

# ── Formatting ────────────────────────────────────────────────────────────────

# Format server Go files
fmt-server:
    cd server && gofmt -l -w .

# Format agent Go files
fmt-agent:
    cd agent && gofmt -l -w .

# Format all Go files
fmt: fmt-server fmt-agent

# ── Vet ───────────────────────────────────────────────────────────────────────

# Vet server Go code
vet-server:
    cd server && go vet ./...

# Vet agent Go code
vet-agent:
    cd agent && go vet ./...

# Vet all Go code
vet: vet-server vet-agent

# ── Lint ──────────────────────────────────────────────────────────────────────

# Lint server Go code with golangci-lint
lint-server:
    cd server && golangci-lint run ./...

# Lint agent Go code with golangci-lint
lint-agent:
    cd agent && golangci-lint run ./...

# Lint all Go code
lint: lint-server lint-agent

# Auto-fix lint issues in server
lint-fix-server:
    cd server && golangci-lint run --fix ./...

# Auto-fix lint issues in agent
lint-fix-agent:
    cd agent && golangci-lint run --fix ./...

# Auto-fix lint issues in all Go code
lint-fix: lint-fix-server lint-fix-agent

# ── Build ─────────────────────────────────────────────────────────────────────

# Build frontend (outputs to frontend/dist and copies to server/internal/frontend/dist)
build-frontend:
    cd frontend && VITE_APP_VERSION={{version}} npm run build
    rm -rf server/internal/frontend/dist
    cp -r frontend/dist server/internal/frontend/dist

# Build server binary (depends on frontend)
build-server: build-frontend
    cd server && go build \
        -ldflags "-X {{server-pkg}}.Version={{version}} -X {{server-pkg}}.Commit={{commit}} -X {{server-pkg}}.BuildDate={{build-date}}" \
        -o ../bin/server ./cmd/server

# Build server binary only (skip frontend rebuild)
build-server-only:
    cd server && go build \
        -ldflags "-X {{server-pkg}}.Version={{version}} -X {{server-pkg}}.Commit={{commit}} -X {{server-pkg}}.BuildDate={{build-date}}" \
        -o ../bin/server ./cmd/server

# Build agent binary
build-agent:
    cd agent && go build \
        -ldflags "-X {{agent-pkg}}.Version={{version}} -X {{agent-pkg}}.Commit={{commit}} -X {{agent-pkg}}.BuildDate={{build-date}}" \
        -o ../bin/agent ./cmd/agent

# Build all binaries
build: build-server build-agent

# ── Proto ─────────────────────────────────────────────────────────────────────

# Regenerate proto files
proto-gen:
    cd proto && buf dep update && buf generate

# Lint proto files
proto-lint:
    cd proto && buf lint

# Check proto for breaking changes against main branch
proto-breaking:
    cd proto && buf breaking --against '.git#branch=main'

# ── Clean ─────────────────────────────────────────────────────────────────────

# Remove build artifacts
clean:
    rm -rf bin/
    rm -rf server/internal/frontend/dist
    rm -rf server/coverage.out server/coverage.html
    rm -rf agent/coverage.out agent/coverage.html

# ── Docker ────────────────────────────────────────────────────────────────────

# Log in to GitHub Container Registry
# Usage: just docker-login github-user=yourusername
docker-login github-user="":
    @echo "Log in with a Personal Access Token (PAT) with 'write:packages' scope"
    docker login {{registry}} -u {{github-user}} --password-stdin

# Build images locally (native platform only, loaded into local Docker)
docker-build:
    docker build -f docker/Dockerfile.server -t {{registry}}/{{github-repo}}-server:{{image-tag}} .
    docker build -f docker/Dockerfile.agent  -t {{registry}}/{{github-repo}}-agent:{{image-tag}}  .

# Build multi-arch images and push to ghcr.io (requires docker buildx and docker-login)
[confirm("Push multi-arch images to ghcr.io? This requires docker buildx and you must be logged in.")]
docker-push:
    docker buildx build --platform linux/amd64,linux/arm64 \
        -f docker/Dockerfile.server \
        -t {{registry}}/{{github-repo}}-server:{{image-tag}} \
        --push .
    docker buildx build --platform linux/amd64,linux/arm64 \
        -f docker/Dockerfile.agent \
        -t {{registry}}/{{github-repo}}-agent:{{image-tag}} \
        --push .
