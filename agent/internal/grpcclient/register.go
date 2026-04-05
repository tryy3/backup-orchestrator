package grpcclient

import (
	"context"
	"fmt"
	"runtime"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

// Register calls the Register RPC to enroll this agent with the server.
func (c *Client) Register(ctx context.Context, hostname string) (*backupv1.RegisterResponse, error) {
	req := &backupv1.RegisterRequest{
		Hostname:      hostname,
		Os:            fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		AgentVersion:  "0.1.0",  // placeholder for MVP
		ResticVersion: "0.17.3", // placeholder for MVP
		RcloneVersion: "1.68.0", // placeholder for MVP
	}

	resp, err := c.client.Register(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("register RPC: %w", err)
	}
	return resp, nil
}
