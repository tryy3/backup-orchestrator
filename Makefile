.PHONY: proto-gen proto-lint build-frontend build-server build-agent build-all clean \
        docker-build docker-push docker-login

# Docker — GitHub Container Registry
# Auto-detected from git remote; override with: make docker-push GITHUB_REPO=youruser/yourrepo
GITHUB_REPO ?= $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*github\.com[:/]||;s|\.git$$||' | tr '[:upper:]' '[:lower:]')
REGISTRY    := ghcr.io
IMAGE_TAG   ?= latest

# Version metadata — injected at build time.
# For tagged releases: VERSION=v1.2.3; for dev builds: VERSION=dev
VERSION    ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "dev")
COMMIT     ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

SERVER_PKG := github.com/tryy3/backup-orchestrator/server/internal/version
AGENT_PKG  := github.com/tryy3/backup-orchestrator/agent/internal/version

LDFLAGS_SERVER := -X $(SERVER_PKG).Version=$(VERSION) \
                  -X $(SERVER_PKG).Commit=$(COMMIT) \
                  -X $(SERVER_PKG).BuildDate=$(BUILD_DATE)

LDFLAGS_AGENT  := -X $(AGENT_PKG).Version=$(VERSION) \
                  -X $(AGENT_PKG).Commit=$(COMMIT) \
                  -X $(AGENT_PKG).BuildDate=$(BUILD_DATE)

# Proto generation
proto-gen:
	cd proto && buf dep update && buf generate

proto-lint:
	cd proto && buf lint

# Frontend
build-frontend:
	cd frontend && VITE_APP_VERSION=$(VERSION) npm run build
	rm -rf server/internal/frontend/dist
	cp -r frontend/dist server/internal/frontend/dist

# Server (depends on frontend being built)
build-server: build-frontend
	cd server && go build -ldflags "$(LDFLAGS_SERVER)" -o ../bin/server ./cmd/server

# Agent
build-agent:
	cd agent && go build -ldflags "$(LDFLAGS_AGENT)" -o ../bin/agent ./cmd/agent

# All
build-all: build-server build-agent

# Dev: build server without rebuilding frontend
build-server-only:
	cd server && go build -ldflags "$(LDFLAGS_SERVER)" -o ../bin/server ./cmd/server

clean:
	rm -rf bin/
	rm -rf server/internal/frontend/dist

# Log in to GitHub Container Registry
# Usage: make docker-login GITHUB_USER=yourusername
docker-login:
	echo "Log in with a Personal Access Token (PAT) with 'write:packages' scope"
	docker login $(REGISTRY) -u $(GITHUB_USER) --password-stdin

# Build images locally (native platform only, loaded into local Docker)
docker-build:
	docker build -f docker/Dockerfile.server -t $(REGISTRY)/$(GITHUB_REPO)-server:$(IMAGE_TAG) .
	docker build -f docker/Dockerfile.agent  -t $(REGISTRY)/$(GITHUB_REPO)-agent:$(IMAGE_TAG)  .

# Build multi-arch images and push to ghcr.io
# Requires: docker buildx and being logged in (make docker-login)
docker-push:
	docker buildx build --platform linux/amd64,linux/arm64 \
		-f docker/Dockerfile.server \
		-t $(REGISTRY)/$(GITHUB_REPO)-server:$(IMAGE_TAG) \
		--push .
	docker buildx build --platform linux/amd64,linux/arm64 \
		-f docker/Dockerfile.agent \
		-t $(REGISTRY)/$(GITHUB_REPO)-agent:$(IMAGE_TAG) \
		--push .
