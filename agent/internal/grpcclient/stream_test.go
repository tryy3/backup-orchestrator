package grpcclient

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/agent/internal/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

// ---------------------------------------------------------------------------
// Mock stream helpers (for send-serialisation tests)
// ---------------------------------------------------------------------------

// mockConnectStream is a fake bidirectional stream used for testing.
// It tracks how many goroutines call Send concurrently to detect races.
type mockConnectStream struct {
	grpc.ClientStream

	recvCh chan *backupv1.ServerMessage // test feeds messages here
	sentMu sync.Mutex
	sent   []*backupv1.AgentMessage

	// concurrentSends is incremented on Send entry, decremented on exit.
	// If it ever exceeds 1, there is a concurrent send.
	concurrentSends atomic.Int32
	maxConcurrent   atomic.Int32

	closeSendCalled atomic.Bool
}

func newMockStream() *mockConnectStream {
	return &mockConnectStream{
		recvCh: make(chan *backupv1.ServerMessage, 64),
	}
}

func (m *mockConnectStream) Send(msg *backupv1.AgentMessage) error {
	cur := m.concurrentSends.Add(1)
	defer m.concurrentSends.Add(-1)
	for {
		old := m.maxConcurrent.Load()
		if cur <= old || m.maxConcurrent.CompareAndSwap(old, cur) {
			break
		}
	}
	// Simulate a tiny amount of work so concurrent calls overlap.
	time.Sleep(time.Millisecond)
	m.sentMu.Lock()
	m.sent = append(m.sent, msg)
	m.sentMu.Unlock()
	return nil
}

func (m *mockConnectStream) Recv() (*backupv1.ServerMessage, error) {
	msg, ok := <-m.recvCh
	if !ok {
		return nil, io.EOF
	}
	return msg, nil
}

func (m *mockConnectStream) Header() (metadata.MD, error) { return nil, nil }
func (m *mockConnectStream) Trailer() metadata.MD         { return nil }
func (m *mockConnectStream) CloseSend() error {
	m.closeSendCalled.Store(true)
	return nil
}
func (m *mockConnectStream) Context() context.Context { return context.Background() }
func (m *mockConnectStream) SendMsg(any) error        { return nil }
func (m *mockConnectStream) RecvMsg(any) error        { return nil }

func (m *mockConnectStream) getSent() []*backupv1.AgentMessage {
	m.sentMu.Lock()
	defer m.sentMu.Unlock()
	cp := make([]*backupv1.AgentMessage, len(m.sent))
	copy(cp, m.sent)
	return cp
}

// mockBackupServiceClient is a fake gRPC client that returns our mock stream.
type mockBackupServiceClient struct {
	backupv1.BackupServiceClient
	stream *mockConnectStream
}

func (m *mockBackupServiceClient) Connect(_ context.Context, _ ...grpc.CallOption) (grpc.BidiStreamingClient[backupv1.AgentMessage, backupv1.ServerMessage], error) {
	return m.stream, nil
}

// ---------------------------------------------------------------------------
// bufconn helpers (for integration tests from main)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Send serialisation tests (mock-based)
// ---------------------------------------------------------------------------

