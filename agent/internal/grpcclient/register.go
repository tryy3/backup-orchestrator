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
		AgentVersion:  AgentVersion,
		ResticVersion: ResticVersion,
		RcloneVersion: RcloneVersion,
	}

	resp, err := c.client.Register(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("register RPC: %w", err)
	}
	return resp, nil
}
