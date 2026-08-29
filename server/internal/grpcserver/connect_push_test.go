package grpcserver

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tryy3/backup-orchestrator/server/internal/agentmgr"
	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	backupv1 "github.com/tryy3/backup-orchestrator/server/internal/gen/backup/v1"
)

func TestRequestConfigPushOnConnect_ApprovedSchedulesPush(t *testing.T) {
	t.Parallel()
	db, err := database.New(filepath.Join(t.TempDir(), "t.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	ctx := t.Context()
	require.NoError(t, db.CreateAgent(ctx, &database.Agent{
		ID: "agent-1", Name: "a", Hostname: "h", Status: "approved",
	}))

	mgr := agentmgr.New()
	sendCh := make(chan *backupv1.ServerMessage, 1)
	mgr.Register("agent-1", sendCh)
	t.Cleanup(func() { mgr.Unregister("agent-1") })

	resolver := configpush.New(db, mgr)
	requestConfigPushOnConnect(resolver, "agent-1", "approved")

	select {
	case msg := <-sendCh:
		require.NotNil(t, msg.GetConfig())
	case <-time.After(2 * time.Second):
		t.Fatal("expected config push for approved agent")
	}
}

func TestRequestConfigPushOnConnect_PendingDoesNotPush(t *testing.T) {
	t.Parallel()
	db, err := database.New(filepath.Join(t.TempDir(), "t.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mgr := agentmgr.New()
	sendCh := make(chan *backupv1.ServerMessage, 1)
	mgr.Register("agent-1", sendCh)
	t.Cleanup(func() { mgr.Unregister("agent-1") })

	resolver := configpush.New(db, mgr)
	requestConfigPushOnConnect(resolver, "agent-1", "pending")

	select {
	case <-sendCh:
		t.Fatal("pending agent must not receive config")
	case <-time.After(100 * time.Millisecond):
	}
}
