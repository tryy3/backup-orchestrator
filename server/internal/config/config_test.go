package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetAllowedOrigins_Default(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", "")

	got := getAllowedOrigins()
	assert.Equal(t, defaultAllowedOrigins, got)
}

func TestGetAllowedOrigins_Single(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", "https://app.example.com")

	got := getAllowedOrigins()
	assert.Equal(t, []string{"https://app.example.com"}, got)
}

func TestGetAllowedOrigins_Multiple(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", "https://app.example.com,https://admin.example.com")

	got := getAllowedOrigins()
	assert.Equal(t, []string{"https://app.example.com", "https://admin.example.com"}, got)
}

func TestGetAllowedOrigins_TrimsWhitespace(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", " https://app.example.com , https://admin.example.com ")

	got := getAllowedOrigins()
	assert.Equal(t, []string{"https://app.example.com", "https://admin.example.com"}, got)
}

func TestGetAllowedOrigins_EmptyEntriesIgnored(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", "https://app.example.com,,https://admin.example.com")

	got := getAllowedOrigins()
	assert.Equal(t, []string{"https://app.example.com", "https://admin.example.com"}, got)
}

func TestGetAllowedOrigins_AllWhitespaceFallsBackToDefault(t *testing.T) {
	t.Setenv("BACKUP_ALLOWED_ORIGINS", "  ,  ")

	got := getAllowedOrigins()
	assert.Equal(t, defaultAllowedOrigins, got)
}

func TestParseDriver_DefaultEmpty(t *testing.T) {
	d, err := ParseDriver("")
	require.NoError(t, err)
	assert.Equal(t, DriverSQLite, d)
}

func TestParseDriver_Valid(t *testing.T) {
	d, err := ParseDriver("turso-sync")
	require.NoError(t, err)
	assert.Equal(t, DriverTursoSync, d)
}

func TestParseDriver_Invalid(t *testing.T) {
	_, err := ParseDriver("postgres")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BACKUP_DB_DRIVER")
}

func TestLoad_SQLiteDefaultNoNewVars(t *testing.T) {
	t.Setenv("BACKUP_DB_DRIVER", "")
	t.Setenv("BACKUP_DB_PATH", filepath.Join(t.TempDir(), "server.db"))
	t.Setenv("BACKUP_ENCRYPTION_KEY", strings.Repeat("ab", 32))
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, DriverSQLite, cfg.Driver)
	assert.Equal(t, 30*time.Second, cfg.SyncInterval)
}

func TestLoad_TursoRequiresURLTokenAndKeyEnv(t *testing.T) {
	t.Setenv("BACKUP_DB_DRIVER", "turso")
	t.Setenv("BACKUP_DB_URL", "https://example.turso.io")
	t.Setenv("BACKUP_DB_AUTH_TOKEN", "tok")
	t.Setenv("BACKUP_ENCRYPTION_KEY", "")
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BACKUP_ENCRYPTION_KEY")
}

func TestLoad_TursoSyncRequiresPathURLToken(t *testing.T) {
	t.Setenv("BACKUP_DB_DRIVER", "turso-sync")
	t.Setenv("BACKUP_DB_PATH", "")
	t.Setenv("BACKUP_DB_URL", "https://example.turso.io")
	t.Setenv("BACKUP_DB_AUTH_TOKEN", "tok")
	t.Setenv("BACKUP_ENCRYPTION_KEY", strings.Repeat("ab", 32))
	_, err := Load()
	require.Error(t, err)
}

func TestLoad_TursoSyncInvalidInterval(t *testing.T) {
	t.Setenv("BACKUP_DB_DRIVER", "turso-sync")
	t.Setenv("BACKUP_DB_PATH", filepath.Join(t.TempDir(), "server.db"))
	t.Setenv("BACKUP_DB_URL", "https://example.turso.io")
	t.Setenv("BACKUP_DB_AUTH_TOKEN", "tok")
	t.Setenv("BACKUP_DB_SYNC_INTERVAL", "nope")
	t.Setenv("BACKUP_ENCRYPTION_KEY", strings.Repeat("ab", 32))
	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BACKUP_DB_SYNC_INTERVAL")
}
