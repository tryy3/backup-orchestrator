package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tryy3/backup-orchestrator/server/internal/agentmgr"
	"github.com/tryy3/backup-orchestrator/server/internal/configpush"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/settings"
)

func openAPITestDB(t *testing.T) *database.DB {
	t.Helper()
	db, err := database.New(filepath.Join(t.TempDir(), "t.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestGetSettings_ReturnsAllKeysWithDefaults(t *testing.T) {
	t.Parallel()
	db := openAPITestDB(t)

	handler := getSettingsHandler(db)
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var body map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))
	assert.Len(t, body, 20)

	var heartbeat int
	require.NoError(t, json.Unmarshal(body["heartbeat_interval_seconds"], &heartbeat))
	assert.Equal(t, 30, heartbeat)
}

func TestUpdateSettings_UnknownAndInvalid_NoWrite(t *testing.T) {
	t.Parallel()
	db := openAPITestDB(t)
	mgr := agentmgr.New()
	resolver := configpush.New(db, mgr)

	handler := updateSettingsHandler(db, resolver)
	payload := []byte(`{"bogus":1,"heartbeat_interval_seconds":2}`)
	req := httptest.NewRequest(http.MethodPut, "/settings", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	var resp struct {
		Errors []settings.FieldError `json:"errors"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.Len(t, resp.Errors, 2)

	keys := make([]string, 0, len(resp.Errors))
	for _, e := range resp.Errors {
		keys = append(keys, e.Key)
	}
	assert.ElementsMatch(t, []string{"bogus", "heartbeat_interval_seconds"}, keys)

	val, err := db.GetSetting(context.Background(), "heartbeat_interval_seconds")
	require.NoError(t, err)
	assert.Nil(t, val)
}

func TestUpdateSettings_Valid_WritesAndReturnsResolved(t *testing.T) {
	t.Parallel()
	db := openAPITestDB(t)
	mgr := agentmgr.New()
	resolver := configpush.New(db, mgr)

	handler := updateSettingsHandler(db, resolver)
	payload := []byte(`{"heartbeat_interval_seconds":45}`)
	req := httptest.NewRequest(http.MethodPut, "/settings", bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var body map[string]json.RawMessage
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&body))

	var heartbeat int
	require.NoError(t, json.Unmarshal(body["heartbeat_interval_seconds"], &heartbeat))
	assert.Equal(t, 45, heartbeat)

	val, err := db.GetSetting(context.Background(), "heartbeat_interval_seconds")
	require.NoError(t, err)
	require.NotNil(t, val)
	assert.JSONEq(t, `45`, *val)
}
