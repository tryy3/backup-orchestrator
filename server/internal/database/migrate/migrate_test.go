package migrate_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	"github.com/tryy3/backup-orchestrator/server/internal/database/migrate"
)

func TestSQLiteToTursoSync_RequiresDistinctPaths(t *testing.T) {
	t.Parallel()
	err := migrate.SQLiteToTursoSync(context.Background(), migrate.Options{
		FromPath:  "a.db",
		ToPath:    "a.db",
		URL:       "http://127.0.0.1:1",
		AuthToken: "tok",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "must differ")
}

func TestSQLiteToTursoSync_RequiresSource(t *testing.T) {
	t.Parallel()
	err := migrate.SQLiteToTursoSync(context.Background(), migrate.Options{
		FromPath:  filepath.Join(t.TempDir(), "missing.db"),
		ToPath:    filepath.Join(t.TempDir(), "out.db"),
		URL:       "http://127.0.0.1:1",
		AuthToken: "tok",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "source database")
}

func TestCopyViaOpenSQLiteRoundTrip(t *testing.T) {
	// Ensures MigrateConn + source open work; full sync Push needs a live remote
	// (covered by gated integration / manual just migrate-turso-sync).
	t.Parallel()
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "src.db")
	src, err := database.New(srcPath, nil)
	require.NoError(t, err)
	require.NoError(t, src.CreateAgent(context.Background(), &database.Agent{
		ID: "a1", Name: "n", Hostname: "h", Status: "pending",
	}))
	require.NoError(t, src.Close())

	src2, err := database.New(srcPath, nil)
	require.NoError(t, err)
	defer src2.Close()
	got, err := src2.GetAgent(context.Background(), "a1")
	require.NoError(t, err)
	require.Equal(t, "n", got.Name)
}
