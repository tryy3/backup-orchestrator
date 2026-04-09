package grpcserver

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/events"
	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
)

// Register handles agent enrollment. Creates a new agent record with pending status.
func (s *GRPCServer) Register(ctx context.Context, req *backupv1.RegisterRequest) (*backupv1.RegisterResponse, error) {
	agentID := uuid.New().String()

	agent := &database.Agent{
		ID:       agentID,
		Name:     req.Hostname, // Default name to hostname.
		Hostname: req.Hostname,
		Status:   "pending",
	}

	if req.Os != "" {
		agent.OS = &req.Os
	}
	if req.AgentVersion != "" {
		agent.AgentVersion = &req.AgentVersion
	}
	if req.ResticVersion != "" {
		agent.ResticVersion = &req.ResticVersion
	}
	if req.RcloneVersion != "" {
		agent.RcloneVersion = &req.RcloneVersion
	}

	if err := s.db.CreateAgent(ctx, agent); err != nil {
		return nil, status.Errorf(codes.Internal, "create agent: %v", err)
	}

	// Broadcast agent.registered event.
	s.hub.Broadcast(events.Event{
		Type:    "agent.registered",
		Payload: agent,
	})

	return &backupv1.RegisterResponse{
		AgentId: agentID,
		Status:  backupv1.AgentStatus_AGENT_STATUS_PENDING,
	}, nil
}
