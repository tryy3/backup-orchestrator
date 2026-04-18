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

// Default per-command timeouts. Commands that can legitimately run for a long
// time (e.g. restic backup/restore against slow remote backends) get a much
// larger budget than lookup-style commands. These are the fallbacks used when
// the server has not pushed CommandTimeouts in AgentConfig.
const (
	defaultCommandTimeout         = 5 * time.Minute
	defaultBackupCommandTimeout   = 24 * time.Hour
	defaultRestoreCommandTimeout  = 24 * time.Hour
	defaultBrowseFSCommandTimeout = 30 * time.Second
	defaultListSnapshotsTimeout   = 5 * time.Minute
	defaultBrowseSnapshotTimeout  = 5 * time.Minute
)

// commandTimeout returns the per-command deadline to apply based on the
// command kind, honoring server-pushed overrides if present. Browse/list
// commands default to short timeouts so a single hung lookup does not tie up
// a worker for long; backup/restore get a much larger budget since they can
// run for hours against remote backends.
func (s *StreamHandler) commandTimeout(cmd *backupv1.Command) time.Duration {
	s.cmdTimeoutsMu.RLock()
	overrides := s.cmdTimeouts
	s.cmdTimeoutsMu.RUnlock()

	pickSecs := func(override int32, fallback time.Duration) time.Duration {
		if override > 0 {
			return time.Duration(override) * time.Second
		}
		return fallback
	}

	switch cmd.GetAction().(type) {
	case *backupv1.Command_TriggerBackup:
		return pickSecs(overrides.GetBackupSecs(), defaultBackupCommandTimeout)
	case *backupv1.Command_TriggerRestore:
		return pickSecs(overrides.GetRestoreSecs(), defaultRestoreCommandTimeout)
	case *backupv1.Command_ListSnapshots:
		return pickSecs(overrides.GetListSnapshotsSecs(), defaultListSnapshotsTimeout)
	case *backupv1.Command_BrowseSnapshot:
		return pickSecs(overrides.GetBrowseSnapshotSecs(), defaultBrowseSnapshotTimeout)
	case *backupv1.Command_BrowseFilesystem:
		return pickSecs(overrides.GetBrowseFilesystemSecs(), defaultBrowseFSCommandTimeout)
	default:
		return pickSecs(overrides.GetDefaultSecs(), defaultCommandTimeout)
	}
}

// applyCommandTimeouts stores command timeout overrides pushed by the server.
// A nil argument clears any previously stored overrides (reverting to defaults).
func (s *StreamHandler) applyCommandTimeouts(t *backupv1.CommandTimeouts) {
	s.cmdTimeoutsMu.Lock()
	s.cmdTimeouts = t
	s.cmdTimeoutsMu.Unlock()
}

// StreamHandler manages the bidirectional Connect stream lifecycle.
type StreamHandler struct {
	client            *Client
	identity          *identity.Identity
	identityMu        sync.RWMutex
	onApproval        func(agentID, apiKey string)
	onConfig          func(cfg *backupv1.AgentConfig)
	onCommand         func(ctx context.Context, cmd *backupv1.Command) *backupv1.CommandResult
	jobStatusFn       JobStatusFunc
	liveLogCh         <-chan *backupv1.LogEntry // receives live log entries from running jobs
	heartbeatInterval time.Duration
	heartbeatMu       sync.RWMutex
	// sendMu serializes writes to the stream. gRPC streams are not safe for
	// concurrent Send calls, and command results are now sent from per-command
	// goroutines in addition to heartbeats (send loop) and config acks
	// (recv loop).
	sendMu sync.Mutex
	// inflight tracks per-command goroutines so Run can wait for them to
	// finish (or observe their context cancellation) before returning.
	inflight sync.WaitGroup
	// cmdTimeouts holds server-pushed per-command timeout overrides; nil means
	// fall back to the package-level defaults.
	cmdTimeouts   *backupv1.CommandTimeouts
	cmdTimeoutsMu sync.RWMutex
}

// NewStreamHandler creates a new StreamHandler.
func NewStreamHandler(
	client *Client,
	id *identity.Identity,
	onApproval func(agentID, apiKey string),
	onConfig func(cfg *backupv1.AgentConfig),
	onCommand func(ctx context.Context, cmd *backupv1.Command) *backupv1.CommandResult,
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

// safeSend serializes concurrent stream.Send calls. gRPC client streams are
// not safe for concurrent writes so every outbound message must go through
// this helper.
func (s *StreamHandler) safeSend(stream backupv1.BackupService_ConnectClient, msg *backupv1.AgentMessage) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return stream.Send(msg)
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
			case <-streamCtx.Done():
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
				s.handleCommand(streamCtx, stream, payload.Command)

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

	// Cancel stream context to unblock Recv and signal the other goroutine, then wait.
	streamCancel()
	wg.Wait()
	// Wait for in-flight per-command goroutines. Their contexts are derived
	// from streamCtx so they have already been signalled to cancel.
	s.inflight.Wait()
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
	return s.safeSend(stream, msg)
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
	return s.safeSend(stream, msg)
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

	// Apply per-command timeout overrides if provided. A nil value reverts
	// to the built-in defaults.
	s.applyCommandTimeouts(cfg.GetCommandTimeouts())

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
	if err := s.safeSend(stream, ack); err != nil {
		slog.Error("error sending config ack", "source", "stream", "error", err)
	}
}

func (s *StreamHandler) handleCommand(ctx context.Context, stream backupv1.BackupService_ConnectClient, cmd *backupv1.Command) {
	slog.Info("received command", "source", "stream", "command_id", cmd.GetCommandId())

	// Dispatch the command in its own goroutine so the recv loop keeps
	// draining ServerMessages while this command runs. Each command gets a
	// per-kind timeout so a single hung restic/rclone invocation cannot
	// wedge the agent forever.
	s.inflight.Add(1)
	go func() {
		defer s.inflight.Done()

		cmdCtx, cancel := context.WithTimeout(ctx, s.commandTimeout(cmd))
		defer cancel()

		var result *backupv1.CommandResult
		if s.onCommand != nil {
			result = s.onCommand(cmdCtx, cmd)
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

		msg := &backupv1.AgentMessage{
			AgentId: agentID,
			ApiKey:  apiKey,
			Payload: &backupv1.AgentMessage_CommandResult{
				CommandResult: result,
			},
		}
		if err := s.safeSend(stream, msg); err != nil {
			slog.Error("error sending command result",
				"source", "stream",
				"command_id", cmd.GetCommandId(),
				"error", err,
			)
		}
	}()
}
