package migrate

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tryy3/backup-orchestrator/server/internal/database"
	_ "modernc.org/sqlite"
)

func TestRemoteAppTables_EmptyAndPopulated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	emptyPath := filepath.Join(t.TempDir(), "empty.db")
	migrated, err := database.New(emptyPath, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = migrated.Close() })

	// Fresh migrate creates all app tables — treat as "has app tables".
	found, err := remoteAppTables(ctx, migrated.DB)
	require.NoError(t, err)
	require.NotEmpty(t, found)
	require.Contains(t, found, "agents")

	barePath := filepath.Join(t.TempDir(), "bare.db")
	bare, err := sql.Open("sqlite", barePath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = bare.Close() })
	_, err = bare.ExecContext(ctx, "SELECT 1")
	require.NoError(t, err)

	found, err = remoteAppTables(ctx, bare)
	require.NoError(t, err)
	require.Empty(t, found)
}
