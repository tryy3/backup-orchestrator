package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTursoIntegration_MigrateAndAgentRoundTrip(t *testing.T) {
	url := os.Getenv("BACKUP_TEST_TURSO_URL")
	token := os.Getenv("BACKUP_TEST_TURSO_AUTH_TOKEN")
	if url == "" {
		t.Skip("set BACKUP_TEST_TURSO_URL (and token if needed) to run Turso integration")
	}
	ctx := context.Background()
	driver := DriverTursoSync
	if os.Getenv("BACKUP_TEST_TURSO_DRIVER") == "turso" {
		driver = DriverTurso
	}
	path := filepath.Join(t.TempDir(), "sync.db")
	db, err := Open(Options{
		Driver:        driver,
		Path:          path,
		URL:           url,
		AuthToken:     token,
		SyncInterval:  time.Hour,
		EncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	})
	require.NoError(t, err)
	defer db.Close()

	id := "itest-agent"
	require.NoError(t, db.CreateAgent(ctx, &Agent{
		ID: id, Name: "itest", Hostname: "localhost", Status: "pending",
	}))
	got, err := db.GetAgent(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "itest", got.Name)
}
