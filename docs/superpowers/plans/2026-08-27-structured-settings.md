# Structured Settings Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a typed settings registry with server-owned defaults, collect-all validation before writes, full resolved GET/PUT responses, and frontend field-error display — closing [#48](https://github.com/tryy3/backup-orchestrator/issues/48) without settings versioning.

**Architecture:** New `server/internal/settings` package owns the key registry (defaults + validators). HTTP handlers in `api/settings.go` become thin adapters. `database` settings methods stay dumb key/value storage. `configpush` replaces hardcoded default literals with registry defaults; unset command-timeout/outbox globals keep today’s “omit / zero → agent built-in” behavior. Frontend parses `errors[]`, stores `fieldErrors`, and shows them globally + inline.

**Tech Stack:** Go 1.26, `encoding/json`, `log/slog`, `database/sql`, chi HTTP handlers, testify; Vue 3 + Pinia + TypeScript + Vitest.

**Spec:** [docs/superpowers/specs/2026-08-27-structured-settings-design.md](../specs/2026-08-27-structured-settings-design.md)

## Global Constraints

- No settings versioning / migration metadata.
- No DB schema change: `settings(key TEXT PRIMARY KEY, value TEXT NOT NULL)` stays.
- Validate **all** keys/values before any `SetSetting`; on any error → `slog.Warn`, `400` with `{ "errors": [...] }`, zero writes.
- `GET` and successful `PUT` return every registry key (stored if valid, else default).
- PUT success body is the full resolved object (not `{ "status": "updated" }`).
- Cross-field validation (e.g. failing vs warning threshold order) is out of scope.
- Prefer root `just` recipes: `just test-server`, `just test-frontend`, `just fmt`, `just vet`.
- Do not commit secrets. Do not change agent settings storage.

## File map

| File | Responsibility |
|---|---|
| `server/internal/settings/registry.go` | Registry entries, defaults, validators, `Validate` |
| `server/internal/settings/store.go` | `LoadResolved`, `Apply` against `*database.DB` |
| `server/internal/settings/registry_test.go` | Validate / defaults unit tests |
| `server/internal/settings/store_test.go` | LoadResolved / Apply integration with temp DB |
| `server/internal/api/settings.go` | Thin GET/PUT handlers |
| `server/internal/api/settings_test.go` | HTTP handler tests |
| `server/internal/api/router.go` | Optional helper `writeJSON` only if needed; keep `writeError` for other endpoints |
| `server/internal/configpush/resolver.go` | Use registry defaults for heartbeat / hook timeout / blocked-path fallbacks |
| `frontend/src/types/api.ts` | `Settings`, `SETTINGS_DEFAULTS`, `SettingsFieldError` |
| `frontend/src/api/client.ts` | Parse settings `errors[]` into a typed error |
| `frontend/src/stores/settings.ts` | `fieldErrors`, simplified `resolved` |
| `frontend/src/views/SettingsView.vue` | Global + inline field errors |
| `frontend/src/stores/__tests__/settings.test.ts` | Store error parsing / fieldErrors |
| `docs/database-schema.md` | Note allow-listed validated keys in `internal/settings` |
| `docs/grpc-api.md` | Document GET/PUT shapes and error body |

---

### Task 1: Settings registry + Validate

**Files:**
- Create: `server/internal/settings/registry.go`
- Create: `server/internal/settings/registry_test.go`

**Interfaces:**
- Consumes: `database.RetentionPolicy` (for retention shape only; do not import `api`)
- Produces:
  - `type FieldError struct { Key string \`json:"key"\`; Message string \`json:"message"\` }`
  - `func Keys() []string`
  - `func DefaultJSON(key string) (json.RawMessage, bool)`
  - `func Validate(input map[string]json.RawMessage) []FieldError`
  - Unexported registry map covering every key in the spec table

- [ ] **Step 1: Write the failing tests**

Create `server/internal/settings/registry_test.go`:

