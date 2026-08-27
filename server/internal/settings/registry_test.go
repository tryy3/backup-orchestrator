package settings_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tryy3/backup-orchestrator/server/internal/settings"
)

func TestValidate_UnknownKey(t *testing.T) {
	t.Parallel()
	errs := settings.Validate(map[string]json.RawMessage{
		"not_a_real_key": json.RawMessage(`1`),
	})
	require.Len(t, errs, 1)
	assert.Equal(t, "not_a_real_key", errs[0].Key)
	assert.Contains(t, errs[0].Message, "unknown")
}

func TestValidate_CollectsAllErrors(t *testing.T) {
	t.Parallel()
	errs := settings.Validate(map[string]json.RawMessage{
		"bogus":                      json.RawMessage(`1`),
		"heartbeat_interval_seconds": json.RawMessage(`2`),
		"job_history_days":           json.RawMessage(`"nope"`),
	})
	require.Len(t, errs, 3)
	keys := []string{errs[0].Key, errs[1].Key, errs[2].Key}
	assert.ElementsMatch(t, []string{"bogus", "heartbeat_interval_seconds", "job_history_days"}, keys)
}

func TestValidate_AcceptsValidSubset(t *testing.T) {
	t.Parallel()
	errs := settings.Validate(map[string]json.RawMessage{
		"heartbeat_interval_seconds": json.RawMessage(`30`),
		"file_browser_blocked_paths": json.RawMessage(`["/proc","/sys"]`),
		"default_retention":          json.RawMessage(`{"keep_last":5,"keep_hourly":0,"keep_daily":0,"keep_weekly":0,"keep_monthly":0,"keep_yearly":0}`),
	})
	assert.Empty(t, errs)
}

func TestValidate_RejectsNullDefaultRetention(t *testing.T) {
	t.Parallel()
	errs := settings.Validate(map[string]json.RawMessage{
		"default_retention": json.RawMessage(`null`),
	})
	require.Len(t, errs, 1)
	assert.Equal(t, "default_retention", errs[0].Key)
	assert.NotEmpty(t, errs[0].Message)
}

func TestValidate_RejectsNullBlockedPaths(t *testing.T) {
	t.Parallel()
	errs := settings.Validate(map[string]json.RawMessage{
		"file_browser_blocked_paths": json.RawMessage(`null`),
	})
	require.Len(t, errs, 1)
	assert.Equal(t, "file_browser_blocked_paths", errs[0].Key)
	assert.NotEmpty(t, errs[0].Message)
}

func TestValidate_AcceptsEmptyBlockedPathsArray(t *testing.T) {
	t.Parallel()
	errs := settings.Validate(map[string]json.RawMessage{
		"file_browser_blocked_paths": json.RawMessage(`[]`),
	})
	assert.Empty(t, errs)
}

func TestDefaultJSON_Heartbeat(t *testing.T) {
	t.Parallel()
	raw, ok := settings.DefaultJSON("heartbeat_interval_seconds")
	require.True(t, ok)
	var n int
	require.NoError(t, json.Unmarshal(raw, &n))
	assert.Equal(t, 30, n)
}

func TestDefaultInt_KnownKeys(t *testing.T) {
	t.Parallel()
	heartbeat, ok := settings.DefaultInt("heartbeat_interval_seconds")
	require.True(t, ok)
	assert.Equal(t, 30, heartbeat)

	hookTimeout, ok := settings.DefaultInt("default_hook_timeout_seconds")
	require.True(t, ok)
	assert.Equal(t, 60, hookTimeout)
}

func TestDefaultInt_UnknownKey(t *testing.T) {
	t.Parallel()
	_, ok := settings.DefaultInt("not_a_real_key")
	assert.False(t, ok)
}

func TestKeys_IncludesAllKnown(t *testing.T) {
	t.Parallel()
	keys := settings.Keys()
	assert.Contains(t, keys, "default_retention")
	assert.Contains(t, keys, "outbox_max_attempts")
	assert.Len(t, keys, 20)
}
