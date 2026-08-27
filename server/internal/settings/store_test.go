package settings_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/settings"
)

func openSettingsTestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(filepath.Join(t.TempDir(), "t.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestLoadResolved_EmptyDB_AllDefaults(t *testing.T) {
	t.Parallel()
	db := openSettingsTestDB(t)
	got, err := settings.LoadResolved(context.Background(), db)
	require.NoError(t, err)
	require.Len(t, got, len(settings.Keys()))
	raw, ok := settings.DefaultJSON("heartbeat_interval_seconds")
	require.True(t, ok)
	assert.JSONEq(t, string(raw), string(got["heartbeat_interval_seconds"]))
}

func TestLoadResolved_StoredOverrideWins(t *testing.T) {
	t.Parallel()
	db := openSettingsTestDB(t)
	require.NoError(t, db.SetSetting(context.Background(), "heartbeat_interval_seconds", `45`))
	got, err := settings.LoadResolved(context.Background(), db)
	require.NoError(t, err)
	assert.JSONEq(t, `45`, string(got["heartbeat_interval_seconds"]))
}

func TestLoadResolved_CorruptStored_FallsBackToDefault(t *testing.T) {
	t.Parallel()
	db := openSettingsTestDB(t)
	require.NoError(t, db.SetSetting(context.Background(), "heartbeat_interval_seconds", `"nope"`))
	got, err := settings.LoadResolved(context.Background(), db)
	require.NoError(t, err)
	raw, _ := settings.DefaultJSON("heartbeat_interval_seconds")
	assert.JSONEq(t, string(raw), string(got["heartbeat_interval_seconds"]))
}

func TestApply_WritesValidatedKeys(t *testing.T) {
	t.Parallel()
	db := openSettingsTestDB(t)
	input := map[string]json.RawMessage{
		"heartbeat_interval_seconds": json.RawMessage(`55`),
	}
	require.Empty(t, settings.Validate(input))
	require.NoError(t, settings.Apply(context.Background(), db, input))
	val, err := db.GetSetting(context.Background(), "heartbeat_interval_seconds")
	require.NoError(t, err)
	require.NotNil(t, val)
	assert.JSONEq(t, `55`, *val)
}