```go
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
		"bogus":                        json.RawMessage(`1`),
		"heartbeat_interval_seconds":   json.RawMessage(`2`),
		"job_history_days":             json.RawMessage(`"nope"`),
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

func TestDefaultJSON_Heartbeat(t *testing.T) {
	t.Parallel()
	raw, ok := settings.DefaultJSON("heartbeat_interval_seconds")
	require.True(t, ok)
	var n int
	require.NoError(t, json.Unmarshal(raw, &n))
	assert.Equal(t, 30, n)
}

func TestKeys_IncludesAllKnown(t *testing.T) {
	t.Parallel()
	keys := settings.Keys()
	assert.Contains(t, keys, "default_retention")
	assert.Contains(t, keys, "outbox_max_attempts")
	assert.Len(t, keys, 20)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/settings/ -count=1`

Expected: FAIL (package does not exist / undefined)

- [ ] **Step 3: Implement registry**

Create `server/internal/settings/registry.go` with:

```go
package settings

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/tryy3/backup-orchestrator/server/internal/database"
)

type FieldError struct {
	Key     string `json:"key"`
	Message string `json:"message"`
}

type entry struct {
	defaultJSON json.RawMessage
	validate    func(json.RawMessage) error
}

// registry is populated in init via mustRegister helpers.
var registry = map[string]entry{}

func Keys() []string { /* stable iteration order matching registration order slice */ }

func DefaultJSON(key string) (json.RawMessage, bool) { /* copy of entry.defaultJSON */ }

func Validate(input map[string]json.RawMessage) []FieldError {
	var errs []FieldError
	for key, raw := range input {
		e, ok := registry[key]
		if !ok {
			errs = append(errs, FieldError{Key: key, Message: "unknown setting"})
			continue
		}
		if err := e.validate(raw); err != nil {
			errs = append(errs, FieldError{Key: key, Message: err.Error()})
		}
	}
	return errs
}
```

Register all 20 keys from the spec with matching defaults and mins:

- Int keys: decode as `json.Number` or float64 then require whole number; enforce `>= min`.
- Health thresholds: float `> 0 && <= 1`.
- `file_browser_blocked_paths`: `[]string`, each path `strings.TrimSpace` non-empty (reject empty strings).
- `default_retention`: unmarshal into `database.RetentionPolicy`; all keep_* `>= 0`.

Keep a parallel `var keyOrder []string` filled at registration so `Keys()` is deterministic.

Defaults (verbatim from spec):

| Key | Default |
|---|---|
| `default_retention` | `{"keep_last":5,"keep_hourly":0,"keep_daily":0,"keep_weekly":0,"keep_monthly":0,"keep_yearly":0}` |
| `heartbeat_interval_seconds` | `30` |
| `agent_offline_threshold_seconds` | `300` |
| `job_history_days` | `30` |
| `health_threshold_failing` | `0.9` |
| `health_threshold_warning` | `0.99` |
| `max_heatmap_runs` | `30` |
| `default_hook_timeout_seconds` | `60` |
| `file_browser_blocked_paths` | `["/proc","/sys","/dev","/run/credentials","/selinux","/cgroup"]` |
| `command_timeout_backup_seconds` | `86400` |
| `command_timeout_restore_seconds` | `86400` |
| `command_timeout_list_snapshots_seconds` | `300` |
| `command_timeout_browse_snapshot_seconds` | `300` |
| `command_timeout_browse_filesystem_seconds` | `30` |
| `command_timeout_default_seconds` | `300` |
| `outbox_spill_max_rows` | `20000` |
| `outbox_spill_retention_seconds` | `604800` |
| `outbox_flush_interval_seconds` | `60` |
| `outbox_delivery_timeout_seconds` | `10` |
| `outbox_max_attempts` | `10` |

Mins: heartbeat ≥5, offline ≥30, job_history ≥1, heatmap ≥5, hook ≥5, backup/restore timeout ≥60, list/browse_snapshot/default timeout ≥5, browse_fs ≥1, spill_max_rows ≥100, spill_retention ≥60, flush/delivery/attempts ≥1.

Helper sketch for ints:

