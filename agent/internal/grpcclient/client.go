package grpcclient

import (
	"fmt"
	"time"

	"github.com/tryy3/backup-orchestrator/agent/internal/config"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/agent/internal/version"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

// AgentVersion holds the agent binary version from build-time metadata.
// ResticVersion and RcloneVersion report installed tool versions; they default
// to "unknown" until runtime detection is implemented.
var (
	AgentVersion  = version.Version
	ResticVersion = "unknown"
	RcloneVersion = "unknown"
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
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second, // ping if no activity for 30 s
			Timeout:             10 * time.Second, // wait 10 s for ping ack
			PermitWithoutStream: true,             // keep probing even when idle
		}),
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
		_ = c.conn.Close()
	}
}

// ServiceClient returns the underlying BackupServiceClient for direct use.
func (c *Client) ServiceClient() backupv1.BackupServiceClient {
	return c.client
}
