// Package version holds build-time version metadata for the agent.
// The variables are set via -ldflags at build time.
package version

// Version, Commit, and BuildDate are injected at build time via:
//
//	go build -ldflags "-X github.com/tryy3/backup-orchestrator/agent/internal/version.Version=v1.0.0 \
//	  -X github.com/tryy3/backup-orchestrator/agent/internal/version.Commit=$(git rev-parse --short HEAD) \
//	  -X github.com/tryy3/backup-orchestrator/agent/internal/version.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)