// TestSendSerialisation verifies that all stream.Send calls are serialised
// through the single send goroutine, even when config pushes, commands, and
// heartbeats happen concurrently. Running with -race detects any violation.
func TestSendSerialisation(t *testing.T) {
	ms := newMockStream()
	mockClient := &mockBackupServiceClient{stream: ms}

	// Wire up a StreamHandler that uses our mock client.
	liveLogCh := make(chan *backupv1.LogEntry, 64)
	id := &identity.Identity{AgentID: "test-agent", APIKey: "test-key"}
	sh := &StreamHandler{
		client: &Client{
			client: mockClient,
		},
		identity:          id,
		heartbeatInterval: 50 * time.Millisecond, // fast heartbeats for the test
		liveLogCh:         liveLogCh,
		onConfig: func(_ *backupv1.AgentConfig) {
			// Simulate some work.
			time.Sleep(time.Millisecond)
		},
		onCommand: func(_ context.Context, cmd *backupv1.Command) *backupv1.CommandResult {
			return &backupv1.CommandResult{
				CommandId: cmd.GetCommandId(),
				Success:   true,
			}
		},
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- sh.Run(ctx)
	}()

	// Give the send goroutine time to start and send the initial heartbeat.
	time.Sleep(20 * time.Millisecond)

	// Fire several config pushes and commands concurrently with heartbeats.
	const n = 20
	for i := range n {
		if i%2 == 0 {
			ms.recvCh <- &backupv1.ServerMessage{
				Payload: &backupv1.ServerMessage_Config{
					Config: &backupv1.AgentConfig{
						ConfigVersion: 1,
					},
				},
			}
		} else {
			ms.recvCh <- &backupv1.ServerMessage{
				Payload: &backupv1.ServerMessage_Command{
					Command: &backupv1.Command{
						CommandId: "cmd-test",
					},
				},
			}
		}
	}

	// Also pump some live log entries.
	for range 5 {
		liveLogCh <- &backupv1.LogEntry{
			Message:    "test log",
			Attributes: `{"job_id":"job-1"}`,
		}
	}

	// Let everything process.
	time.Sleep(300 * time.Millisecond)

	// Shut down the stream.
	close(ms.recvCh)
	cancel()

	err := <-errCh
	if err != nil && err != context.Canceled {
		t.Logf("Run returned (expected): %v", err)
	}

	// Verify that concurrent sends never exceeded 1.
	if max := ms.maxConcurrent.Load(); max > 1 {
		t.Fatalf("detected %d concurrent Send calls; expected at most 1", max)
	}

	// Verify we received outbound messages (heartbeats + config acks + command results + live logs).
	sent := ms.getSent()
	if len(sent) == 0 {
		t.Fatal("expected at least some sent messages")
	}

	// Count message types.
	var heartbeats, configAcks, commandResults, liveLogs int
	for _, msg := range sent {
		switch msg.GetPayload().(type) {
		case *backupv1.AgentMessage_Heartbeat:
			heartbeats++
		case *backupv1.AgentMessage_ConfigAck:
			configAcks++
		case *backupv1.AgentMessage_CommandResult:
			commandResults++
		case *backupv1.AgentMessage_LiveLogs:
			liveLogs++
		}
	}

	// We should have at least some heartbeats (initial + ticker).
	if heartbeats < 1 {
		t.Errorf("expected at least 1 heartbeat, got %d", heartbeats)
	}

	// We sent 10 config messages.
	if configAcks != 10 {
		t.Errorf("expected 10 config acks, got %d", configAcks)
	}

	// We sent 10 command messages.
	if commandResults != 10 {
		t.Errorf("expected 10 command results, got %d", commandResults)
	}

	// We sent 5 log entries; should have at least 1 live logs batch.
	if liveLogs < 1 {
		t.Errorf("expected at least 1 live logs batch, got %d", liveLogs)
	}

	t.Logf("messages: heartbeats=%d configAcks=%d commandResults=%d liveLogs=%d",
		heartbeats, configAcks, commandResults, liveLogs)
}

// TestHandleConfigEnqueuesAck verifies handleConfig enqueues a config ack
// on the outbound channel rather than calling stream.Send directly.
func TestHandleConfigEnqueuesAck(t *testing.T) {
	id := &identity.Identity{AgentID: "agent-1", APIKey: "key-1"}
	sh := &StreamHandler{
		identity:          id,
		heartbeatInterval: 30 * time.Second,
	}

	outboundCh := make(chan *backupv1.AgentMessage, 8)
	cfg := &backupv1.AgentConfig{
		ConfigVersion:         42,
		HeartbeatIntervalSecs: 10,
	}

	sh.handleConfig(outboundCh, cfg)

	select {
	case msg := <-outboundCh:
		ack := msg.GetConfigAck()
		if ack == nil {
			t.Fatal("expected ConfigAck payload")
		}
		if ack.ConfigVersion != 42 {
			t.Errorf("config version: got %d, want %d", ack.ConfigVersion, 42)
		}
		if !ack.Success {
			t.Error("expected success=true")
		}
		if msg.AgentId != "agent-1" {
			t.Errorf("agent_id: got %q, want %q", msg.AgentId, "agent-1")
		}
	default:
		t.Fatal("expected a message on outboundCh")
	}

	// Verify heartbeat interval was updated.
	sh.heartbeatMu.RLock()
	interval := sh.heartbeatInterval
	sh.heartbeatMu.RUnlock()
	if interval != 10*time.Second {
		t.Errorf("heartbeat interval: got %v, want %v", interval, 10*time.Second)
	}
}

// TestHandleCommandEnqueuesResult verifies handleCommand enqueues a command
// result on the outbound channel.
func TestHandleCommandEnqueuesResult(t *testing.T) {
	id := &identity.Identity{AgentID: "agent-2", APIKey: "key-2"}
	sh := &StreamHandler{
		identity:          id,
		heartbeatInterval: 30 * time.Second,
		onCommand: func(_ context.Context, cmd *backupv1.Command) *backupv1.CommandResult {
			return &backupv1.CommandResult{
				CommandId: cmd.GetCommandId(),
				Success:   true,
			}
		},
	}

	outboundCh := make(chan *backupv1.AgentMessage, 8)
	cmd := &backupv1.Command{CommandId: "cmd-99"}

	sh.handleCommand(context.Background(), outboundCh, cmd)

	select {
	case msg := <-outboundCh:
		result := msg.GetCommandResult()
		if result == nil {
			t.Fatal("expected CommandResult payload")
		}
		if result.CommandId != "cmd-99" {
			t.Errorf("command_id: got %q, want %q", result.CommandId, "cmd-99")
		}
		if !result.Success {
			t.Error("expected success=true")
		}
		if msg.AgentId != "agent-2" {
			t.Errorf("agent_id: got %q, want %q", msg.AgentId, "agent-2")
		}
	default:
		t.Fatal("expected a message on outboundCh")
	}
}

// ---------------------------------------------------------------------------
// Integration tests (bufconn-based, from main)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Per-command timeout tests
// ---------------------------------------------------------------------------

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
