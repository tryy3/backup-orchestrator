package grpcclient

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/tryy3/backup-orchestrator/agent/internal/identity"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
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
	client      *Client
	identity    *identity.Identity
	identityMu  sync.RWMutex
	onApproval  func(agentID, apiKey string)
	onConfig    func(cfg *backupv1.AgentConfig)
	onCommand   func(cmd *backupv1.Command) *backupv1.CommandResult
	jobStatusFn JobStatusFunc
}

// NewStreamHandler creates a new StreamHandler.
func NewStreamHandler(
	client *Client,
	id *identity.Identity,
	onApproval func(agentID, apiKey string),
	onConfig func(cfg *backupv1.AgentConfig),
	onCommand func(cmd *backupv1.Command) *backupv1.CommandResult,
	jobStatusFn JobStatusFunc,
) *StreamHandler {
	return &StreamHandler{
		client:      client,
		identity:    id,
		onApproval:  onApproval,
		onConfig:    onConfig,
		onCommand:   onCommand,
		jobStatusFn: jobStatusFn,
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

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	// Start send goroutine: heartbeats every 30 seconds.
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-runCtx.Done():
				// Close the send side of the stream.
				stream.CloseSend()
				return
			case <-ticker.C:
				if err := s.sendHeartbeat(stream); err != nil {
					errCh <- fmt.Errorf("sending heartbeat: %w", err)
					return
				}
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
