package configpush

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

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
			slog.Warn("failed to parse file_browser_blocked_paths setting", "error", parseErr)
		}
	}

	// Build per-command timeouts: start from globals, then layer the per-agent
	// override (if any) on top. Zero-valued fields mean "use the agent's
	// compiled-in default" so callers only need to set non-default values.
	commandTimeouts, err := r.resolveCommandTimeouts(ctx, agent)
	if err != nil {
		return fmt.Errorf("resolve command timeouts: %w", err)
	}

	// Build outbox config: globals from settings + per-agent override.
	outboxCfg, err := r.resolveOutbox(ctx, agent)
	if err != nil {
		return fmt.Errorf("resolve outbox: %w", err)
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
			slog.Error("failed to load hooks for plan", "plan_id", p.ID, "error", hookErr)
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
					slog.Error("failed to resolve script for hook", "script_id", *h.ScriptID, "hook_id", h.ID, "error", scriptErr)
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
		CommandTimeouts:         commandTimeouts,
		Outbox:                  outboxCfg,
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

	slog.Info("pushed config to agent", "config_version", version, "agent_id", agentID)
	return nil
}

// PushConfigToAllAgents pushes config to every connected and approved agent.
func (r *Resolver) PushConfigToAllAgents(ctx context.Context) {
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		slog.Error("failed to list agents for config push", "error", err)
		return
	}

	for _, a := range agents {
		if a.Status == "approved" && r.mgr.IsOnline(a.ID) {
			if err := r.PushConfigToAgent(ctx, a.ID); err != nil {
				slog.Error("failed to push config to agent", "agent_id", a.ID, "error", err)
			}
		}
	}
}

// CommandTimeouts mirrors backupv1.CommandTimeouts as a JSON-friendly struct
// so it can be persisted in the agents.command_timeouts column and accepted
// from the API. A zero/missing field means "use the agent's compiled-in
// default".
type CommandTimeouts struct {
	BackupSecs           int32 `json:"backup_secs,omitempty"`
	RestoreSecs          int32 `json:"restore_secs,omitempty"`
	ListSnapshotsSecs    int32 `json:"list_snapshots_secs,omitempty"`
	BrowseSnapshotSecs   int32 `json:"browse_snapshot_secs,omitempty"`
	BrowseFilesystemSecs int32 `json:"browse_filesystem_secs,omitempty"`
	DefaultSecs          int32 `json:"default_secs,omitempty"`
}

// commandTimeoutSettingKeys maps the proto field index to its global setting key.
var commandTimeoutSettingKeys = map[string]string{
	"backup":            "command_timeout_backup_seconds",
	"restore":           "command_timeout_restore_seconds",
	"list_snapshots":    "command_timeout_list_snapshots_seconds",
	"browse_snapshot":   "command_timeout_browse_snapshot_seconds",
	"browse_filesystem": "command_timeout_browse_filesystem_seconds",
	"default":           "command_timeout_default_seconds",
}

// resolveCommandTimeouts loads the global command-timeout settings from the
// settings table, then layers any per-agent override (stored as JSON in
// agents.command_timeouts) on top. Returns nil if no values are configured
// at either level so the agent falls back to its own defaults.
func (r *Resolver) resolveCommandTimeouts(ctx context.Context, agent *database.Agent) (*backupv1.CommandTimeouts, error) {
	loadInt := func(key string) (int32, error) {
		val, err := r.db.GetSetting(ctx, key)
		if err != nil {
			return 0, err
		}
		if val == nil {
			return 0, nil
		}
		var n int32
		if parseErr := json.Unmarshal([]byte(*val), &n); parseErr != nil || n <= 0 {
			return 0, nil
		}
		return n, nil
	}

	merged := &CommandTimeouts{}
	for field, key := range commandTimeoutSettingKeys {
		n, err := loadInt(key)
		if err != nil {
			return nil, fmt.Errorf("get %s: %w", key, err)
		}
		switch field {
		case "backup":
			merged.BackupSecs = n
		case "restore":
			merged.RestoreSecs = n
		case "list_snapshots":
			merged.ListSnapshotsSecs = n
		case "browse_snapshot":
			merged.BrowseSnapshotSecs = n
		case "browse_filesystem":
			merged.BrowseFilesystemSecs = n
		case "default":
			merged.DefaultSecs = n
		}
	}

	if agent.CommandTimeouts != nil && *agent.CommandTimeouts != "" {
		var override CommandTimeouts
		if err := json.Unmarshal([]byte(*agent.CommandTimeouts), &override); err != nil {
			slog.Warn("failed to parse per-agent command_timeouts", "agent_id", agent.ID, "error", err)
		} else {
			if override.BackupSecs > 0 {
				merged.BackupSecs = override.BackupSecs
			}
			if override.RestoreSecs > 0 {
				merged.RestoreSecs = override.RestoreSecs
			}
			if override.ListSnapshotsSecs > 0 {
				merged.ListSnapshotsSecs = override.ListSnapshotsSecs
			}
			if override.BrowseSnapshotSecs > 0 {
				merged.BrowseSnapshotSecs = override.BrowseSnapshotSecs
			}
			if override.BrowseFilesystemSecs > 0 {
				merged.BrowseFilesystemSecs = override.BrowseFilesystemSecs
			}
			if override.DefaultSecs > 0 {
				merged.DefaultSecs = override.DefaultSecs
			}
		}
	}

	// If everything is zero, return nil so the agent uses its built-in defaults.
	if merged.BackupSecs == 0 && merged.RestoreSecs == 0 && merged.ListSnapshotsSecs == 0 &&
		merged.BrowseSnapshotSecs == 0 && merged.BrowseFilesystemSecs == 0 && merged.DefaultSecs == 0 {
		return nil, nil
	}

	return &backupv1.CommandTimeouts{
		BackupSecs:           merged.BackupSecs,
		RestoreSecs:          merged.RestoreSecs,
		ListSnapshotsSecs:    merged.ListSnapshotsSecs,
		BrowseSnapshotSecs:   merged.BrowseSnapshotSecs,
		BrowseFilesystemSecs: merged.BrowseFilesystemSecs,
		DefaultSecs:          merged.DefaultSecs,
	}, nil
}

