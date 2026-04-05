package grpcserver

import (
	"io"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	agent, err := s.db.GetAgent(agentID)
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

	log.Printf("Agent %s (%s) connected, status=%s", agentID, agent.Hostname, agent.Status)

	// Process the first message.
	s.handleAgentMessage(agentID, agent.Status, firstMsg)

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

			// Re-check the agent status from DB (agent might have been approved/rejected).
			currentAgent, dbErr := s.db.GetAgent(agentID)
			currentStatus := agent.Status
			if dbErr == nil && currentAgent != nil {
				currentStatus = currentAgent.Status
			}

			s.handleAgentMessage(agentID, currentStatus, msg)
		}
	}()

	// Wait for either goroutine to finish (error or stream close).
	streamErr := <-errCh

	// Unregister the agent (deferred above), which closes sendCh
	// and stops the send goroutine. No manual close(sendCh) here —
	// Unregister owns the channel lifecycle to prevent races.

	log.Printf("Agent %s disconnected", agentID)

	if streamErr != nil && streamErr != io.EOF {
		return streamErr
	}
	return nil
}

// handleAgentMessage processes a single message from an agent.
func (s *GRPCServer) handleAgentMessage(agentID, agentStatus string, msg *backupv1.AgentMessage) {
	switch payload := msg.Payload.(type) {
	case *backupv1.AgentMessage_Heartbeat:
		hb := payload.Heartbeat
		hbStatus := hb.Status
		if hbStatus == "" {
			hbStatus = "idle"
		}

		// Update manager state.
		s.mgr.UpdateHeartbeat(agentID, hbStatus)

		// Update database.
		if err := s.db.UpdateHeartbeat(agentID, hb.AgentVersion, hb.ResticVersion, hb.RcloneVersion); err != nil {
			log.Printf("Failed to update heartbeat for agent %s: %v", agentID, err)
		}

	case *backupv1.AgentMessage_ConfigAck:
		ack := payload.ConfigAck
		if ack.Success {
			now := time.Now().UTC()
			if err := s.db.UpdateConfigApplied(agentID, now); err != nil {
				log.Printf("Failed to update config applied for agent %s: %v", agentID, err)
			}
		} else {
			log.Printf("Agent %s failed to apply config version %d: %s", agentID, ack.ConfigVersion, ack.Error)
		}

	case *backupv1.AgentMessage_CommandResult:
		s.mgr.HandleCommandResult(agentID, payload.CommandResult)

	default:
		log.Printf("Unknown message type from agent %s", agentID)
	}
}