```go
func validateIntMin(min int) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var f float64
		if err := json.Unmarshal(raw, &f); err != nil {
			return fmt.Errorf("must be a number")
		}
		if math.Trunc(f) != f {
			return fmt.Errorf("must be an integer")
		}
		n := int(f)
		if n < min {
			return fmt.Errorf("must be at least %d", min)
		}
		return nil
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/settings/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/settings/registry.go server/internal/settings/registry_test.go
git commit -m "$(cat <<'EOF'
feat(server): add typed settings registry with validation

Introduce internal/settings as the allow-list of known keys with
defaults and per-key validators for #48.
EOF
)"
```

---

### Task 2: LoadResolved + Apply

**Files:**
- Create: `server/internal/settings/store.go`
- Create: `server/internal/settings/store_test.go`

**Interfaces:**
- Consumes: `Validate`, `Keys`, `DefaultJSON`, `database.DB.GetSetting`, `database.DB.SetSetting`
- Produces:
  - `func LoadResolved(ctx context.Context, db *database.DB) (map[string]json.RawMessage, error)`
  - `func Apply(ctx context.Context, db *database.DB, input map[string]json.RawMessage) error` — assumes already validated; returns error only on DB failure
  - Optional: `func ParseInt(raw json.RawMessage) (int, error)` used by configpush later, or export small typed getters in Task 4

- [ ] **Step 1: Write the failing tests**

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/settings/ -count=1 -run 'LoadResolved|Apply'`

Expected: FAIL (undefined LoadResolved/Apply)

- [ ] **Step 3: Implement store.go**

```go
func LoadResolved(ctx context.Context, db *database.DB) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(registry))
	for _, key := range Keys() {
		def, _ := DefaultJSON(key)
		val, err := db.GetSetting(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("get setting %q: %w", key, err)
		}
		if val == nil {
			out[key] = def
			continue
		}
		raw := json.RawMessage(*val)
		if err := registry[key].validate(raw); err != nil {
			slog.Warn("invalid stored setting; using default", "key", key, "error", err)
			out[key] = def
			continue
		}
		out[key] = raw
	}
	return out, nil
}

func Apply(ctx context.Context, db *database.DB, input map[string]json.RawMessage) error {
	for key, raw := range input {
		if err := db.SetSetting(ctx, key, string(raw)); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/settings/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/settings/store.go server/internal/settings/store_test.go
git commit -m "$(cat <<'EOF'
feat(server): resolve and apply settings via registry

LoadResolved fills defaults for missing/corrupt keys; Apply writes
only after Validate has succeeded.
EOF
)"
```

---

### Task 3: Wire HTTP handlers + tests

**Files:**
- Modify: `server/internal/api/settings.go` (replace allow-list implementation)
- Create: `server/internal/api/settings_test.go`
- Modify: `server/internal/api/router.go` only if a small `writeErrors` helper is added next to `writeError`

**Interfaces:**
- Consumes: `settings.Validate`, `settings.Apply`, `settings.LoadResolved`, `configpush.Resolver.PushConfigToAllAgents`
- Produces: GET returns full map; PUT validates → errors or Apply + resolved map

- [ ] **Step 1: Write the failing HTTP tests**

Create `server/internal/api/settings_test.go`. Use `httptest.NewRecorder`, `http.NewRequest`, and a real temp DB. Build the resolver like production:

```go
mgr := agentmgr.New()
resolver := configpush.New(db, mgr)
```

(`PushConfigToAllAgents` is safe with an empty manager — no agents online.)

```go
func TestGetSettings_ReturnsAllKeysWithDefaults(t *testing.T) {
	// GET /settings on empty DB → 200, len(body)==20, heartbeat==30
}

func TestUpdateSettings_UnknownAndInvalid_NoWrite(t *testing.T) {
	// PUT body {"bogus":1,"heartbeat_interval_seconds":2}
	// → 400, errors length 2, GetSetting heartbeat still nil
}

func TestUpdateSettings_Valid_WritesAndReturnsResolved(t *testing.T) {
	// PUT {"heartbeat_interval_seconds":45} → 200, body heartbeat 45, DB has 45
}
```

Use `chi` router or call handler funcs directly:

```go
handler := getSettingsHandler(db)
req := httptest.NewRequest(http.MethodGet, "/settings", nil)
rr := httptest.NewRecorder()
handler.ServeHTTP(rr, req)
```

For PUT, decode response into `map[string]json.RawMessage` on success and `struct { Errors []settings.FieldError \`json:"errors"\` }` on failure.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd server && go test ./internal/api/ -count=1 -run Settings`

Expected: FAIL (old handler returns incomplete GET / status-updated PUT)

- [ ] **Step 3: Rewrite settings.go**

```go
func getSettingsHandler(db *database.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resolved, err := settings.LoadResolved(r.Context(), db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resolved)
	}
}

func updateSettingsHandler(db *database.DB, resolver *configpush.Resolver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var input map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		errs := settings.Validate(input)
		if len(errs) > 0 {
			slog.Warn("settings validation failed", "error_count", len(errs), "errors", errs)
			writeJSON(w, http.StatusBadRequest, map[string]any{"errors": errs})
			return
		}
		if err := settings.Apply(r.Context(), db, input); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		go resolver.PushConfigToAllAgents(context.Background())
		resolved, err := settings.LoadResolved(r.Context(), db)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, resolved)
	}
}
```

Remove `settingsKeys` and `allowedSettings` from this file.

Invalid JSON body may keep `{ "error": "invalid request body" }` (not field errors).

- [ ] **Step 4: Run tests**

Run: `cd server && go test ./internal/api/ ./internal/settings/ -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add server/internal/api/settings.go server/internal/api/settings_test.go
git commit -m "$(cat <<'EOF'
feat(server): validate settings updates via registry

