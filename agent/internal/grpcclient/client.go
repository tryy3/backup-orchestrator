package grpcclient

import (
	"fmt"

	"github.com/tryy3/backup-orchestrator/agent/internal/config"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Client wraps the gRPC connection and service client.
type Client struct {
	conn   *grpc.ClientConn
	client backupv1.BackupServiceClient
	cfg    *config.Config
}

// New dials the gRPC server and returns a connected Client.
// Uses insecure transport for MVP development (Tailscale provides encryption).
func New(cfg *config.Config) (*Client, error) {
	conn, err := grpc.NewClient(
		cfg.ServerURL,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("dialing server at %s: %w", cfg.ServerURL, err)
	}

	return &Client{
		conn:   conn,
		client: backupv1.NewBackupServiceClient(conn),
		cfg:    cfg,
	}, nil
}

// Close closes the gRPC connection.
func (c *Client) Close() {
	if c.conn != nil {
		c.conn.Close()
	}
}

// ServiceClient returns the underlying BackupServiceClient for direct use.
func (c *Client) ServiceClient() backupv1.BackupServiceClient {
	return c.client
}
