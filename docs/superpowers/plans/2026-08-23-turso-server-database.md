# Turso Server Database Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add optional `turso` (remote-only) and `turso-sync` (local file + Push/Pull) server backends while keeping `sqlite` as the default and leaving the agent `state.db` unchanged.

**Architecture:** Keep the existing `*database.DB` query API. Add `database.Options` + `Open` to select a backend. `New(path, key)` remains a sqlite convenience wrapper so current tests stay valid. Sync is a background loop behind a small `remoteSync` interface so startup and error behavior can be tested without Turso Cloud. Request-path SQL never calls Turso in `turso-sync`.

**Tech Stack:** Go 1.26, `database/sql`, `modernc.org/sqlite` (default), `turso.tech/database/tursogo` (sync), `turso.tech/database/tursogo-serverless` (remote-only), testify, optional local `tursodb --sync-server` for integration tests.

**Spec:** [docs/superpowers/specs/2026-08-23-turso-server-database-design.md](../specs/2026-08-23-turso-server-database-design.md)

## Global Constraints

- Agent `state.db`, `config.json`, and `identity.json` must not change.
- `BACKUP_DB_DRIVER` unset or empty means `sqlite`. Existing installs need no new env vars.
- No CGO. Do not add `github.com/tursodatabase/go-libsql`.
- Remote-only and sync target Turso Database (`tursogo` / `tursogo-serverless`), not classic libSQL (`libsql-client-go`).
- v1 is one server writer. Do not run `turso` and `turso-sync` against the same cloud DB at once.
- `New(path, encryptionKey)` must keep working as sqlite-only so `server/internal/database/db_test.go` does not need a mass rewrite.
- Encryption key stays host-local. `turso` must not auto-generate an ephemeral key.
- Pin Turso module versions in `server/go.mod`; do not use `@latest` in commits.
- Prefer root `just` recipes: `just test-server`, `just fmt`, `just vet`.

## File map

| File | Responsibility |
|---|---|
| `server/internal/config/config.go` | Driver, URL, token, sync interval; encryption-key rules per driver |
| `server/internal/config/config_test.go` | Validation and key-resolution tests |
| `server/internal/database/options.go` | `Driver`, `Options`, `Open`, sqlite `New` wrapper |
| `server/internal/database/db.go` | `*DB` fields, migrate/encrypt after open, `Close` |
| `server/internal/database/pragma.go` | sqlite-only PRAGMAs; dialect-safe column check |
| `server/internal/database/sync.go` | `remoteSync`, loop, status, startup decision |
| `server/internal/database/backend_turso.go` | Open remote-only and turso-sync via official SDKs |
| `server/internal/database/sync_test.go` | Loop + startup matrix with fakes |
| `server/internal/database/turso_integration_test.go` | Gated migrate + CRUD against local/cloud Turso |
| `server/cmd/server/main.go` | Pass `Options` from config; log driver |
| `docs/database-schema.md` | Three server backends; agent unchanged |
| `docs/turso-server-database.md` | Env vars, startup table, migration runbook |
| `.env.dev` | Commented Turso examples |
| `README.md` | Document the three `BACKUP_DB_*` vars |

Do not touch `agent/internal/database/`.

---

### Task 1: Config — driver, validation, encryption rules

**Files:**
- Modify: `server/internal/config/config.go`
- Modify: `server/internal/config/config_test.go`

**Interfaces:**
- Consumes: existing `Load()`, `loadEncryptionKey(dbPath string)`
- Produces:
  - `type Driver string` with `DriverSQLite = "sqlite"`, `DriverTurso = "turso"`, `DriverTursoSync = "turso-sync"`
  - `Config` fields: `Driver Driver`, `DBPath string`, `DBURL string`, `DBAuthToken string`, `SyncInterval time.Duration`, plus existing fields
  - `func ParseDriver(raw string) (Driver, error)`
  - `func loadEncryptionKey(driver Driver, dbPath string) ([]byte, error)`

- [ ] **Step 1: Write the failing tests**

Add to `server/internal/config/config_test.go` (keep existing origin tests). Use `t.Setenv` and a temp dir when a key file would be written:

```go
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
```