GET/PUT return fully resolved settings; validation failures return
all field errors and never write.
EOF
)"
```

---

### Task 4: configpush uses registry defaults

**Files:**
- Modify: `server/internal/configpush/resolver.go`
- Modify or create: `server/internal/configpush/resolver_test.go` if tests exist; otherwise add focused tests for default fallbacks

**Interfaces:**
- Consumes: `settings.DefaultJSON` and/or small helpers
- Produces: same resolver behavior, but heartbeat default `30` and hook timeout default `60` come from registry; blocked paths / retention parsing unchanged except defaults sourced from registry when helpful

- [ ] **Step 1: Add a typed helper if needed**

In `server/internal/settings/registry.go` (or `store.go`):

```go
func DefaultInt(key string) (int, bool) {
	raw, ok := DefaultJSON(key)
	if !ok {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil {
		return 0, false
	}
	return int(f), true
}
```

- [ ] **Step 2: Update resolver hardcoded literals**

Replace:

```go
defaultHookTimeout := int32(60)
// ...
heartbeatInterval := int32(30)
```

with:

```go
defaultHookTimeout := int32(60)
if v, ok := settings.DefaultInt("default_hook_timeout_seconds"); ok {
	defaultHookTimeout = int32(v)
}
heartbeatInterval := int32(30)
if v, ok := settings.DefaultInt("heartbeat_interval_seconds"); ok {
	heartbeatInterval = int32(v)
}
```

Keep command-timeout and outbox loaders as today: missing/invalid stored value → `0` / omit from push (do **not** force registry defaults into the agent push for those keys). That preserves “agent compiled-in default when unset”.

For `default_retention`, keep “only if stored and parseable”; do not invent a push when unset unless existing code already did. (GET still returns the retention default for the UI.)

- [ ] **Step 3: Run server tests**

Run: `cd server && go test ./internal/configpush/ ./internal/settings/ ./internal/api/ -count=1`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add server/internal/configpush/resolver.go server/internal/settings/*.go
git commit -m "$(cat <<'EOF'
refactor(server): source configpush defaults from settings registry

Heartbeat and hook-timeout fallbacks read from internal/settings
instead of magic numbers.
EOF
)"
```

---

### Task 5: Frontend types, client, and store

**Files:**
- Modify: `frontend/src/types/api.ts`
- Modify: `frontend/src/api/client.ts`
- Modify: `frontend/src/stores/settings.ts`
- Create: `frontend/src/stores/__tests__/settings.test.ts`

**Interfaces:**
- Produces:
  - `export interface SettingsFieldError { key: string; message: string }`
  - `export class SettingsValidationError extends Error { errors: SettingsFieldError[] }`
  - Store: `fieldErrors: Ref<Record<string, string>>`, cleared on fetch/success
  - `settings.update` catches `SettingsValidationError` and populates `fieldErrors`

- [ ] **Step 1: Write the failing store test**

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { useSettingsStore } from '../settings'
import * as api from '../../api/client'
import { SettingsValidationError } from '../../api/client'

vi.mock('../../api/client', async () => {
  const actual = await vi.importActual<typeof import('../../api/client')>('../../api/client')
  return { ...actual, settings: { get: vi.fn(), update: vi.fn() } }
})

describe('settings store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.mocked(api.settings.update).mockReset()
  })

  it('maps validation errors to fieldErrors', async () => {
    vi.mocked(api.settings.update).mockRejectedValue(
      new SettingsValidationError([
        { key: 'heartbeat_interval_seconds', message: 'must be at least 5' },
      ]),
    )
    const store = useSettingsStore()
    const ok = await store.update({} as never)
    expect(ok).toBe(false)
    expect(store.fieldErrors.heartbeat_interval_seconds).toBe('must be at least 5')
  })
})
```

Adjust import paths to match repo layout (`frontend/src/stores/__tests__/`).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && npx vitest run src/stores/__tests__/settings.test.ts`

