package executor

import (
	"context"
	"log/slog"
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
	got, err := expandTemplate("backup {{.PlanName}} status={{.Status}} snap={{.SnapshotID}}", hctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "backup daily status=success snap=abc123"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandTemplate_NoPlaceholders(t *testing.T) {
	got, err := expandTemplate("echo hello", &HookContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "echo hello" {
		t.Errorf("got %q, want %q", got, "echo hello")
	}
}

func TestExpandTemplate_InvalidTemplate(t *testing.T) {
	_, err := expandTemplate("echo {{.Broken", &HookContext{})
	if err == nil {
		t.Fatal("expected error for invalid template")
	}
}

func TestExpandTemplate_MissingField(t *testing.T) {
	_, err := expandTemplate("echo {{.NonExistent}}", &HookContext{})
	if err == nil {
		t.Fatal("expected error for missing field")
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

func TestRunHook_TemplateError(t *testing.T) {
	hook := &backupv1.ResolvedHook{
		Name:    "bad-template",
		OnEvent: "before_backup",
		Command: "echo {{.Invalid",
	}
	result := RunHook(context.Background(), hook, &HookContext{}, discardLogger())

	if result.Status != "failed" {
		t.Errorf("status: got %q, want failed", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error")
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
