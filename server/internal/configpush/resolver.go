package configpush

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"

	"github.com/tryy3/backup-orchestrator/server/internal/agentmgr"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

// Resolver builds and pushes config to agents.
type Resolver struct {
	db  *database.DB
	mgr *agentmgr.Manager
}

// New creates a new config resolver.
func New(db *database.DB, mgr *agentmgr.Manager) *Resolver {
	return &Resolver{db: db, mgr: mgr}
}

// PushConfigToAgent builds a complete config for the given agent and sends it.
func (r *Resolver) PushConfigToAgent(ctx context.Context, agentID string) error {
	if !r.mgr.IsOnline(agentID) {
		return nil // Agent not connected, skip.
	}

	agent, err := r.db.GetAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("get agent: %w", err)
	}
	if agent == nil || agent.Status != "approved" {
		return nil
	}

	// Load all plans for this agent.
	plans, err := r.db.ListPlans(ctx, agentID)
	if err != nil {
		return fmt.Errorf("list plans: %w", err)
	}

	// Collect all repository IDs referenced by plans.
	repoIDSet := make(map[string]bool)
	for _, p := range plans {
		for _, rid := range p.RepositoryIDs {
			repoIDSet[rid] = true
		}
	}

	// Batch-load all referenced repositories in a single query.
	repoIDs := make([]string, 0, len(repoIDSet))
	for rid := range repoIDSet {
		repoIDs = append(repoIDs, rid)
	}
	repoMap, err := r.db.GetRepositoriesByIDs(ctx, repoIDs)
	if err != nil {
		return fmt.Errorf("get repositories: %w", err)
	}

	// Load default retention from settings.
	var defaultRetention *backupv1.RetentionPolicy
	retVal, err := r.db.GetSetting(ctx, "default_retention")
	if err != nil {
		return fmt.Errorf("get default retention: %w", err)
	}
	if retVal != nil {
		var rp database.RetentionPolicy
		if parseErr := json.Unmarshal([]byte(*retVal), &rp); parseErr == nil {
			defaultRetention = &backupv1.RetentionPolicy{
				KeepLast:    int32(rp.KeepLast),
				KeepHourly:  int32(rp.KeepHourly),
				KeepDaily:   int32(rp.KeepDaily),
				KeepWeekly:  int32(rp.KeepWeekly),
				KeepMonthly: int32(rp.KeepMonthly),
				KeepYearly:  int32(rp.KeepYearly),
			}
		}
	}

	// Load default hook timeout from settings (default: 60 seconds).
	defaultHookTimeout := int32(60)
	hookTimeoutVal, err := r.db.GetSetting(ctx, "default_hook_timeout_seconds")
	if err != nil {
		return fmt.Errorf("get default hook timeout: %w", err)
	}
	if hookTimeoutVal != nil {
		var t int32
		if parseErr := json.Unmarshal([]byte(*hookTimeoutVal), &t); parseErr == nil && t > 0 {
			defaultHookTimeout = t
		}
	}

	// Load heartbeat interval from settings (default: 30 seconds).
	heartbeatInterval := int32(30)
	hbVal, err := r.db.GetSetting(ctx, "heartbeat_interval_seconds")
	if err != nil {
		return fmt.Errorf("get heartbeat interval: %w", err)
	}
	if hbVal != nil {
		var hb int32
		if parseErr := json.Unmarshal([]byte(*hbVal), &hb); parseErr == nil && hb > 0 {
			heartbeatInterval = hb
		}
	}

	// Load file browser blocked paths from settings.
	var blockedPaths []string
	bpVal, err := r.db.GetSetting(ctx, "file_browser_blocked_paths")
	if err != nil {
		return fmt.Errorf("get blocked paths: %w", err)
	}
	if bpVal != nil {
		if parseErr := json.Unmarshal([]byte(*bpVal), &blockedPaths); parseErr != nil {
			log.Printf("Warning: failed to parse file_browser_blocked_paths setting: %v", parseErr)
		}
	}

	// Build protobuf repositories.
	var pbRepos []*backupv1.Repository
	for _, repo := range repoMap {
		pbRepos = append(pbRepos, &backupv1.Repository{
			Id:       repo.ID,
			Name:     repo.Name,
			Type:     repo.Type,
			Path:     repo.Path,
			Password: repo.Password,
		})
	}

	// Build protobuf plans with resolved hooks.
	var pbPlans []*backupv1.BackupPlan
	for _, p := range plans {
		pbPlan := &backupv1.BackupPlan{
			Id:                p.ID,
			Name:              p.Name,
			Paths:             p.Paths,
			Excludes:          p.Excludes,
			Tags:              p.Tags,
			RepositoryIds:     p.RepositoryIDs,
			Schedule:          p.Schedule,
			ForgetAfterBackup: p.ForgetAfterBackup,
			PruneAfterForget:  p.PruneAfterForget,
			Enabled:           p.Enabled,
		}
		if p.PruneSchedule != nil {
			pbPlan.PruneSchedule = *p.PruneSchedule
		}
		if p.Retention != nil {
			pbPlan.Retention = &backupv1.RetentionPolicy{
				KeepLast:    int32(p.Retention.KeepLast),
				KeepHourly:  int32(p.Retention.KeepHourly),
				KeepDaily:   int32(p.Retention.KeepDaily),
				KeepWeekly:  int32(p.Retention.KeepWeekly),
				KeepMonthly: int32(p.Retention.KeepMonthly),
				KeepYearly:  int32(p.Retention.KeepYearly),
			}
		}

		// Resolve hooks for this plan.
		hooks, hookErr := r.db.ListHooks(ctx, p.ID)
		if hookErr != nil {
			log.Printf("Failed to load hooks for plan %s: %v", p.ID, hookErr)
			continue
		}

		for _, h := range hooks {
			resolved := &backupv1.ResolvedHook{
				OnEvent:   h.OnEvent,
				SortOrder: int32(h.SortOrder),
				OnError:   "continue",
			}

			if h.ScriptID != nil {
				// Resolve from script.
				script, scriptErr := r.db.GetScript(ctx, *h.ScriptID)
				if scriptErr != nil || script == nil {
					log.Printf("Failed to resolve script %s for hook %s: %v", *h.ScriptID, h.ID, scriptErr)
					continue
				}
				resolved.Name = script.Name
				resolved.Type = script.Type
				resolved.Command = script.Command
				resolved.TimeoutSeconds = int32(script.Timeout)
				resolved.OnError = script.OnError

				// Apply per-hook overrides.
				if h.Timeout != nil {
					resolved.TimeoutSeconds = int32(*h.Timeout)
				}
				if h.OnError != nil {
					resolved.OnError = *h.OnError
				}
			} else {
				// Inline hook.
				if h.Command != nil {
					resolved.Command = *h.Command
				}
				if h.Type != nil {
					resolved.Type = *h.Type
				} else {
					resolved.Type = "command"
				}
				resolved.Name = resolved.OnEvent + "-inline"
				resolved.TimeoutSeconds = defaultHookTimeout
				if h.Timeout != nil {
					resolved.TimeoutSeconds = int32(*h.Timeout)
				}
				if h.OnError != nil {
					resolved.OnError = *h.OnError
				}
			}

			pbPlan.Hooks = append(pbPlan.Hooks, resolved)
		}

		pbPlans = append(pbPlans, pbPlan)
	}

	// Increment config version.
	version, err := r.db.UpdateConfigVersion(ctx, agentID)
	if err != nil {
		return fmt.Errorf("update config version: %w", err)
	}

	// Build config message.
	config := &backupv1.AgentConfig{
		ConfigVersion:           int32(version),
		Repositories:            pbRepos,
		BackupPlans:             pbPlans,
		DefaultRetention:        defaultRetention,
		HeartbeatIntervalSecs:   heartbeatInterval,
		FileBrowserBlockedPaths: blockedPaths,
	}
	if agent.RcloneConfig != nil {
		config.RcloneConfig = *agent.RcloneConfig
	}

	// Send to agent.
	msg := &backupv1.ServerMessage{
		Payload: &backupv1.ServerMessage_Config{
			Config: config,
		},
	}

	if err := r.mgr.Send(agentID, msg); err != nil {
		return fmt.Errorf("send config to agent %s: %w", agentID, err)
	}

	log.Printf("Pushed config version %d to agent %s", version, agentID)
	return nil
}

// PushConfigToAllAgents pushes config to every connected and approved agent.
func (r *Resolver) PushConfigToAllAgents(ctx context.Context) {
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		log.Printf("Failed to list agents for config push: %v", err)
		return
	}

	for _, a := range agents {
		if a.Status == "approved" && r.mgr.IsOnline(a.ID) {
			if err := r.PushConfigToAgent(ctx, a.ID); err != nil {
				log.Printf("Failed to push config to agent %s: %v", a.ID, err)
			}
		}
	}
}