Expected: FAIL (missing types/error class)

- [ ] **Step 3: Implement client + types + store**

In `client.ts`, for `/settings` PUT failures (or generally when body has `errors` array):

```ts
export class SettingsValidationError extends Error {
  errors: SettingsFieldError[]
  constructor(errors: SettingsFieldError[]) {
    super(errors.map((e) => `${e.key}: ${e.message}`).join('; '))
    this.errors = errors
  }
}

// inside request(), when !res.ok:
const parsed = JSON.parse(body)
if (Array.isArray(parsed?.errors)) {
  throw new SettingsValidationError(parsed.errors)
}
if (typeof parsed?.error === 'string') {
  throw new Error(parsed.error)
}
```

Update `Settings` so fields returned by GET are required (match registry). Keep `SETTINGS_DEFAULTS` values identical to the Go registry for pre-fetch fallback.

Store changes:

```ts
const fieldErrors = ref<Record<string, string>>({})

async function fetch() {
  fieldErrors.value = {}
  // existing fetch...
}

async function update(data: Settings) {
  fieldErrors.value = {}
  try {
    settings.value = await api.update(data)
    return true
  } catch (e) {
    if (e instanceof SettingsValidationError) {
      fieldErrors.value = Object.fromEntries(e.errors.map((x) => [x.key, x.message]))
      error.value = e.message
    } else {
      error.value = e instanceof Error ? e.message : String(e)
    }
    return false
  }
}
```

Simplify `resolved` to use `settings.value` fields first, then `SETTINGS_DEFAULTS` only as fallback before first fetch.

- [ ] **Step 4: Run frontend tests**

Run: `cd frontend && npx vitest run src/stores/__tests__/settings.test.ts`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add frontend/src/types/api.ts frontend/src/api/client.ts frontend/src/stores/settings.ts frontend/src/stores/__tests__/settings.test.ts
git commit -m "$(cat <<'EOF'
feat(frontend): parse settings field errors into the store

