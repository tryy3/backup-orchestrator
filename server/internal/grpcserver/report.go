package grpcserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/google/uuid"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/events"
)

// snapshotCache stores the latest snapshot reports per agent/repository in memory.
var (
	snapshotCacheMu sync.RWMutex
	snapshotCache   = make(map[string][]*backupv1.SnapshotInfo) // key: "agentID:repoID"
)

// ReportJob handles a completed job report from an agent.
func (s *GRPCServer) ReportJob(ctx context.Context, req *backupv1.JobReport) (*backupv1.JobReportAck, error) {
	agentID := agentIDFromContext(ctx)
	if agentID == "" {
		agentID = req.AgentId
	}

	job := &database.Job{
		ID:       req.JobId,
		AgentID:  agentID,
		PlanName: req.PlanName,
		Type:     req.Type,
		Trigger:  req.Trigger,
		Status:   req.Status,
	}

	if job.ID == "" {
		job.ID = uuid.New().String()
	}

	if req.PlanId != "" {
		job.PlanID = &req.PlanId
	}

	if req.StartedAt != nil {
		job.StartedAt = req.StartedAt.AsTime()
	}
	if req.FinishedAt != nil {
		t := req.FinishedAt.AsTime()
		job.FinishedAt = &t
	}
	// Serialize structured log entries as JSON into log_tail column.
	if len(req.LogEntries) > 0 {
		type logEntry struct {
			Timestamp  string            `json:"timestamp"`
			Level      string            `json:"level"`
			Source     string            `json:"source"`
			Message    string            `json:"message"`
			Attributes map[string]string `json:"attributes,omitempty"`
		}
		entries := make([]logEntry, 0, len(req.LogEntries))
		for _, e := range req.LogEntries {
			le := logEntry{
				Timestamp: e.Timestamp,
				Level:     e.Level,
				Source:    e.Source,
				Message:   e.Message,
			}
			if e.Attributes != "" {
				_ = json.Unmarshal([]byte(e.Attributes), &le.Attributes)
			}
			entries = append(entries, le)
		}
		if data, err := json.Marshal(entries); err == nil {
			s := string(data)
			job.LogTail = &s
		}
	} else if req.LogTail != "" {
		job.LogTail = &req.LogTail
	}

	// Convert repository results.
	for _, rr := range req.RepositoryResults {
		result := database.JobRepositoryResult{
			ID:             uuid.New().String(),
			RepositoryID:   rr.RepositoryId,
			RepositoryName: rr.RepositoryName,
			Status:         rr.Status,
		}
		if rr.SnapshotId != "" {
			result.SnapshotID = &rr.SnapshotId
		}
		if rr.Error != "" {
			result.Error = &rr.Error
		}
		if rr.FilesNew != 0 {
			result.FilesNew = &rr.FilesNew
		}
		if rr.FilesChanged != 0 {
			result.FilesChanged = &rr.FilesChanged
		}
		if rr.FilesUnmodified != 0 {
			result.FilesUnmodified = &rr.FilesUnmodified
		}
		if rr.BytesAdded != 0 {
			result.BytesAdded = &rr.BytesAdded
		}
		if rr.TotalBytes != 0 {
			result.TotalBytes = &rr.TotalBytes
		}
		if rr.DurationMs != 0 {
			result.DurationMs = &rr.DurationMs
		}
		job.RepositoryResults = append(job.RepositoryResults, result)
	}

	// Convert hook results.
	for _, hr := range req.HookResults {
		result := database.JobHookResult{
			ID:       uuid.New().String(),
			HookName: hr.HookName,
			Phase:    hr.Phase,
			Status:   hr.Status,
		}
		if hr.Error != "" {
			result.Error = &hr.Error
		}
		if hr.DurationMs != 0 {
			result.DurationMs = &hr.DurationMs
		}
		job.HookResults = append(job.HookResults, result)
	}

	if err := s.storeJobReport(ctx, job); err != nil {
		log.Printf("Failed to create job from report: %v", err)
		return &backupv1.JobReportAck{
			Success: false,
			Error:   fmt.Sprintf("failed to store job: %v", err),
		}, nil
	}

	// Broadcast job.completed event.
	s.hub.Broadcast(events.Event{
		Type:    "job.completed",
		Payload: job,
	})

	return &backupv1.JobReportAck{Success: true}, nil
}

// storeJobReport either updates an existing planned/running job or creates a new one.
func (s *GRPCServer) storeJobReport(ctx context.Context, job *database.Job) error {
	// Try to find a planned/running job to update.
	if job.PlanID != nil && *job.PlanID != "" {
		planned, err := s.db.FindPlannedJob(ctx, job.AgentID, *job.PlanID)
		if err != nil {
			log.Printf("Failed to find planned job: %v", err)
			// Fall through to create a new job.
		}
		if planned != nil {
			// Update the existing planned job with the final report data.
			job.ID = planned.ID
			return s.db.CompleteJob(ctx, job)
		}
	}

	// No planned job found — create a new one (e.g., scheduled jobs).
	return s.db.CreateJob(ctx, job)
}

// ReportSnapshots stores snapshot data from an agent in an in-memory cache.
func (s *GRPCServer) ReportSnapshots(ctx context.Context, req *backupv1.SnapshotReport) (*backupv1.SnapshotReportAck, error) {
	agentID := agentIDFromContext(ctx)
	if agentID == "" {
		agentID = req.AgentId
	}

	key := agentID + ":" + req.RepositoryId

	snapshotCacheMu.Lock()
	snapshotCache[key] = req.Snapshots
	snapshotCacheMu.Unlock()

	return &backupv1.SnapshotReportAck{Success: true}, nil
}