// OutboxOverrides mirrors backupv1.OutboxConfig as a JSON-friendly struct so
// it can be persisted in agents.outbox_overrides and accepted from the API.
// A zero/missing field means "use the agent's compiled-in default" for that
// knob. The in-memory channel capacity (OUTBOX_MEMORY_MAX) is intentionally
// not configurable from the server — Go channels cannot be resized at
// runtime, so it stays an agent-side bootstrap env var.
type OutboxOverrides struct {
	SpillMaxRows        int32 `json:"spill_max_rows,omitempty"`
	SpillRetentionSecs  int32 `json:"spill_retention_secs,omitempty"`
	FlushIntervalSecs   int32 `json:"flush_interval_secs,omitempty"`
	DeliveryTimeoutSecs int32 `json:"delivery_timeout_secs,omitempty"`
	MaxAttempts         int32 `json:"max_attempts,omitempty"`
}

// outboxSettingKeys maps each OutboxOverrides field to its global setting key.
var outboxSettingKeys = map[string]string{
	"spill_max_rows":        "outbox_spill_max_rows",
	"spill_retention_secs":  "outbox_spill_retention_seconds",
	"flush_interval_secs":   "outbox_flush_interval_seconds",
	"delivery_timeout_secs": "outbox_delivery_timeout_seconds",
	"max_attempts":          "outbox_max_attempts",
}

// resolveOutbox loads the global outbox settings and layers any per-agent
// override (stored as JSON in agents.outbox_overrides) on top. Returns nil if
// no values are configured at either level so the agent uses its own defaults.
func (r *Resolver) resolveOutbox(ctx context.Context, agent *database.Agent) (*backupv1.OutboxConfig, error) {
	loadInt := func(key string) (int32, error) {
		val, err := r.db.GetSetting(ctx, key)
		if err != nil {
			return 0, err
		}
		if val == nil {
			return 0, nil
		}
		var n int32
		if parseErr := json.Unmarshal([]byte(*val), &n); parseErr != nil || n <= 0 {
			return 0, nil
		}
		return n, nil
	}

	merged := &OutboxOverrides{}
	for field, key := range outboxSettingKeys {
		n, err := loadInt(key)
		if err != nil {
			return nil, fmt.Errorf("get %s: %w", key, err)
		}
		switch field {
		case "spill_max_rows":
			merged.SpillMaxRows = n
		case "spill_retention_secs":
			merged.SpillRetentionSecs = n
		case "flush_interval_secs":
			merged.FlushIntervalSecs = n
		case "delivery_timeout_secs":
			merged.DeliveryTimeoutSecs = n
		case "max_attempts":
			merged.MaxAttempts = n
		}
	}

	if agent.OutboxOverrides != nil && *agent.OutboxOverrides != "" {
		var override OutboxOverrides
		if err := json.Unmarshal([]byte(*agent.OutboxOverrides), &override); err != nil {
			slog.Warn("failed to parse per-agent outbox_overrides", "agent_id", agent.ID, "error", err)
		} else {
			if override.SpillMaxRows > 0 {
				merged.SpillMaxRows = override.SpillMaxRows
			}
			if override.SpillRetentionSecs > 0 {
				merged.SpillRetentionSecs = override.SpillRetentionSecs
			}
			if override.FlushIntervalSecs > 0 {
				merged.FlushIntervalSecs = override.FlushIntervalSecs
			}
			if override.DeliveryTimeoutSecs > 0 {
				merged.DeliveryTimeoutSecs = override.DeliveryTimeoutSecs
			}
			if override.MaxAttempts > 0 {
				merged.MaxAttempts = override.MaxAttempts
			}
		}
	}

	if merged.SpillMaxRows == 0 && merged.SpillRetentionSecs == 0 && merged.FlushIntervalSecs == 0 &&
		merged.DeliveryTimeoutSecs == 0 && merged.MaxAttempts == 0 {
		return nil, nil
	}

	return &backupv1.OutboxConfig{
		SpillMaxRows:        merged.SpillMaxRows,
		SpillRetentionSecs:  merged.SpillRetentionSecs,
		FlushIntervalSecs:   merged.FlushIntervalSecs,
		DeliveryTimeoutSecs: merged.DeliveryTimeoutSecs,
		MaxAttempts:         merged.MaxAttempts,
	}, nil
}
