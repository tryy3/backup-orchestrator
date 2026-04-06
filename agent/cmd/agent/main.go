package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/tryy3/backup-orchestrator/agent/internal/config"
	"github.com/tryy3/backup-orchestrator/agent/internal/database"
	"github.com/tryy3/backup-orchestrator/agent/internal/executor"
	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
	"github.com/tryy3/backup-orchestrator/agent/internal/grpcclient"
	"github.com/tryy3/backup-orchestrator/agent/internal/identity"
	"github.com/tryy3/backup-orchestrator/agent/internal/localconfig"
	"github.com/tryy3/backup-orchestrator/agent/internal/reporter"
	"github.com/tryy3/backup-orchestrator/agent/internal/scheduler"
)

func main() {
	// Set up structured logging.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	// Step 1: Load config from env vars.
	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}
	slog.Info("agent starting",
		"source", "agent",
		"agent_name", cfg.AgentName,
		"server", cfg.ServerURL,
		"data_dir", cfg.DataDir,
	)

	// Step 2: Ensure data dir exists.
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		slog.Error("creating data dir", "error", err)
		os.Exit(1)
	}

	// Step 3: Load or create identity.
	id, err := identity.Load(cfg.DataDir)
	if err != nil {
		slog.Error("loading identity", "error", err)
		os.Exit(1)
	}

	// Step 4: Open agent SQLite DB, run migrations.
	db, err := database.Open(cfg.DataDir)
	if err != nil {
		slog.Error("opening database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Step 5: Load local config from disk (if exists).
	localCfg, err := localconfig.Load(cfg.DataDir)
	if err != nil {
		slog.Info("no local config found (first run or missing)", "source", "agent", "error", err)
		localCfg = nil
	}

	// Step 6: Create ResticExecutor, JobOrchestrator.
	resticExec := &executor.ResticExecutor{
		RcloneConfigPath: executor.RcloneConfigPath(cfg.DataDir),
	}
	orchestrator := &executor.JobOrchestrator{
		Restic:    resticExec,
		AgentName: cfg.AgentName,
	}

	// Step 7: Create gRPC client.
	grpcClient, err := grpcclient.New(cfg)
	if err != nil {
		slog.Error("creating gRPC client", "error", err)
		os.Exit(1)
	}
	defer grpcClient.Close()

	// Step 8: Create Reporter (buffer flusher).
	rep := reporter.New(db, grpcClient, 60*time.Second)

	// Shared state for current config (protected by mutex).
	var (
		currentConfig *backupv1.AgentConfig
		configMu      sync.RWMutex
	)

	// ReportFunc for scheduler: buffer the report and attempt delivery.
	reportFn := func(report *backupv1.JobReport) {
		configMu.RLock()
		if id != nil {
			report.AgentId = id.AgentID
			report.ApiKey = id.APIKey
		}
		configMu.RUnlock()

		// Record local job.
		startedAt := ""
		finishedAt := ""
		if report.StartedAt != nil {
			startedAt = report.StartedAt.AsTime().Format(time.RFC3339)
		}
		if report.FinishedAt != nil {
			finishedAt = report.FinishedAt.AsTime().Format(time.RFC3339)
		}
		if err := db.InsertLocalJob(
			report.JobId, report.PlanName, report.Type, report.Status,
			startedAt, finishedAt, report.LogTail,
		); err != nil {
			slog.Error("error recording local job", "source", "agent", "error", err)
		}

		// Try direct delivery first, buffer on failure.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := grpcClient.ReportJob(ctx, report); err != nil {
			slog.Warn("direct report delivery failed, buffering", "source", "agent", "error", err)
			if bufErr := rep.BufferReport(report); bufErr != nil {
				slog.Error("error buffering report", "source", "agent", "error", bufErr)
			}
		} else {
			slog.Info("report delivered successfully", "source", "agent", "job_id", report.JobId)
		}
	}

	// Step 9: Create Scheduler.
	sched := scheduler.New(orchestrator, reportFn)

	// Step 10: If no identity (first run): call Register, get agent_id, save identity.
	if id == nil {
		slog.Info("no identity found, registering with server...", "source", "agent")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		resp, err := grpcClient.Register(ctx, cfg.AgentName)
		cancel()
		if err != nil {
			slog.Error("registration failed", "error", err)
			os.Exit(1)
		}

		id = &identity.Identity{
			AgentID: resp.GetAgentId(),
		}
		if err := identity.Save(cfg.DataDir, id); err != nil {
			slog.Error("saving identity", "error", err)
			os.Exit(1)
		}
		slog.Info("registered with server",
			"source", "agent",
			"agent_id", id.AgentID,
			"status", resp.GetStatus(),
		)
	} else {
		slog.Info("loaded identity", "source", "agent", "agent_id", id.AgentID)
	}

	// Root context for graceful shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Step 11: Start StreamHandler in goroutine with reconnect loop.
	go func() {
		streamHandler := grpcclient.NewStreamHandler(
			grpcClient,
			id,
			// onApproval: save API key to identity.
			func(agentID, apiKey string) {
				configMu.Lock()
				id.APIKey = apiKey
				configMu.Unlock()

				if err := identity.Save(cfg.DataDir, id); err != nil {
					slog.Error("error saving identity after approval", "source", "agent", "error", err)
				} else {
					slog.Info("API key saved after approval", "source", "agent")
				}
			},
			// onConfig: save to disk, write rclone.conf, update scheduler.
			func(agentCfg *backupv1.AgentConfig) {
				configMu.Lock()
				currentConfig = agentCfg
				configMu.Unlock()

				// Save config to disk.
				if err := localconfig.Save(cfg.DataDir, agentCfg); err != nil {
					slog.Error("error saving local config", "source", "agent", "error", err)
				}

				// Write rclone config if present.
				if rcloneCfg := agentCfg.GetRcloneConfig(); rcloneCfg != "" {
					if err := executor.WriteRcloneConfig(cfg.DataDir, rcloneCfg); err != nil {
						slog.Error("error writing rclone config", "source", "agent", "error", err)
					}
				}

				// Update scheduler.
				sched.UpdateSchedule(
					agentCfg.GetBackupPlans(),
					agentCfg.GetRepositories(),
					agentCfg.GetDefaultRetention(),
				)
				slog.Info("config applied",
					"source", "agent",
					"config_version", agentCfg.GetConfigVersion(),
				)
			},
			// onCommand: dispatch to executor.
			func(cmd *backupv1.Command) *backupv1.CommandResult {
				return handleCommand(cmd, sched, resticExec, &configMu, &currentConfig, id)
			},
			// jobStatus: report current running job for heartbeats.
			sched.JobStatusFunc(),
		)

		backoff := time.Second
		maxBackoff := 5 * time.Minute

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			slog.Info("connecting to server...", "source", "agent")
			err := streamHandler.Run(ctx)
			if ctx.Err() != nil {
				return // context cancelled, shutting down
			}
			slog.Warn("stream disconnected", "source", "agent", "error", err)

			// Flush any buffered reports on reconnect.
			rep.FlushNow()

			slog.Info("reconnecting", "source", "agent", "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			}

			// Exponential backoff with cap.
			backoff = time.Duration(math.Min(float64(backoff*2), float64(maxBackoff)))
		}
	}()

	// Step 12: If local config exists, start scheduler immediately.
	if localCfg != nil {
		slog.Info("starting scheduler with local config",
			"source", "agent",
			"config_version", localCfg.GetConfigVersion(),
		)
		configMu.Lock()
		currentConfig = localCfg
		configMu.Unlock()

		// Write rclone config if present.
		if rcloneCfg := localCfg.GetRcloneConfig(); rcloneCfg != "" {
			if err := executor.WriteRcloneConfig(cfg.DataDir, rcloneCfg); err != nil {
				slog.Error("error writing rclone config", "source", "agent", "error", err)
			}
		}

		sched.UpdateSchedule(
			localCfg.GetBackupPlans(),
			localCfg.GetRepositories(),
			localCfg.GetDefaultRetention(),
		)
	}

	sched.Start()

	// Step 13: Start reporter.Run in goroutine.
	go rep.Run(ctx)

	// Step 14: Handle SIGINT/SIGTERM.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	slog.Info("received signal, shutting down...", "source", "agent", "signal", sig)

	cancel() // cancel context for all goroutines
	sched.Stop()
	grpcClient.Close()
	db.Close()

	slog.Info("shutdown complete", "source", "agent")
}

