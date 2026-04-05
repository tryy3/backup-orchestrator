.PHONY: proto-gen proto-lint build-frontend build-server build-agent build-all clean \
        docker-build docker-push docker-login

# Docker — GitHub Container Registry
# Auto-detected from git remote; override with: make docker-push GITHUB_REPO=youruser/yourrepo
GITHUB_REPO ?= $(shell git remote get-url origin 2>/dev/null | sed -E 's|.*github\.com[:/]||;s|\.git$$||' | tr '[:upper:]' '[:lower:]')
REGISTRY    := ghcr.io
IMAGE_TAG   ?= latest

# Proto generation
proto-gen:
	cd proto && buf dep update && buf generate

proto-lint:
	cd proto && buf lint

# Frontend
build-frontend:
	cd frontend && npm run build
	rm -rf server/internal/frontend/dist
	cp -r frontend/dist server/internal/frontend/dist

# Server (depends on frontend being built)
build-server: build-frontend
	cd server && go build -o ../bin/server ./cmd/server

# Agent
build-agent:
	cd agent && go build -o ../bin/agent ./cmd/agent

# All
build-all: build-server build-agent

# Dev: build server without rebuilding frontend
build-server-only:
	cd server && go build -o ../bin/server ./cmd/server

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
