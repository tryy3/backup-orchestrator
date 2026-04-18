package grpcclient

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/agent/internal/identity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

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
		onCommand: func(cmd *backupv1.Command) *backupv1.CommandResult {
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
		onCommand: func(cmd *backupv1.Command) *backupv1.CommandResult {
			return &backupv1.CommandResult{
				CommandId: cmd.GetCommandId(),
				Success:   true,
			}
		},
	}

	outboundCh := make(chan *backupv1.AgentMessage, 8)
	cmd := &backupv1.Command{CommandId: "cmd-99"}

	sh.handleCommand(outboundCh, cmd)

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
