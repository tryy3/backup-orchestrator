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
	// Per-attempt context: cancelling this also cancels the underlying stream,
	// which makes stream.Recv() return immediately instead of blocking until
	// a TCP timeout on half-open connections.
	streamCtx, streamCancel := context.WithCancel(ctx)
	defer streamCancel()

	stream, err := s.client.client.Connect(streamCtx)
	if err != nil {
		return fmt.Errorf("opening connect stream: %w", err)
	}

	// Send initial heartbeat immediately.
	if err := s.sendHeartbeat(stream); err != nil {
		return fmt.Errorf("sending initial heartbeat: %w", err)
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	// streamCtx is used by the send goroutine to detect shutdown; it is also
	// the context attached to the gRPC stream, so cancelling it unblocks Recv.
	runCtx := streamCtx
	runCancel := streamCancel

	// Start send goroutine: heartbeats at configured interval + live log forwarding.
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
				// Close the send side of the stream.
				_ = stream.CloseSend()
				return
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
				s.handleConfig(stream, payload.Config)

			case *backupv1.ServerMessage_Command:
				s.handleCommand(stream, payload.Command)

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

func (s *StreamHandler) handleConfig(stream backupv1.BackupService_ConnectClient, cfg *backupv1.AgentConfig) {
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

	// Send config ack.
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
	if err := stream.Send(ack); err != nil {
		slog.Error("error sending config ack", "source", "stream", "error", err)
	}
}

func (s *StreamHandler) handleCommand(stream backupv1.BackupService_ConnectClient, cmd *backupv1.Command) {
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

	// Send command result.
	msg := &backupv1.AgentMessage{
		AgentId: agentID,
		ApiKey:  apiKey,
		Payload: &backupv1.AgentMessage_CommandResult{
			CommandResult: result,
		},
	}
	if err := stream.Send(msg); err != nil {
		slog.Error("error sending command result", "source", "stream", "error", err)
	}
}
