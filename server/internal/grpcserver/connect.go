package grpcserver

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tryy3/backup-orchestrator/server/internal/agentmgr"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/events"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
)

// Connect handles the bidirectional streaming RPC for agent connections.
func (s *GRPCServer) Connect(stream backupv1.BackupService_ConnectServer) error {
	// Wait for the first message to identify the agent.
	firstMsg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Internal, "failed to receive first message: %v", err)
	}

	agentID := firstMsg.AgentId
	if agentID == "" {
		return status.Error(codes.InvalidArgument, "agent_id is required")
	}

	// Look up the agent.
	agent, err := s.db.GetAgent(stream.Context(), agentID)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to get agent: %v", err)
	}
	if agent == nil {
		return status.Error(codes.NotFound, "agent not found")
	}

	// Validate API key for approved agents.
	if agent.Status == "approved" && (agent.APIKey == nil || firstMsg.ApiKey != *agent.APIKey) {
		return status.Error(codes.Unauthenticated, "invalid api_key")
	}

	if agent.Status == "rejected" {
		return status.Error(codes.PermissionDenied, "agent is rejected")
	}

	// Register agent in the in-memory manager.
	sendCh := make(chan *backupv1.ServerMessage, 32)
	s.mgr.Register(agentID, sendCh)
	defer s.mgr.Unregister(agentID)

	slog.Info("agent connected", "agent_id", agentID, "hostname", agent.Hostname, "status", agent.Status)

	// Broadcast agent.connected event.
	s.hub.Broadcast(events.Event{
		Type: "agent.connected",
		Payload: map[string]interface{}{
			"agent_id": agentID,
			"hostname": agent.Hostname,
		},
	})

	// Process the first message.
	s.handleAgentMessage(stream.Context(), agentID, agent.Status, firstMsg)

	// Error channel to coordinate goroutine shutdown.
	errCh := make(chan error, 2)

	// Send goroutine: reads from the agent's send channel and writes to the stream.
	go func() {
		for msg := range sendCh {
			if err := stream.Send(msg); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()

	// Receive goroutine: reads from the stream and processes agent messages.
	go func() {
		cachedStatus := agent.Status
		var lastStatusCheck time.Time

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				errCh <- nil
				return
			}
			if err != nil {
				errCh <- err
				return
			}

			// Re-check the agent status from DB periodically (not every message).
			if time.Since(lastStatusCheck) > 60*time.Second {
				currentAgent, dbErr := s.db.GetAgent(stream.Context(), agentID)
				if dbErr == nil && currentAgent != nil {
					cachedStatus = currentAgent.Status
				}
				lastStatusCheck = time.Now()
			}

			s.handleAgentMessage(stream.Context(), agentID, cachedStatus, msg)
		}
	}()

	// Wait for either goroutine to finish (error or stream close).
	streamErr := <-errCh

	// Unregister the agent (deferred above), which closes sendCh
	// and stops the send goroutine. No manual close(sendCh) here —
	// Unregister owns the channel lifecycle to prevent races.

	slog.Info("agent disconnected", "agent_id", agentID)

	// Broadcast agent.disconnected event.
	s.hub.Broadcast(events.Event{
		Type: "agent.disconnected",
		Payload: map[string]interface{}{
			"agent_id": agentID,
		},
	})

	if streamErr != nil && streamErr != io.EOF {
		return streamErr
	}
	return nil
}

