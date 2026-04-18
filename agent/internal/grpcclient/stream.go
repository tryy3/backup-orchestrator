package grpcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/agent/internal/identity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// JobStatus represents the current job state for heartbeat reporting.
type JobStatus struct {
	PlanName        string
	StartedAt       time.Time
	ProgressPercent float32 // 0-100, or -1 if unknown
}

// JobStatusFunc returns the current running job status, or nil if idle.
type JobStatusFunc func() *JobStatus

// outboundChSize is the buffer size for the channel that serialises all
// outbound stream.Send calls. 32 matches the server-side send channel and
// provides enough headroom for bursts of config acks / command results.
const outboundChSize = 32

// StreamHandler manages the bidirectional Connect stream lifecycle.
type StreamHandler struct {
	client            *Client
	identity          *identity.Identity
	identityMu        sync.RWMutex
	onApproval        func(agentID, apiKey string)
	onConfig          func(cfg *backupv1.AgentConfig)
	onCommand         func(cmd *backupv1.Command) *backupv1.CommandResult
	jobStatusFn       JobStatusFunc
	liveLogCh         <-chan *backupv1.LogEntry // receives live log entries from running jobs
	heartbeatInterval time.Duration
	heartbeatMu       sync.RWMutex
}

// NewStreamHandler creates a new StreamHandler.
func NewStreamHandler(
	client *Client,
	id *identity.Identity,
	onApproval func(agentID, apiKey string),
	onConfig func(cfg *backupv1.AgentConfig),
	onCommand func(cmd *backupv1.Command) *backupv1.CommandResult,
	jobStatusFn JobStatusFunc,
	liveLogCh <-chan *backupv1.LogEntry,
) *StreamHandler {
	return &StreamHandler{
		client:            client,
		identity:          id,
		onApproval:        onApproval,
		onConfig:          onConfig,
		onCommand:         onCommand,
		jobStatusFn:       jobStatusFn,
		liveLogCh:         liveLogCh,
		heartbeatInterval: 30 * time.Second,
	}
}

// Run opens the Connect stream, sends heartbeats, and dispatches server messages.
// It returns when the stream disconnects or the context is cancelled.
// The caller is responsible for reconnection with exponential backoff.
func (s *StreamHandler) Run(ctx context.Context) error {
	stream, err := s.client.client.Connect(ctx)
	if err != nil {
		return fmt.Errorf("opening connect stream: %w", err)
	}

	// Send initial heartbeat immediately.
	if err := s.sendHeartbeat(stream); err != nil {
		return fmt.Errorf("sending initial heartbeat: %w", err)
	}

	// Derived context so we can signal both goroutines to stop when Run exits.
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	// outboundCh funnels all messages from the recv goroutine (config acks,
	// command results) into the single send goroutine so that stream.Send is
	// never called concurrently.
	outboundCh := make(chan *backupv1.AgentMessage, outboundChSize)

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	// handlerWg tracks in-flight handler goroutines (handleConfig,
	// handleCommand) so we can wait for them before closing the stream.
	var handlerWg sync.WaitGroup

	// Start send goroutine: owns all stream.Send calls.
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.heartbeatMu.RLock()
		interval := s.heartbeatInterval
		s.heartbeatMu.RUnlock()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		// Pending log entries waiting to be batched and sent.
		var pendingLogs []*backupv1.LogEntry
		var pendingJobID string

		// flushLogs sends any accumulated log entries as a LiveLogs message.
		flushLogs := func() {
			if len(pendingLogs) == 0 {
				return
			}
			if err := s.sendLiveLogs(stream, pendingJobID, pendingLogs); err != nil {
				slog.Error("error sending live logs", "source", "stream", "error", err)
			}
			pendingLogs = nil
		}

		// logFlushTicker batches log entries so we don't send per-entry.
		logFlushTicker := time.NewTicker(2 * time.Second)
		defer logFlushTicker.Stop()

		for {
			select {
			case <-runCtx.Done():
				flushLogs()
				// Drain remaining outbound messages from in-flight handlers.
				for msg := range outboundCh {
					if err := stream.Send(msg); err != nil {
						slog.Error("error sending outbound message during drain", "source", "stream", "error", err)
					}
				}
				// Close the send side of the stream.
				_ = stream.CloseSend()
				return
			case msg, ok := <-outboundCh:
				if !ok {
					// Channel closed during shutdown; already drained above.
					return
				}
				if err := stream.Send(msg); err != nil {
					slog.Error("error sending outbound message", "source", "stream", "error", err)
				}
			case <-ticker.C:
				if err := s.sendHeartbeat(stream); err != nil {
					errCh <- fmt.Errorf("sending heartbeat: %w", err)
					return
				}
			case entry, ok := <-s.liveLogCh:
				if !ok {
					flushLogs()
					return
				}
				// Extract job_id from attributes if available.
				if jobID := extractJobID(entry); jobID != "" {
					pendingJobID = jobID
				}
				pendingLogs = append(pendingLogs, entry)
			case <-logFlushTicker.C:
				flushLogs()
			}
		}
	}()

	// Start recv goroutine: dispatch server messages.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			msg, err := stream.Recv()
			if err != nil {
				errCh <- fmt.Errorf("receiving server message: %w", err)
				return
			}

			switch payload := msg.GetPayload().(type) {
			case *backupv1.ServerMessage_Approval:
				s.handleApproval(payload.Approval)

			case *backupv1.ServerMessage_Config:
				handlerWg.Add(1)
				go func() {
					defer handlerWg.Done()
					s.handleConfig(outboundCh, payload.Config)
				}()

			case *backupv1.ServerMessage_Command:
				handlerWg.Add(1)
				go func() {
					defer handlerWg.Done()
					s.handleCommand(outboundCh, payload.Command)
				}()

			default:
				slog.Warn("unknown server message type", "source", "stream", "type", fmt.Sprintf("%T", payload))
			}
		}
	}()

	// Wait for either goroutine to exit with an error.
	var retErr error
	select {
	case retErr = <-errCh:
	case <-ctx.Done():
		retErr = ctx.Err()
	}

	// Cancel derived context to signal the other goroutine, then wait.
	runCancel()

	// Wait for in-flight handler goroutines so their outbound messages
	// are enqueued before we drain the channel.
	handlerWg.Wait()
	close(outboundCh)

	wg.Wait()
	return retErr
}

