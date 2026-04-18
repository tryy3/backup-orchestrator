package grpcclient

import (
	"context"
	"net"
	"testing"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/agent/internal/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// stubServer implements BackupServiceServer for testing.
// Connect reads messages from the agent and blocks until the stream closes.
type stubServer struct {
	backupv1.UnimplementedBackupServiceServer
	// connected is closed once the server has received the first message.
	connected chan struct{}
}

func (s *stubServer) Connect(stream grpc.BidiStreamingServer[backupv1.AgentMessage, backupv1.ServerMessage]) error {
	// Read the first message (heartbeat) and signal the test.
	if _, err := stream.Recv(); err != nil {
		return err
	}
	close(s.connected)

	// Block reading until the client disconnects.
	for {
		if _, err := stream.Recv(); err != nil {
			return err
		}
	}
}

// newBufconnClient sets up a bufconn-based gRPC server and returns a Client
// wired to it, plus the stubServer for coordination.
func newBufconnClient(t *testing.T) (*Client, *stubServer) {
	t.Helper()

	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	srv := &stubServer{connected: make(chan struct{})}
	gs := grpc.NewServer()
	backupv1.RegisterBackupServiceServer(gs, srv)

	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() { gs.Stop() })

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dialing bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	c := &Client{
		conn:   conn,
		client: backupv1.NewBackupServiceClient(conn),
	}
	return c, srv
}

// noopOnCommand is a test helper for the ctx-aware onCommand callback.
func noopOnCommand(_ context.Context, cmd *backupv1.Command) *backupv1.CommandResult {
	return &backupv1.CommandResult{CommandId: cmd.GetCommandId(), Success: true}
}

// TestRun_CancelUnblocksRecv verifies that cancelling the parent context
// causes Run to return promptly (within 500 ms), proving the recv goroutine
// is unblocked by the stream-scoped context cancellation.
func TestRun_CancelUnblocksRecv(t *testing.T) {
	client, srv := newBufconnClient(t)

	liveLogCh := make(chan *backupv1.LogEntry, 16)
	sh := NewStreamHandler(
		client,
		&identity.Identity{AgentID: "test-agent"},
		func(agentID, apiKey string) {},
		func(cfg *backupv1.AgentConfig) {},
		noopOnCommand,
		nil,
		liveLogCh,
	)

	ctx, cancel := context.WithCancel(context.Background())

	// Run in a goroutine.
	errCh := make(chan error, 1)
	go func() { errCh <- sh.Run(ctx) }()

	// Wait for the server to confirm the stream is established.
	select {
	case <-srv.connected:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for stream to connect")
	}

	// Cancel the parent context — this should cause Run to return quickly.
	cancel()

	select {
	case err := <-errCh:
		// Run returned; err should contain a context-related error.
		if err == nil {
			t.Fatal("expected non-nil error from Run after cancel")
		}
		t.Logf("Run returned with: %v", err)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not return within 500 ms after context cancel — recv goroutine likely stuck")
	}
}

// TestRun_ServerDisconnect verifies Run returns when the server closes the stream.
func TestRun_ServerDisconnect(t *testing.T) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	// A server that accepts one message then returns (closing the stream).
	srvImpl := &closeAfterFirstMsg{connected: make(chan struct{})}
	gs := grpc.NewServer()
	backupv1.RegisterBackupServiceServer(gs, srvImpl)

	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() { gs.Stop() })

	conn, err := grpc.NewClient(
		"passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dialing bufconn: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	c := &Client{
		conn:   conn,
		client: backupv1.NewBackupServiceClient(conn),
	}

	liveLogCh := make(chan *backupv1.LogEntry, 16)
	sh := NewStreamHandler(
		c,
		&identity.Identity{AgentID: "test-agent"},
		func(agentID, apiKey string) {},
		func(cfg *backupv1.AgentConfig) {},
		noopOnCommand,
		nil,
		liveLogCh,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- sh.Run(ctx) }()

	// Wait for stream to be established.
	select {
	case <-srvImpl.connected:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for stream to connect")
	}

	// Server handler returns, closing the stream.
	// Run should detect this promptly.
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected non-nil error from Run after server disconnect")
		}
		t.Logf("Run returned with: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return within 2 s after server disconnect")
	}
}

// closeAfterFirstMsg is a test server that closes the stream after the first message.
type closeAfterFirstMsg struct {
	backupv1.UnimplementedBackupServiceServer
	connected chan struct{}
}

