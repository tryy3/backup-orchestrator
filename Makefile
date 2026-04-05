.PHONY: proto-gen proto-lint build-frontend build-server build-agent build-all clean

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