func (s *StreamHandler) sendHeartbeat(stream backupv1.BackupService_ConnectClient) error {
	hb := &backupv1.Heartbeat{
		Timestamp:     timestamppb.Now(),
		Status:        "idle",
		AgentVersion:  AgentVersion,
		ResticVersion: ResticVersion,
		RcloneVersion: RcloneVersion,
	}

	if s.jobStatusFn != nil {
		if js := s.jobStatusFn(); js != nil {
			hb.Status = "running"
			hb.CurrentJob = &backupv1.RunningJob{
				PlanName:        js.PlanName,
				StartedAt:       timestamppb.New(js.StartedAt),
				ProgressPercent: js.ProgressPercent,
			}
		}
	}

	s.identityMu.RLock()
	agentID := s.identity.AgentID
	apiKey := s.identity.APIKey
	s.identityMu.RUnlock()

	msg := &backupv1.AgentMessage{
		AgentId: agentID,
		ApiKey:  apiKey,
		Payload: &backupv1.AgentMessage_Heartbeat{
			Heartbeat: hb,
		},
	}
	return stream.Send(msg)
}

func (s *StreamHandler) sendLiveLogs(stream backupv1.BackupService_ConnectClient, jobID string, entries []*backupv1.LogEntry) error {
	s.identityMu.RLock()
	agentID := s.identity.AgentID
	apiKey := s.identity.APIKey
	s.identityMu.RUnlock()

	msg := &backupv1.AgentMessage{
		AgentId: agentID,
		ApiKey:  apiKey,
		Payload: &backupv1.AgentMessage_LiveLogs{
			LiveLogs: &backupv1.LiveLogs{
				JobId:   jobID,
				Entries: entries,
			},
		},
	}
	return stream.Send(msg)
}

// extractJobID looks for a "job_id" attribute in the log entry's JSON attributes.
func extractJobID(entry *backupv1.LogEntry) string {
	if entry.Attributes == "" {
		return ""
	}
	var attrs map[string]string
	if err := json.Unmarshal([]byte(entry.Attributes), &attrs); err != nil {
		return ""
	}
	return attrs["job_id"]
}

func (s *StreamHandler) handleApproval(approval *backupv1.Approval) {
	slog.Info("received approval", "source", "stream", "status", approval.GetStatus())
	if approval.GetStatus() == backupv1.AgentStatus_AGENT_STATUS_APPROVED {
		s.identityMu.Lock()
		s.identity.APIKey = approval.GetApiKey()
		agentID := s.identity.AgentID
		s.identityMu.Unlock()

		if s.onApproval != nil {
			s.onApproval(agentID, approval.GetApiKey())
		}
	} else if approval.GetStatus() == backupv1.AgentStatus_AGENT_STATUS_REJECTED {
		slog.Warn("agent rejected by server", "source", "stream")
	}
}

func (s *StreamHandler) handleConfig(outboundCh chan<- *backupv1.AgentMessage, cfg *backupv1.AgentConfig) {
	slog.Info("received config", "source", "stream", "config_version", cfg.GetConfigVersion())

	// Apply heartbeat interval if provided.
	if hb := cfg.GetHeartbeatIntervalSecs(); hb > 0 {
		s.heartbeatMu.Lock()
		s.heartbeatInterval = time.Duration(hb) * time.Second
		s.heartbeatMu.Unlock()
		slog.Info("heartbeat interval updated", "source", "stream", "interval_secs", hb)
	}

	if s.onConfig != nil {
		s.onConfig(cfg)
	}

	s.identityMu.RLock()
	agentID := s.identity.AgentID
	apiKey := s.identity.APIKey
	s.identityMu.RUnlock()

	// Enqueue config ack for the send goroutine.
	ack := &backupv1.AgentMessage{
		AgentId: agentID,
		ApiKey:  apiKey,
		Payload: &backupv1.AgentMessage_ConfigAck{
			ConfigAck: &backupv1.ConfigAck{
				ConfigVersion: cfg.GetConfigVersion(),
				Success:       true,
			},
		},
	}
	outboundCh <- ack
}

func (s *StreamHandler) handleCommand(outboundCh chan<- *backupv1.AgentMessage, cmd *backupv1.Command) {
	slog.Info("received command", "source", "stream", "command_id", cmd.GetCommandId())

	var result *backupv1.CommandResult
	if s.onCommand != nil {
		result = s.onCommand(cmd)
	} else {
		result = &backupv1.CommandResult{
			CommandId: cmd.GetCommandId(),
			Success:   false,
			Error:     "no command handler registered",
		}
	}

	s.identityMu.RLock()
	agentID := s.identity.AgentID
	apiKey := s.identity.APIKey
	s.identityMu.RUnlock()

	// Enqueue command result for the send goroutine.
	msg := &backupv1.AgentMessage{
		AgentId: agentID,
		ApiKey:  apiKey,
		Payload: &backupv1.AgentMessage_CommandResult{
			CommandResult: result,
		},
	}
	outboundCh <- msg
}
