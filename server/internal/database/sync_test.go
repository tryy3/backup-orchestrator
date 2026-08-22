package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errRemoteDown = errors.New("remote down")

func TestEvaluateSyncStartup(t *testing.T) {
	t.Parallel()
	require.NoError(t, evaluateSyncStartup(false, nil))
	require.NoError(t, evaluateSyncStartup(true, nil))
	require.NoError(t, evaluateSyncStartup(true, errRemoteDown))
	err := evaluateSyncStartup(false, errRemoteDown)
	require.Error(t, err)
	assert.ErrorIs(t, err, errRemoteDown)
	assert.Contains(t, err.Error(), "empty")
}

func TestLocalDBReady(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.db")
	assert.False(t, localDBReady(missing))
	empty := filepath.Join(dir, "empty.db")
	require.NoError(t, os.WriteFile(empty, nil, 0o600))
	assert.False(t, localDBReady(empty))
	ready := filepath.Join(dir, "ready.db")
	require.NoError(t, os.WriteFile(ready, []byte("x"), 0o600))
	assert.True(t, localDBReady(ready))
}

type fakeSync struct {
	pulls   atomic.Int32
	pushes  atomic.Int32
	pullErr error
	pushErr error
}

func (f *fakeSync) Pull(ctx context.Context) error {
	f.pulls.Add(1)
	return f.pullErr
}
func (f *fakeSync) Push(ctx context.Context) error {
	f.pushes.Add(1)
	return f.pushErr
}

func TestSyncLoop_DoesNotFailWhenRemoteErrors(t *testing.T) {
	db := newTestDB(t)
	fake := &fakeSync{pullErr: errRemoteDown, pushErr: errRemoteDown}
	db.syncer = fake
	db.startSyncLoop(20 * time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	_, err := db.ExecContext(context.Background(), "SELECT 1 FROM agents LIMIT 0")
	require.NoError(t, err)
	_, lastErr := db.SyncStatus()
	require.Error(t, lastErr)
	require.NoError(t, db.Close())
	assert.GreaterOrEqual(t, fake.pushes.Load(), int32(1)) // Close best-effort Push
}

func TestClose_PushesThenCloses(t *testing.T) {
	db := newTestDB(t)
	fake := &fakeSync{}
	db.syncer = fake
	require.NoError(t, db.Close())
	assert.Equal(t, int32(1), fake.pushes.Load())
	_, err := db.ExecContext(context.Background(), "SELECT 1")
	assert.Error(t, err)
}