// handleAgentMessage processes a single message from an agent.
func (s *GRPCServer) handleAgentMessage(ctx context.Context, agentID, agentStatus string, msg *backupv1.AgentMessage) {
	switch payload := msg.Payload.(type) {
	case *backupv1.AgentMessage_Heartbeat:
		hb := payload.Heartbeat
		hbStatus := hb.Status
		if hbStatus == "" {
			hbStatus = "idle"
		}

		// Extract current job info from heartbeat.
		var currentJob *agentmgr.CurrentJobInfo
		if hb.CurrentJob != nil {
			currentJob = &agentmgr.CurrentJobInfo{
				PlanName:        hb.CurrentJob.PlanName,
				ProgressPercent: hb.CurrentJob.ProgressPercent,
			}
			if hb.CurrentJob.StartedAt != nil {
				currentJob.StartedAt = hb.CurrentJob.StartedAt.AsTime().Format(time.RFC3339)
			}
		}

		// Update manager state and detect job transitions.
		transition := s.mgr.UpdateHeartbeat(agentID, hbStatus, currentJob)

		// Emit job events based on the transition.
		switch transition {
		case agentmgr.JobStarted:
			// Try to find and update the planned job in the DB.
			s.handleJobStarted(ctx, agentID, currentJob)
		case agentmgr.JobProgress:
			if currentJob != nil {
				s.hub.Broadcast(events.Event{
					Type: "job.progress",
					Payload: map[string]interface{}{
						"agent_id":         agentID,
						"plan_name":        currentJob.PlanName,
						"progress_percent": currentJob.ProgressPercent,
						"started_at":       currentJob.StartedAt,
					},
				})
			}
		}

		// Update database.
		if err := s.db.UpdateHeartbeat(ctx, agentID, hb.AgentVersion, hb.ResticVersion, hb.RcloneVersion); err != nil {
			slog.Error("failed to update heartbeat", "agent_id", agentID, "error", err)
		}

		// Broadcast agent.heartbeat event.
		s.hub.Broadcast(events.Event{
			Type: "agent.heartbeat",
			Payload: map[string]interface{}{
				"agent_id":  agentID,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			},
		})

	case *backupv1.AgentMessage_ConfigAck:
		ack := payload.ConfigAck
		if ack.Success {
			now := time.Now().UTC()
			if err := s.db.UpdateConfigApplied(ctx, agentID, now); err != nil {
				slog.Error("failed to update config applied", "agent_id", agentID, "error", err)
			}
		} else {
			slog.Warn("agent failed to apply config", "agent_id", agentID, "config_version", ack.ConfigVersion, "error", ack.Error)
		}

	case *backupv1.AgentMessage_CommandResult:
		s.mgr.HandleCommandResult(agentID, payload.CommandResult)

	case *backupv1.AgentMessage_LiveLogs:
		s.handleLiveLogs(ctx, agentID, payload.LiveLogs)

	default:
		slog.Warn("unknown message type from agent", "agent_id", agentID)
	}
}

// handleJobStarted processes a job-started transition detected from heartbeats.
// It finds any planned job for this agent's plan and updates it to "running".
func (s *GRPCServer) handleJobStarted(ctx context.Context, agentID string, currentJob *agentmgr.CurrentJobInfo) {
	if currentJob == nil {
		return
	}

	// Look up plans for this agent to find the plan ID from the plan name.
	plans, err := s.db.ListPlans(ctx, agentID)
	if err != nil {
		slog.Error("failed to list plans for agent", "agent_id", agentID, "error", err)
		return
	}

	var planID string
	for _, p := range plans {
		if p.Name == currentJob.PlanName {
			planID = p.ID
			break
		}
	}

	if planID == "" {
		slog.Warn("no plan found for agent", "plan_name", currentJob.PlanName, "agent_id", agentID)
		return
	}

	// Find planned job and update to running.
	planned, err := s.db.FindPlannedJob(ctx, agentID, planID)
	if err != nil {
		slog.Error("failed to find planned job", "agent_id", agentID, "plan_id", planID, "error", err)
		return
	}

	var jobID string
	if planned != nil {
		jobID = planned.ID
		if err := s.db.UpdateJobStatus(ctx, planned.ID, "running", nil, nil); err != nil {
			slog.Error("failed to update planned job to running", "job_id", planned.ID, "error", err)
		}
	}

	s.hub.Broadcast(events.Event{
		Type: "job.started",
		Payload: map[string]interface{}{
			"job_id":           jobID,
			"agent_id":         agentID,
			"plan_id":          planID,
			"plan_name":        currentJob.PlanName,
			"started_at":       currentJob.StartedAt,
			"progress_percent": currentJob.ProgressPercent,
		},
	})
}

// handleLiveLogs processes incremental log entries sent by an agent during a running job.
func (s *GRPCServer) handleLiveLogs(ctx context.Context, agentID string, ll *backupv1.LiveLogs) {
	if len(ll.GetEntries()) == 0 {
		return
	}

	// Convert proto entries to database log entries.
	dbEntries := make([]database.LogEntry, 0, len(ll.GetEntries()))
	for _, e := range ll.GetEntries() {
		le := database.LogEntry{
			Timestamp: e.Timestamp,
			Level:     e.Level,
			Source:    e.Source,
			Message:   e.Message,
		}
		if e.Attributes != "" {
			_ = json.Unmarshal([]byte(e.Attributes), &le.Attributes)
		}
		dbEntries = append(dbEntries, le)
	}

	// Append to the running job's log_tail in the database.
	if ll.GetJobId() != "" {
		if err := s.db.AppendJobLogs(ctx, ll.GetJobId(), dbEntries); err != nil {
			slog.Error("failed to append live logs", "job_id", ll.GetJobId(), "error", err)
		}
	}

	// Broadcast live log entries to the frontend.
	s.hub.Broadcast(events.Event{
		Type: "job.live_logs",
		Payload: map[string]interface{}{
			"agent_id": agentID,
			"job_id":   ll.GetJobId(),
			"entries":  dbEntries,
		},
	})
}