Import `path/filepath`, `strings`, `time`, and `github.com/stretchr/testify/require`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/config/ -count=1`
Expected: FAIL — `ParseDriver` undefined and `Load` does not validate Turso fields.

- [ ] **Step 3: Implement config**

In `config.go` add `time` to imports. Extend `Config` and replace `Load` / `loadEncryptionKey` as follows (keep `getAllowedOrigins` and `getenv` unchanged):

```go
type Driver string

const (
	DriverSQLite    Driver = "sqlite"
	DriverTurso     Driver = "turso"
	DriverTursoSync Driver = "turso-sync"
)

func ParseDriver(raw string) (Driver, error) {
	switch strings.TrimSpace(raw) {
	case "", string(DriverSQLite):
		return DriverSQLite, nil
	case string(DriverTurso):
		return DriverTurso, nil
	case string(DriverTursoSync):
		return DriverTursoSync, nil
	default:
		return "", fmt.Errorf("BACKUP_DB_DRIVER must be sqlite, turso, or turso-sync, got %q", raw)
	}
}

func Load() (*Config, error) {
	driver, err := ParseDriver(os.Getenv("BACKUP_DB_DRIVER"))
	if err != nil {
		return nil, err
	}
	dbPath := getenv("BACKUP_DB_PATH", "/var/lib/backup-orchestrator/server.db")
	url := strings.TrimSpace(os.Getenv("BACKUP_DB_URL"))
	token := strings.TrimSpace(os.Getenv("BACKUP_DB_AUTH_TOKEN"))
	interval := 30 * time.Second
	if raw := os.Getenv("BACKUP_DB_SYNC_INTERVAL"); raw != "" {
		interval, err = time.ParseDuration(raw)
		if err != nil || interval <= 0 {
			return nil, fmt.Errorf("BACKUP_DB_SYNC_INTERVAL must be a positive duration: %w", err)
		}
	}
	switch driver {
	case DriverTurso:
		if url == "" || token == "" {
			return nil, fmt.Errorf("BACKUP_DB_URL and BACKUP_DB_AUTH_TOKEN are required when BACKUP_DB_DRIVER=turso")
		}
	case DriverTursoSync:
		if dbPath == "" || url == "" || token == "" {
			return nil, fmt.Errorf("BACKUP_DB_PATH, BACKUP_DB_URL, and BACKUP_DB_AUTH_TOKEN are required when BACKUP_DB_DRIVER=turso-sync")
		}
	}
	key, err := loadEncryptionKey(driver, dbPath)
	if err != nil {
		return nil, fmt.Errorf("load encryption key: %w", err)
	}
	return &Config{
		Driver:         driver,
		DBPath:         dbPath,
		DBURL:          url,
		DBAuthToken:    token,
		SyncInterval:   interval,
		HTTPPort:       getenv("BACKUP_HTTP_PORT", "8080"),
		GRPCPort:       getenv("BACKUP_GRPC_PORT", "8443"),
		AllowedOrigins: getAllowedOrigins(),
		EncryptionKey:  key,
	}, nil
}

func loadEncryptionKey(driver Driver, dbPath string) ([]byte, error) {
	if hexKey := os.Getenv("BACKUP_ENCRYPTION_KEY"); hexKey != "" {
		key, err := hex.DecodeString(hexKey)
		if err != nil {
			return nil, fmt.Errorf("BACKUP_ENCRYPTION_KEY is not valid hex: %w", err)
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("BACKUP_ENCRYPTION_KEY must be 64 hex characters (32 bytes), got %d bytes", len(key))
		}
		return key, nil
	}
	if driver == DriverTurso {
		return nil, fmt.Errorf("BACKUP_ENCRYPTION_KEY is required when BACKUP_DB_DRIVER=turso")
	}
	// existing file-then-generate logic using filepath.Dir(dbPath)/encryption.key
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/config/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/config/config.go server/internal/config/config_test.go
git commit -m "$(cat <<'EOF'
feat(server): add Turso database driver config

Parse BACKUP_DB_DRIVER and require URL/token (and a durable encryption key) only for the Turso backends.
EOF
)"
```

---

### Task 2: `Options` + `Open` for sqlite; keep `New` as wrapper

**Files:**
- Create: `server/internal/database/options.go`
- Modify: `server/internal/database/db.go`
- Create: `server/internal/database/pragma.go` (move sqlite PRAGMAs + pool setup here)
- Test: `server/internal/database/db_test.go` (existing tests must still pass via `New`)

**Interfaces:**
- Consumes: Task 1 `config.Driver` values as strings (`"sqlite"`, `"turso"`, `"turso-sync"`)
- Produces:
  - `type Driver string` in database package (mirror config constants; do not import config into database)
  - `type Options struct { Driver Driver; Path string; URL string; AuthToken string; SyncInterval time.Duration; EncryptionKey []byte }`
  - `func Open(opts Options) (*DB, error)`
  - `func New(path string, encryptionKey []byte) (*DB, error)` calls `Open(Options{Driver: DriverSQLite, Path: path, EncryptionKey: encryptionKey})`

- [ ] **Step 1: Write a failing test for Open sqlite**

Add to `db_test.go`:

```go
func TestOpen_SQLiteEquivalentToNew(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "open.db")
	db, err := Open(Options{Driver: DriverSQLite, Path: path})
	require.NoError(t, err)
	defer db.Close()
	_, err = db.ExecContext(context.Background(), "SELECT 1 FROM agents LIMIT 0")
	require.NoError(t, err)
}

