package grpcclient

import (
	"context"
	"fmt"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

// ReportJob sends a completed job report to the server.
func (c *Client) ReportJob(ctx context.Context, report *backupv1.JobReport) error {
	ack, err := c.client.ReportJob(ctx, report)
	if err != nil {
		return fmt.Errorf("report job RPC: %w", err)
	}
	if !ack.GetSuccess() {
		return fmt.Errorf("server rejected job report: %s", ack.GetError())
	}
	return nil
}

// ReportSnapshots sends a snapshot report to the server.
func (c *Client) ReportSnapshots(ctx context.Context, report *backupv1.SnapshotReport) error {
	_, err := c.client.ReportSnapshots(ctx, report)
	if err != nil {
		return fmt.Errorf("report snapshots RPC: %w", err)
	}
	return nil
}
