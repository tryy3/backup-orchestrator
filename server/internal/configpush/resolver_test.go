package configpush

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tryy3/backup-orchestrator/server/internal/agentmgr"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
)

func openResolverTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(filepath.Join(t.TempDir(), "t.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestResolveCommandTimeouts_NoStoredValues_ReturnsNil(t *testing.T) {
	t.Parallel()
	db := openResolverTestDB(t)
	r := New(db, agentmgr.New())

	got, err := r.resolveCommandTimeouts(context.Background(), &database.Agent{ID: "agent-1"})
	require.NoError(t, err)
	assert.Nil(t, got, "missing stored values must not force registry defaults into agent push")
}

func TestResolveOutbox_NoStoredValues_ReturnsNil(t *testing.T) {
	t.Parallel()
	db := openResolverTestDB(t)
	r := New(db, agentmgr.New())

	got, err := r.resolveOutbox(context.Background(), &database.Agent{ID: "agent-1"})
	require.NoError(t, err)
	assert.Nil(t, got, "missing stored values must not force registry defaults into agent push")
}

func TestPushConfigToAgent_UsesRegistryDefaultsForHeartbeatAndHookTimeout(t *testing.T) {
	t.Parallel()
	db := openResolverTestDB(t)
	ctx := context.Background()

	require.NoError(t, db.CreateAgent(ctx, &database.Agent{
		ID:       "agent-1",
		Name:     "test-agent",
		Hostname: "localhost",
		Status:   "approved",
	}))

	plan := &database.BackupPlan{
		Name:     "test-plan",
		AgentID:  "agent-1",
		Schedule: "0 * * * *",
		Paths:    []string{"/tmp"},
		Enabled:  true,
	}
	require.NoError(t, db.CreatePlan(ctx, plan))

	inlineCmd := "echo pre-backup"
	require.NoError(t, db.CreateHook(ctx, &database.PlanHook{
		PlanID:    plan.ID,
		OnEvent:   "pre-backup",
		SortOrder: 0,
		Command:   &inlineCmd,
	}))

	mgr := agentmgr.New()
	sendCh := make(chan *backupv1.ServerMessage, 1)
	mgr.Register("agent-1", sendCh)
	t.Cleanup(func() { mgr.Unregister("agent-1") })

	r := New(db, mgr)
	require.NoError(t, r.PushConfigToAgent(ctx, "agent-1"))

	select {
	case msg := <-sendCh:
		cfg := msg.GetConfig()
		require.NotNil(t, cfg)
		assert.Equal(t, int32(30), cfg.HeartbeatIntervalSecs)
		require.Len(t, cfg.BackupPlans, 1)
		require.Len(t, cfg.BackupPlans[0].Hooks, 1)
		assert.Equal(t, int32(60), cfg.BackupPlans[0].Hooks[0].TimeoutSeconds)
		assert.Nil(t, cfg.CommandTimeouts)
		assert.Nil(t, cfg.Outbox)
	case <-time.After(time.Second):
		t.Fatal("config message not received")
	}
}

func TestPushConfigToAgent_StoredSettingsOverrideRegistryDefaults(t *testing.T) {
	t.Parallel()
	db := openResolverTestDB(t)
	ctx := context.Background()

	require.NoError(t, db.CreateAgent(ctx, &database.Agent{
		ID:       "agent-1",
		Name:     "test-agent",
		Hostname: "localhost",
		Status:   "approved",
	}))
	require.NoError(t, db.SetSetting(ctx, "heartbeat_interval_seconds", "45"))

	mgr := agentmgr.New()
	sendCh := make(chan *backupv1.ServerMessage, 1)
	mgr.Register("agent-1", sendCh)
	t.Cleanup(func() { mgr.Unregister("agent-1") })

	r := New(db, mgr)
	require.NoError(t, r.PushConfigToAgent(ctx, "agent-1"))

	select {
	case msg := <-sendCh:
		cfg := msg.GetConfig()
		require.NotNil(t, cfg)
		assert.Equal(t, int32(45), cfg.HeartbeatIntervalSecs)
	case <-time.After(time.Second):
		t.Fatal("config message not received")
	}
}