Surface structured PUT /settings validation errors for per-field UI.
EOF
)"
```

---

### Task 6: SettingsView inline + global errors

**Files:**
- Modify: `frontend/src/views/SettingsView.vue`

**Interfaces:**
- Consumes: `store.fieldErrors`, `store.error`

- [ ] **Step 1: Update status banner**

Replace single-string error banner with:

- If `store.error` / any `fieldErrors`: show a list of all `Object.entries(store.fieldErrors)` as `<li>{{ key }}: {{ message }}</li>` (and fall back to `store.error` when there are no field keys, e.g. network failure).

- [ ] **Step 2: Add inline errors under controls**

For each bound setting, under the input add:

```vue
<p v-if="store.fieldErrors.heartbeat_interval_seconds" class="mt-1 text-xs text-red-400">
  {{ store.fieldErrors.heartbeat_interval_seconds }}
</p>
```

Map keys:

| Control | Key |
|---|---|
| heartbeat | `heartbeat_interval_seconds` |
| offline | `agent_offline_threshold_seconds` |
| job history | `job_history_days` |
| health failing | `health_threshold_failing` |
| health warning | `health_threshold_warning` |
| heatmap | `max_heatmap_runs` |
| hook timeout | `default_hook_timeout_seconds` |
| blocked paths | `file_browser_blocked_paths` |
| cmd timeouts | matching `command_timeout_*` keys |
| outbox fields | matching `outbox_*` keys |
| retention editor | `default_retention` |

Clear `saved` flash on validation failure (already gated on `ok`).

After successful save, `store.settings` is the full resolved object — prefer reading from `store.settings` without `?? SETTINGS_DEFAULTS` where possible; keep defaults only for initial refs before `onMounted` fetch completes.

- [ ] **Step 3: Manual sanity / light test (optional)**

If a SettingsView component test is cheap with existing mount helpers, assert that seeding `fieldErrors` renders the message. Otherwise skip and rely on store test + manual check.

Run: `cd frontend && npm test`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/views/SettingsView.vue
git commit -m "$(cat <<'EOF'
feat(frontend): show settings validation errors per field

List all server field errors and inline them under matching controls.
EOF
)"
```

---

### Task 7: Docs + final verification

**Files:**
- Modify: `docs/database-schema.md` (settings section)
- Modify: `docs/grpc-api.md` (REST settings bullets)

- [ ] **Step 1: Update database-schema.md**

Under the settings table comment, replace the single-key note with:

```markdown
-- Global settings (key/value JSON). Valid keys, types, defaults, and
-- constraints are defined in server/internal/settings (allow-listed).
-- Unknown keys are rejected by PUT /api/settings. GET returns every
-- known key with stored value or registry default.
```

- [ ] **Step 2: Update grpc-api.md REST section**

Document:

- `GET /api/settings` → full resolved object
- `PUT /api/settings` → partial update; `200` full resolved object; `400` `{ "errors": [{ "key", "message" }] }`

- [ ] **Step 3: Run full verification**

```bash
just test-server
just test-frontend
just fmt
just vet
```

Expected: all pass

- [ ] **Step 4: Commit**

```bash
git add docs/database-schema.md docs/grpc-api.md
git commit -m "$(cat <<'EOF'
docs: document structured settings validation for #48

Describe allow-listed settings keys and the GET/PUT error contract.
EOF
)"
```

---

## Spec coverage checklist

| Spec requirement | Task |
|---|---|
| Registry with types/defaults/validators | 1 |
| Collect-all Validate, no DB I/O | 1 |
| LoadResolved (defaults + corrupt fallback) | 2 |
| Apply only after validate | 2–3 |
| GET full resolved | 3 |
| PUT 400 + errors[] + slog + no write | 3 |
| PUT 200 full resolved body | 3 |
| configpush registry defaults | 4 |
| Frontend fieldErrors + UI | 5–6 |
| Docs | 7 |
| No versioning | honored (non-goal) |
| No schema change | honored |

## Self-review notes

- PUT response shape change is intentional and covered in Tasks 3 and 5.
- Command-timeout/outbox push semantics intentionally keep “unset → agent default”; only heartbeat/hook hardcoded literals move to the registry in Task 4.
- `SettingsValidationError` is thrown from shared `request()` when `errors` is present so settings PUT works without a special-case path; other endpoints do not send `errors` today.