func (s *closeAfterFirstMsg) Connect(stream grpc.BidiStreamingServer[backupv1.AgentMessage, backupv1.ServerMessage]) error {
	// Read the first message (initial heartbeat).
	if _, err := stream.Recv(); err != nil {
		return err
	}
	close(s.connected)

	// Return immediately — this closes the server side of the stream,
	// causing the client's Recv() to return an error.
	return nil
}

func TestCommandTimeout_Defaults(t *testing.T) {
	sh := NewStreamHandler(nil, &identity.Identity{}, nil, nil, nil, nil, nil)

	tests := []struct {
		name string
		cmd  *backupv1.Command
		want time.Duration
	}{
		{
			name: "trigger_backup gets long timeout",
			cmd: &backupv1.Command{
				Action: &backupv1.Command_TriggerBackup{TriggerBackup: &backupv1.TriggerBackup{}},
			},
			want: defaultBackupCommandTimeout,
		},
		{
			name: "trigger_restore gets long timeout",
			cmd: &backupv1.Command{
				Action: &backupv1.Command_TriggerRestore{TriggerRestore: &backupv1.TriggerRestore{}},
			},
			want: defaultRestoreCommandTimeout,
		},
		{
			name: "list_snapshots gets medium timeout",
			cmd: &backupv1.Command{
				Action: &backupv1.Command_ListSnapshots{ListSnapshots: &backupv1.ListSnapshots{}},
			},
			want: defaultListSnapshotsTimeout,
		},
		{
			name: "browse_snapshot gets medium timeout",
			cmd: &backupv1.Command{
				Action: &backupv1.Command_BrowseSnapshot{BrowseSnapshot: &backupv1.BrowseSnapshot{}},
			},
			want: defaultBrowseSnapshotTimeout,
		},
		{
			name: "browse_filesystem gets short timeout",
			cmd: &backupv1.Command{
				Action: &backupv1.Command_BrowseFilesystem{BrowseFilesystem: &backupv1.BrowseFilesystem{}},
			},
			want: defaultBrowseFSCommandTimeout,
		},
		{
			name: "unknown action gets default timeout",
			cmd:  &backupv1.Command{},
			want: defaultCommandTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sh.commandTimeout(tt.cmd)
			if got != tt.want {
				t.Errorf("commandTimeout() = %v, want %v", got, tt.want)
			}
		})
	}

	// Sanity: backup/restore must be strictly longer than browse timeouts, so
	// a short browse-kind default can't ever apply to a long-running backup.
	if defaultBackupCommandTimeout <= defaultBrowseFSCommandTimeout {
		t.Errorf("backup timeout (%v) must be longer than browse_fs timeout (%v)",
			defaultBackupCommandTimeout, defaultBrowseFSCommandTimeout)
	}
	if defaultRestoreCommandTimeout <= defaultBrowseFSCommandTimeout {
		t.Errorf("restore timeout (%v) must be longer than browse_fs timeout (%v)",
			defaultRestoreCommandTimeout, defaultBrowseFSCommandTimeout)
	}
}

// TestCommandTimeout_FromConfig verifies that handleConfig overrides the
// per-kind defaults with values from AgentConfig.CommandTimeouts.
func TestCommandTimeout_FromConfig(t *testing.T) {
	sh := NewStreamHandler(nil, &identity.Identity{}, nil, nil, nil, nil, nil)

	sh.applyCommandTimeouts(&backupv1.CommandTimeouts{
		BackupSecs:           120,
		RestoreSecs:          240,
		ListSnapshotsSecs:    60,
		BrowseSnapshotSecs:   30,
		BrowseFilesystemSecs: 5,
		DefaultSecs:          90,
	})

	cases := map[time.Duration]*backupv1.Command{
		120 * time.Second: {Action: &backupv1.Command_TriggerBackup{TriggerBackup: &backupv1.TriggerBackup{}}},
		240 * time.Second: {Action: &backupv1.Command_TriggerRestore{TriggerRestore: &backupv1.TriggerRestore{}}},
		60 * time.Second:  {Action: &backupv1.Command_ListSnapshots{ListSnapshots: &backupv1.ListSnapshots{}}},
		30 * time.Second:  {Action: &backupv1.Command_BrowseSnapshot{BrowseSnapshot: &backupv1.BrowseSnapshot{}}},
		5 * time.Second:   {Action: &backupv1.Command_BrowseFilesystem{BrowseFilesystem: &backupv1.BrowseFilesystem{}}},
		90 * time.Second:  {},
	}
	for want, cmd := range cases {
		if got := sh.commandTimeout(cmd); got != want {
			t.Errorf("commandTimeout() = %v, want %v", got, want)
		}
	}
}
