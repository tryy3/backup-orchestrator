package database

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
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
	assert.Contains(t, err.Error(), "expected schema")
}

func TestLocalDBReady(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx := context.Background()

	for _, name := range []string{"missing", "empty", "non-sqlite"} {
		path := filepath.Join(dir, name+".db")
		if name == "empty" {
			require.NoError(t, os.WriteFile(path, nil, 0o600))
		}
		if name == "non-sqlite" {
			require.NoError(t, os.WriteFile(path, []byte("not a database"), 0o600))
		}
		sqlDB, err := sql.Open("sqlite", path)
		require.NoError(t, err)
		assert.False(t, localDBReady(ctx, sqlDB), name)
		require.NoError(t, sqlDB.Close())
	}

	path := filepath.Join(dir, "ready.db")
	ready, err := New(path, nil)
	require.NoError(t, err)
	assert.True(t, localDBReady(ctx, ready.DB))
	require.NoError(t, ready.Close())
}

type fakeSync struct {
	pulls   atomic.Int32
	pushes  atomic.Int32
	pullErr error
	pushErr error
}

type shutdownSync struct {
	pullStarted     chan struct{}
	loopPushStarted chan struct{}
	releaseLoopPush chan struct{}
	startOnce       sync.Once
	loopPushOnce    sync.Once
	pushHasTimeout  atomic.Bool
}

func newShutdownSync() *shutdownSync {
	return &shutdownSync{
		pullStarted:     make(chan struct{}),
		loopPushStarted: make(chan struct{}),
		releaseLoopPush: make(chan struct{}),
	}
}

func (s *shutdownSync) Pull(ctx context.Context) error {
	s.startOnce.Do(func() { close(s.pullStarted) })
	<-ctx.Done()
	return ctx.Err()
}

func (s *shutdownSync) Push(ctx context.Context) error {
	_, hasDeadline := ctx.Deadline()
	if hasDeadline {
		s.pushHasTimeout.Store(true)
		return nil
	}
	s.loopPushOnce.Do(func() { close(s.loopPushStarted) })
	<-s.releaseLoopPush
	return nil
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

func TestOpen_TursoSync_RefusesLocalWithoutSchemaWhenPullFails(t *testing.T) {
	for _, test := range []struct {
		name    string
		content []byte
	}{
		{name: "empty"},
		{name: "non-empty", content: []byte("not a database")},
	} {
		t.Run(test.name, func(t *testing.T) {
			orig := newTursoSync
			t.Cleanup(func() { newTursoSync = orig })
			path := filepath.Join(t.TempDir(), "local.db")
			require.NoError(t, os.WriteFile(path, test.content, 0o600))
			newTursoSync = func(opts Options) (*sql.DB, remoteSync, error) {
				sqlDB, err := sql.Open("sqlite", opts.Path)
				require.NoError(t, err)
				return sqlDB, &fakeSync{pullErr: errRemoteDown}, nil
			}
			_, err := Open(Options{Driver: DriverTursoSync, Path: path})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "expected schema")
		})
	}
}

func TestOpen_TursoSync_StartsWhenLocalReadyAndPullFails(t *testing.T) {
	orig := newTursoSync
	t.Cleanup(func() { newTursoSync = orig })
	path := filepath.Join(t.TempDir(), "ready.db")
	seed, err := New(path, nil)
	require.NoError(t, err)
	require.NoError(t, seed.Close())
	newTursoSync = func(opts Options) (*sql.DB, remoteSync, error) {
		sqlDB, err := sql.Open("sqlite", opts.Path)
		require.NoError(t, err)
		return sqlDB, &fakeSync{pullErr: errRemoteDown}, nil
	}
	db, err := Open(Options{Driver: DriverTursoSync, Path: path, SyncInterval: time.Hour})
	require.NoError(t, err)
	defer db.Close()
	_, qerr := db.ExecContext(context.Background(), "SELECT 1 FROM agents LIMIT 0")
	require.NoError(t, qerr)
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

func TestClose_WaitsForSyncLoopAndBoundsFinalPush(t *testing.T) {
	db := newTestDB(t)
	syncer := newShutdownSync()
	db.syncer = syncer
	db.startSyncLoop(time.Millisecond)

	select {
	case <-syncer.pullStarted:
	case <-time.After(time.Second):
		t.Fatal("sync loop did not start")
	}

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- db.Close()
	}()

	select {
	case <-syncer.loopPushStarted:
	case <-time.After(time.Second):
		t.Fatal("in-flight sync push did not start")
	}
	select {
	case err := <-closeResult:
		t.Fatalf("Close returned before sync loop exited: %v", err)
	default:
	}

	close(syncer.releaseLoopPush)
	require.NoError(t, <-closeResult)
	assert.True(t, syncer.pushHasTimeout.Load())
}