func TestOpen_UnknownDriver(t *testing.T) {
	t.Parallel()
	_, err := Open(Options{Driver: "postgres"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "driver")
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `cd server && go test ./internal/database/ -count=1 -run 'TestOpen_'`
Expected: FAIL — `Open` / `Options` undefined.

- [ ] **Step 3: Implement Open for sqlite only**

`options.go`:

```go
package database

import (
	"fmt"
	"time"
)

type Driver string

const (
	DriverSQLite    Driver = "sqlite"
	DriverTurso     Driver = "turso"
	DriverTursoSync Driver = "turso-sync"
)

type Options struct {
	Driver        Driver
	Path          string
	URL           string
	AuthToken     string
	SyncInterval  time.Duration
	EncryptionKey []byte
}

func Open(opts Options) (*DB, error) {
	if opts.Driver == "" {
		opts.Driver = DriverSQLite
	}
	switch opts.Driver {
	case DriverSQLite:
		return openSQLite(opts)
	case DriverTurso, DriverTursoSync:
		return nil, fmt.Errorf("database driver %q is not implemented yet", opts.Driver)
	default:
		return nil, fmt.Errorf("unknown database driver %q", opts.Driver)
	}
}

func New(path string, encryptionKey []byte) (*DB, error) {
	return Open(Options{Driver: DriverSQLite, Path: path, EncryptionKey: encryptionKey})
}
```

Move the current `New` body (sql.Open, pool, three PRAGMAs, migrate, encrypt) into `openSQLite` in `pragma.go` or `db.go`. `DB` stays:

```go
type DB struct {
	*sql.DB
	encryptionKey []byte
}
```

Do not add Turso imports yet.

- [ ] **Step 4: Run database tests**

Run: `cd server && go test ./internal/database/ -count=1`
Expected: PASS (including existing `TestNew_*`)

- [ ] **Step 5: Commit**

```bash
git add server/internal/database/options.go server/internal/database/db.go server/internal/database/pragma.go server/internal/database/db_test.go
git commit -m "$(cat <<'EOF'
feat(server): open sqlite through database.Options

Keep New(path, key) as the sqlite wrapper so existing tests and call sites stay valid.
EOF
)"
```

---

### Task 3: Dialect-safe column presence check

**Files:**
- Modify: `server/internal/database/migrations.go` (`addColumnIfMissing`)
- Modify: `server/internal/database/db_test.go`

**Interfaces:**
- Consumes: `*DB.QueryContext` / `ExecContext`
- Produces: `func (db *DB) hasColumn(ctx context.Context, table, column string) (bool, error)` that works when `PRAGMA table_info` is unavailable (returns an error — then fall back to `SELECT <column> FROM <table> LIMIT 0`)

- [ ] **Step 1: Write tests**

```go
func TestHasColumn_ExistingAndMissing(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	ok, err := db.hasColumn(ctx, "agents", "command_timeouts")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = db.hasColumn(ctx, "agents", "not_a_real_column")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestAddColumnIfMissing_Idempotent(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	ctx := context.Background()
	require.NoError(t, db.addColumnIfMissing(ctx, "agents", "command_timeouts", "TEXT"))
	require.NoError(t, db.addColumnIfMissing(ctx, "agents", "command_timeouts", "TEXT"))
}
```

- [ ] **Step 2: Run tests**

Run: `cd server && go test ./internal/database/ -count=1 -run 'TestHasColumn|TestAddColumnIfMissing'`
Expected: FAIL until `hasColumn` exists.

- [ ] **Step 3: Implement**

```go
func (db *DB) hasColumn(ctx context.Context, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err == nil {
		defer func() { _ = rows.Close() }()
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		for rows.Next() {
			if scanErr := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); scanErr != nil {
				return false, fmt.Errorf("scan column info: %w", scanErr)
			}
			if name == column {
				return true, nil
			}
		}
		return false, rows.Err()
	}
	// Remote engines may reject PRAGMA. A successful projection means the column exists.
	q := fmt.Sprintf("SELECT %s FROM %s LIMIT 0", column, table)
	if _, selErr := db.ExecContext(ctx, q); selErr == nil {
		return true, nil
	}
	return false, nil
}

func (db *DB) addColumnIfMissing(ctx context.Context, table, column, columnType string) error {
	ok, err := db.hasColumn(ctx, table, column)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnType)
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		// Concurrent migrate or engine that already has the column.
		ok, checkErr := db.hasColumn(ctx, table, column)
		if checkErr == nil && ok {
			return nil
		}
		return fmt.Errorf("alter table %s add %s: %w", table, column, err)
	}
	return nil
}
```

Table/column names are identifiers from our migrations only, never user input.

- [ ] **Step 4: Run**

Run: `cd server && go test ./internal/database/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/database/migrations.go server/internal/database/db_test.go
git commit -m "$(cat <<'EOF'
fix(server): check columns without relying on sqlite-only PRAGMA success

Keep PRAGMA table_info for sqlite and fall back to a projection so Turso migrations can add columns safely.
EOF
)"
```

---

### Task 4: Sync loop, status, and Close — tested with a fake

**Files:**
- Create: `server/internal/database/sync.go`
- Create: `server/internal/database/sync_test.go`
- Modify: `server/internal/database/db.go` (`DB` fields + `Close`)

**Interfaces:**
- Consumes: `*sql.DB.Close`
- Produces:
  - `type remoteSync interface { Pull(ctx context.Context) error; Push(ctx context.Context) error }`
  - `func evaluateSyncStartup(localReady bool, pullErr error) error`
  - `func localDBReady(path string) bool`
  - `func (db *DB) startSyncLoop(interval time.Duration)` — no-op if `db.syncer == nil` or interval <= 0
  - `func (db *DB) SyncStatus() (lastOK time.Time, lastErr error)`
  - `Close` best-effort `Push` then stop loop then `sql.DB.Close`

- [ ] **Step 1: Write failing tests**

`sync_test.go`:

```go
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
	pulls atomic.Int32
	pushes atomic.Int32
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
```

- [ ] **Step 2: Run**

Run: `cd server && go test ./internal/database/ -count=1 -run 'TestEvaluateSyncStartup|TestLocalDBReady|TestSyncLoop|TestClose_Pushes'`
Expected: FAIL — symbols undefined.

- [ ] **Step 3: Implement `sync.go` and extend `DB`**

```go
func evaluateSyncStartup(localReady bool, pullErr error) error {
	if pullErr == nil {
		return nil
	}
	if localReady {
		return nil
	}
	return fmt.Errorf("turso-sync: remote unreachable and local database is empty: %w", pullErr)
}

func localDBReady(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Size() > 0
}
```

`startSyncLoop` stores a cancel func on `DB`, ticks `Pull` then `Push`, records status under a mutex, logs errors, never returns them to callers. Default interval if `opts.SyncInterval == 0` is `30 * time.Second` (only when starting a real loop from `Open`, not from the 20ms test).

`Close`:

```go
func (db *DB) Close() error {
	if db.syncCancel != nil {
		db.syncCancel()
	}
	if db.syncer != nil {
		_ = db.syncer.Push(context.Background())
	}
	if db.DB == nil {
		return nil
	}
	return db.DB.Close()
}
```

Existing `TestDB_Close_Idempotent` must still pass: second `Close` must not panic (nil-safe cancel / already-closed `sql.DB`).

- [ ] **Step 4: Run**

Run: `cd server && go test ./internal/database/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/database/sync.go server/internal/database/sync_test.go server/internal/database/db.go
git commit -m "$(cat <<'EOF'
feat(server): add Turso sync loop with offline startup rules

Keep request-path SQL on the local file and treat Push/Pull failures as status, not handler errors.
EOF
)"
```

---

### Task 5: Wire `turso-sync` Open path with an injectable factory

**Files:**
- Modify: `server/internal/database/options.go`
- Modify: `server/internal/database/sync.go`
- Modify: `server/internal/database/sync_test.go`

**Interfaces:**
- Consumes: `evaluateSyncStartup`, `localDBReady`, `remoteSync`, `startSyncLoop`
- Produces:
  - `type syncFactory func(opts Options) (*sql.DB, remoteSync, error)`
  - package-level `var newTursoSync syncFactory` — tests replace this; production sets it in Task 6
  - `func openTursoSync(opts Options) (*DB, error)`

Startup sequence for `DriverTursoSync`:

1. `ready := localDBReady(opts.Path)`
2. `sqlDB, syncer, err := newTursoSync(opts)` — if this fails and `ready`, still error (cannot open local file). If this fails and `!ready`, wrap as refuse-to-start.
3. `pullErr := syncer.Pull(ctx)`
4. `if err := evaluateSyncStartup(ready, pullErr); err != nil { sqlDB.Close(); return nil, err }`
5. If `pullErr != nil && ready`, log a warning and continue.
6. Build `*DB`, `migrate`, optional encrypt, `startSyncLoop(opts.SyncInterval)`, return.

- [ ] **Step 1: Write failing tests**

```go
func TestOpen_TursoSync_RefusesEmptyLocalWhenPullFails(t *testing.T) {
	orig := newTursoSync
	t.Cleanup(func() { newTursoSync = orig })
	newTursoSync = func(opts Options) (*sql.DB, remoteSync, error) {
		sqlDB, err := sql.Open("sqlite", opts.Path)
		require.NoError(t, err)
		return sqlDB, &fakeSync{pullErr: errRemoteDown}, nil
	}
	_, err := Open(Options{Driver: DriverTursoSync, Path: filepath.Join(t.TempDir(), "missing.db")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
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
```

Use `database/sql` + `modernc` sqlite in the fake factory so we do not need tursogo yet.

- [ ] **Step 2: Run**

Run: `cd server && go test ./internal/database/ -count=1 -run 'TestOpen_TursoSync'`
Expected: FAIL — `openTursoSync` not wired or `newTursoSync` nil.

- [ ] **Step 3: Implement `openTursoSync` and switch `Open`**

```go
func Open(opts Options) (*DB, error) {
	if opts.Driver == "" {
		opts.Driver = DriverSQLite
	}
	switch opts.Driver {
	case DriverSQLite:
		return openSQLite(opts)
	case DriverTurso:
		return nil, fmt.Errorf("database driver %q is not implemented yet", opts.Driver)
	case DriverTursoSync:
		return openTursoSync(opts)
	default:
		return nil, fmt.Errorf("unknown database driver %q", opts.Driver)
	}
}
```

If `newTursoSync == nil` in production before Task 6, `openTursoSync` returns `fmt.Errorf("turso-sync factory is not registered")`. Tests set the var.

After a successful open with `pullErr == nil`, call `syncer.Push` once (ignore error, record status) so a first-time local schema can upload.

- [ ] **Step 4: Run**

Run: `cd server && go test ./internal/database/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/database/options.go server/internal/database/sync.go server/internal/database/sync_test.go
git commit -m "$(cat <<'EOF'
feat(server): open turso-sync with offline-start rules

Refuse an empty local file when Pull fails; keep serving from an existing local file.
EOF
)"
```

---

### Task 6: Real `tursogo` factory + adopt-vs-import spike

**Files:**
- Create: `server/internal/database/backend_turso.go`
- Modify: `server/go.mod`, `server/go.sum`
- Modify: `docs/turso-server-database.md` (create in Task 10 if not yet; otherwise add a “File adopt” subsection here)

**Interfaces:**
- Consumes: `Options`, `newTursoSync`
- Produces: `func init()` or `func registerTursoSyncFactory()` setting `newTursoSync` to the real SDK
- Spike result written into the ops doc: **adopt supported** or **import + Pull only**

- [ ] **Step 1: Add the module**

Run from `server/`:

```bash
go get turso.tech/database/tursogo
```

Pin the resolved version in `go.mod`. Do not enable CGO.

- [ ] **Step 2: Spike — can tursogo open a modernc file?**

Script (run once, do not commit the binary output):

```bash
cd server
go test ./internal/database/ -count=1 -run TestNew
# then a small TestTursoAdoptModernc (temporary) that:
# 1. New(path) sqlite, CreateAgent, Close
# 2. turso.NewTursoSyncDb with Path=path, RemoteUrl=http://127.0.0.1:1, BootstrapIfEmpty=false
# 3. Connect + SELECT name FROM agents
```

If `tursodb` is installed, also try `tursodb :memory: --sync-server 127.0.0.1:18080` and `RemoteUrl=http://127.0.0.1:18080` with no token.

Record in the ops doc:

- Adopt works: production runbook step is “point `BACKUP_DB_PATH` at the existing file and Push”.
- Adopt fails: production runbook step is “import into Turso, new empty local path, Pull”.

Delete the temporary spike test if it is environment-specific. Keep a skipped test only if it is deterministic without a network.

- [ ] **Step 3: Implement the real factory**

```go
func openTursoSyncSDK(opts Options) (*sql.DB, remoteSync, error) {
	ctx := context.Background()
	bootstrap := false
	syncDB, err := turso.NewTursoSyncDb(ctx, turso.TursoSyncDbConfig{
		Path:             opts.Path,
		RemoteUrl:        opts.URL,
		AuthToken:        opts.AuthToken,
		BootstrapIfEmpty: &bootstrap,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open turso-sync: %w", err)
	}
	sqlDB, err := syncDB.Connect(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("connect turso-sync: %w", err)
	}
	// If Connect does not return *sql.DB, adapt with the SDK's documented connector.
	// sqlDB.SetMaxOpenConns: use 8 open / 2 idle for the local Turso engine (not the sqlite 25).
	return sqlDB, tursoSyncAdapter{db: syncDB}, nil
}

type tursoSyncAdapter struct {
	db *turso.TursoSyncDb
}

func (a tursoSyncAdapter) Pull(ctx context.Context) error { _, err := a.db.Pull(ctx); return err }
func (a tursoSyncAdapter) Push(ctx context.Context) error { return a.db.Push(ctx) }
```

Set `newTursoSync = openTursoSyncSDK` in `init()` of `backend_turso.go`. Tests that replace `newTursoSync` still run first in each test via `t.Cleanup`.

If `Connect` returns a type other than `*sql.DB`, do **not** change `type DB struct { *sql.DB }`. Use `sql.OpenDB` or the SDK helper so `*database.DB` still embeds `*sql.DB`.

- [ ] **Step 4: Run unit tests (no cloud required)**

Run: `cd server && go test ./internal/database/ -count=1`
Expected: PASS. Fake factory tests still override `newTursoSync`.

- [ ] **Step 5: Commit**

```bash
git add server/go.mod server/go.sum server/internal/database/backend_turso.go docs/turso-server-database.md
git commit -m "$(cat <<'EOF'
feat(server): connect turso-sync through tursogo

Register the official sync factory and record whether an existing modernc SQLite file can be adopted.
EOF
)"
```

---

### Task 7: Remote-only `turso` backend

**Files:**
- Modify: `server/internal/database/backend_turso.go`
- Modify: `server/internal/database/options.go` (`Open` case `DriverTurso`)
- Modify: `server/go.mod` / `server/go.sum`

**Interfaces:**
- Consumes: `Options.URL`, `Options.AuthToken`, `Options.EncryptionKey`
- Produces: `func openTursoRemote(opts Options) (*DB, error)` using `sql.OpenDB(tursoserverless.NewConnector(url, token))`
- Pool: `SetMaxOpenConns(10)`, `SetMaxIdleConns(2)`, `SetConnMaxLifetime(5 * time.Minute)` — do not copy sqlite’s 25.

- [ ] **Step 1: Add a unit test that does not need the network**

```go
func TestOpen_Turso_RequiresURL(t *testing.T) {
	t.Parallel()
	_, err := Open(Options{Driver: DriverTurso})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "BACKUP_DB_URL")
}
```

`openTursoRemote` validates URL and token even if the SDK would also fail.

- [ ] **Step 2: Run**

Run: `cd server && go test ./internal/database/ -count=1 -run TestOpen_Turso_RequiresURL`
Expected: FAIL until validation exists.

- [ ] **Step 3: Implement**

```bash
cd server && go get turso.tech/database/tursogo-serverless
```

```go
func openTursoRemote(opts Options) (*DB, error) {
	if opts.URL == "" || opts.AuthToken == "" {
		return nil, fmt.Errorf("BACKUP_DB_URL and BACKUP_DB_AUTH_TOKEN are required for driver %q", DriverTurso)
	}
	sqlDB := sql.OpenDB(turso.NewConnector(opts.URL, opts.AuthToken))
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping turso: %w", err)
	}
	db := &DB{DB: sqlDB, encryptionKey: opts.EncryptionKey}
	if err := db.migrate(context.Background()); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	if len(opts.EncryptionKey) == 32 {
		if err := db.migrateEncryption(context.Background()); err != nil {
			_ = sqlDB.Close()
			return nil, err
		}
	}
	return db, nil
}
```

Import alias: `turso "turso.tech/database/tursogo-serverless"` so it does not collide with `tursogo`. If `NewConnector`’s signature differs, follow the installed module’s docs and keep `sql.OpenDB`.

- [ ] **Step 4: Run unit tests**

Run: `cd server && go test ./internal/database/ ./internal/config/ -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/go.mod server/go.sum server/internal/database/backend_turso.go server/internal/database/options.go server/internal/database/db_test.go
git commit -m "$(cat <<'EOF'
feat(server): add remote-only Turso database driver

Open tursogo-serverless with a smaller pool and fail startup when the remote cannot be pinged.
EOF
)"
```

---

### Task 8: Wire the server process

**Files:**
- Modify: `server/cmd/server/main.go`

**Interfaces:**
- Consumes: `config.Config` (`Driver`, `DBPath`, `DBURL`, `DBAuthToken`, `SyncInterval`, `EncryptionKey`)
- Produces: `database.Open(database.Options{...})` instead of `database.New(cfg.DBPath, cfg.EncryptionKey)`

- [ ] **Step 1: Change main (no new unit test; this is a wiring change)**

Replace the open + log block:

```go
db, err := database.Open(database.Options{
	Driver:        database.Driver(cfg.Driver),
	Path:          cfg.DBPath,
	URL:           cfg.DBURL,
	AuthToken:     cfg.DBAuthToken,
	SyncInterval:  cfg.SyncInterval,
	EncryptionKey: cfg.EncryptionKey,
})
if err != nil {
	slog.Error("failed to open database", "error", err)
	os.Exit(1)
}
slog.Info("database opened", "driver", cfg.Driver, "path", cfg.DBPath)
```

`defer db.Close()` already exists and now best-effort Pushes. Do not add a second Close.

- [ ] **Step 2: Compile**

Run: `cd server && go build -o /tmp/backup-server ./cmd/server`
Expected: success

- [ ] **Step 3: Smoke sqlite default**

Run: `BACKUP_DB_PATH=/tmp/bo-plan-smoke.db BACKUP_ENCRYPTION_KEY=$(python -c 'print("ab"*32)') /tmp/backup-server` and interrupt after “database opened” / listen logs. Or compile-only if ports are busy.

Expected log contains `driver=sqlite`.

- [ ] **Step 4: Commit**

```bash
git add server/cmd/server/main.go
git commit -m "$(cat <<'EOF'
feat(server): select the database backend from config

Pass driver, URL, token, and sync interval into database.Open so Turso modes are reachable at process start.
EOF
)"
```

---

### Task 9: Gated integration test

**Files:**
- Create: `server/internal/database/turso_integration_test.go`

**Interfaces:**
- Consumes: `Open(Options{Driver: DriverTursoSync or DriverTurso, ...})`, `CreateAgent`, `GetAgent`
- Produces: skipped test unless env is set

- [ ] **Step 1: Write the test**

```go
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
```

Local server (document in the ops doc):

```bash
tursodb :memory: --sync-server 127.0.0.1:18080
BACKUP_TEST_TURSO_URL=http://127.0.0.1:18080 BACKUP_TEST_TURSO_DRIVER=turso-sync \
  go test ./internal/database/ -count=1 -run TestTursoIntegration
```

Do not add Turso Cloud secrets to CI in v1. The skip-by-default test is the automated non-sqlite hook; running it locally (or later CI with `tursodb`) satisfies the spec’s “at least one automated non-sqlite path” once executed in the implementer’s verification.

- [ ] **Step 2: Run default suite (must skip, not fail)**

Run: `just test-server`
Expected: PASS; the integration test skipped.

- [ ] **Step 3: Run integration if `tursodb` is available**

If `tursodb` is not installed, note that in the PR and still ship the skipped test.

- [ ] **Step 4: Commit**

```bash
git add server/internal/database/turso_integration_test.go
git commit -m "$(cat <<'EOF'
test(server): add a gated Turso migrate and agent round-trip

Skip unless BACKUP_TEST_TURSO_URL is set so default CI stays sqlite-only.
EOF
)"
```

---

### Task 10: Docs and env examples

**Files:**
- Modify: `docs/database-schema.md` (title + short “Supported backends” section at the top)
- Create: `docs/turso-server-database.md`
- Modify: `.env.dev`
- Modify: `README.md` (env table rows for `BACKUP_DB_DRIVER`, `BACKUP_DB_URL`, `BACKUP_DB_AUTH_TOKEN`, `BACKUP_DB_SYNC_INTERVAL`)

**Interfaces:**
- Consumes: spike result from Task 6 (adopt vs import)
- Produces: operator runbook with exactly one supported sqlite → turso-sync method

- [ ] **Step 1: Write `docs/turso-server-database.md`**

Must include:

- The three drivers and env vars from the spec
- Startup table from the spec
- Encryption-key rules (`turso` requires `BACKUP_ENCRYPTION_KEY`)
- One-writer warning
- Migration runbook with the **Task 6 spike outcome** as the only sqlite → turso-sync procedure
- How to run the gated integration test with `tursodb`
- Agent outbox is not migrated

- [ ] **Step 2: Update `docs/database-schema.md`**

Replace the first paragraph with:

```markdown
# Database Schema

The **server** can use one of three backends: embedded SQLite (default), remote-only Turso, or Turso Sync (local file + cloud replica). See [turso-server-database.md](turso-server-database.md). Schema SQL below is shared.

The **agent** uses a separate on-disk SQLite database for outbox spill only. That database is not a Turso backend.
```

- [ ] **Step 3: `.env.dev` commented block**

```bash
# Optional Turso (server only). Default is sqlite via BACKUP_DB_PATH from justfile.
# BACKUP_DB_DRIVER=turso-sync
# BACKUP_DB_URL=
# BACKUP_DB_AUTH_TOKEN=
# BACKUP_DB_SYNC_INTERVAL=30s
# BACKUP_ENCRYPTION_KEY=
```

- [ ] **Step 4: README env table**

Add the four vars. Keep `BACKUP_DB_PATH` as the sqlite / sync local path.

- [ ] **Step 5: Final verification**

Run: `just test-server && just fmt && just vet`
Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add docs/database-schema.md docs/turso-server-database.md .env.dev README.md
git commit -m "$(cat <<'EOF'
docs: describe optional Turso server database backends

Document env vars, offline-start behavior, and the sqlite-to-turso-sync cutover.
EOF
)"
```

---

## Self-review

**Spec coverage**

| Spec requirement | Task |
|---|---|
| Three drivers, sqlite default | 1, 2, 8 |
| Agent DB unchanged | File map + Task 10 |
| turso-sync request path local; sync background | 4, 5 |
| Startup table (refuse empty+down; start ready+down) | 4, 5 |
| Close best-effort Push | 4 |
| Encryption key rules | 1 |
| Same Turso Database engine for remote and sync | 6, 7 |
| One writer | Task 10 docs |
| Manual migration; adopt spike | 6, 10 |
| PRAGMA / column check | 3 |
| `New` sqlite wrapper | 2 |
| Gated non-sqlite test | 9 |
| Docs + `.env.dev` | 10 |
| No CGO / no go-libsql | 6, 7 |
| CI stays sqlite | 9 |

**Type consistency:** `config.Driver` and `database.Driver` are both string aliases with the same values; `main` casts `database.Driver(cfg.Driver)`. `remoteSync` is `{Pull, Push}`. `Open(Options)` is the process entry; `New` is sqlite-only.
