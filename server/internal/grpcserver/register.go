package grpcserver

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

// Register handles agent enrollment. Creates a new agent record with pending status.
func (s *GRPCServer) Register(ctx context.Context, req *backupv1.RegisterRequest) (*backupv1.RegisterResponse, error) {
	agentID := uuid.New().String()

	agent := &database.Agent{
		ID:            agentID,
		Name:          req.Hostname, // Default name to hostname.
		Hostname:      req.Hostname,
		Status:        "pending",
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

	if err := s.db.CreateAgent(agent); err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	return &backupv1.RegisterResponse{
		AgentId: agentID,
		Status:  backupv1.AgentStatus_AGENT_STATUS_PENDING,
	}, nil
}