// handleCommand dispatches server commands to the appropriate executor.
func handleCommand(
	cmd *backupv1.Command,
	sched *scheduler.Scheduler,
	resticExec *executor.ResticExecutor,
	configMu *sync.RWMutex,
	currentConfig **backupv1.AgentConfig,
	id *identity.Identity,
) *backupv1.CommandResult {
	result := &backupv1.CommandResult{
		CommandId: cmd.GetCommandId(),
	}

	configMu.RLock()
	cfg := *currentConfig
	configMu.RUnlock()

	if cfg == nil {
		result.Success = false
		result.Error = "no config loaded"
		return result
	}

	switch action := cmd.GetAction().(type) {
	case *backupv1.Command_TriggerBackup:
		sched.TriggerNow(
			action.TriggerBackup.GetPlanId(),
			cfg.GetBackupPlans(),
			cfg.GetRepositories(),
			cfg.GetDefaultRetention(),
		)
		result.Success = true

	case *backupv1.Command_ListSnapshots:
		repo := findRepo(cfg.GetRepositories(), action.ListSnapshots.GetRepositoryId())
		if repo == nil {
			result.Success = false
			result.Error = fmt.Sprintf("repository %s not found", action.ListSnapshots.GetRepositoryId())
			return result
		}
		snapshots, err := resticExec.Snapshots(context.Background(), executor.Repository{
			ID:       repo.GetId(),
			Name:     repo.GetName(),
			Type:     repo.GetType(),
			Path:     repo.GetPath(),
			Password: repo.GetPassword(),
		})
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			return result
		}
		data, _ := json.Marshal(snapshots)
		result.Success = true
		result.Data = data

	case *backupv1.Command_BrowseSnapshot:
		repo := findRepo(cfg.GetRepositories(), action.BrowseSnapshot.GetRepositoryId())
		if repo == nil {
			result.Success = false
			result.Error = fmt.Sprintf("repository %s not found", action.BrowseSnapshot.GetRepositoryId())
			return result
		}
		files, err := resticExec.ListFiles(context.Background(), executor.Repository{
			ID:       repo.GetId(),
			Name:     repo.GetName(),
			Type:     repo.GetType(),
			Path:     repo.GetPath(),
			Password: repo.GetPassword(),
		}, action.BrowseSnapshot.GetSnapshotId(), action.BrowseSnapshot.GetPath())
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			return result
		}
		data, _ := json.Marshal(files)
		result.Success = true
		result.Data = data

	case *backupv1.Command_TriggerRestore:
		repo := findRepo(cfg.GetRepositories(), action.TriggerRestore.GetRepositoryId())
		if repo == nil {
			result.Success = false
			result.Error = fmt.Sprintf("repository %s not found", action.TriggerRestore.GetRepositoryId())
			return result
		}
		err := resticExec.Restore(context.Background(), executor.Repository{
			ID:       repo.GetId(),
			Name:     repo.GetName(),
			Type:     repo.GetType(),
			Path:     repo.GetPath(),
			Password: repo.GetPassword(),
		}, action.TriggerRestore.GetSnapshotId(), action.TriggerRestore.GetPaths(), action.TriggerRestore.GetTarget())
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			return result
		}
		result.Success = true

	default:
		result.Success = false
		result.Error = "unknown command type"
	}

	return result
}

// findRepo looks up a repository by ID from the config.
func findRepo(repos []*backupv1.Repository, id string) *backupv1.Repository {
	for _, r := range repos {
		if r.GetId() == id {
			return r
		}
	}
	return nil
}
