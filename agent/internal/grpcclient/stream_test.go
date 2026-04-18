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
		func(cmd *backupv1.Command) *backupv1.CommandResult {
			return &backupv1.CommandResult{CommandId: cmd.GetCommandId(), Success: true}
		},
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
		func(cmd *backupv1.Command) *backupv1.CommandResult {
			return &backupv1.CommandResult{CommandId: cmd.GetCommandId(), Success: true}
		},
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
