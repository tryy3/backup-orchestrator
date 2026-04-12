package executor

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"sort"
	"strings"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

// HookContext provides template variables for hook command expansion.
type HookContext struct {
	PlanName     string
	Hostname     string
	Status       string
	Duration     string
	BytesAdded   string
	FilesNew     string
	FilesChanged string
	SnapshotID   string
	Error        string
	StartedAt    string
	FinishedAt   string
}

// HookResult records the outcome of running a single hook.
type HookResult struct {
	HookName   string
	Phase      string
	Status     string // "success", "failed"
	Error      string
	Output     string // combined stdout+stderr output
	DurationMs int64
}

// RunHook expands template variables in the hook command and executes it via sh -c.
func RunHook(ctx context.Context, hook *backupv1.ResolvedHook, hctx *HookContext, jlog *slog.Logger) *HookResult {
	start := time.Now()
	result := &HookResult{
		HookName: hook.GetName(),
		Phase:    hook.GetOnEvent(),
		Status:   "success",
	}

	// Expand placeholder variables in the command.
	expanded := expandTemplate(hook.GetCommand(), hctx)

	// Apply timeout if specified.
	timeoutSec := hook.GetTimeoutSeconds()
	if timeoutSec <= 0 {
		timeoutSec = 60 // default 60 seconds
	}
	hookCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	// Log the internally-set hook context (template variables we inject).
	jlog.Info("executing hook",
		"source", "hook",
		"hook", hook.GetName(),
		"phase", hook.GetOnEvent(),
		"timeout_seconds", timeoutSec,
		"plan_name", hctx.PlanName,
		"hostname", hctx.Hostname,
		"status", hctx.Status,
		"duration", hctx.Duration,
		"started_at", hctx.StartedAt,
		"finished_at", hctx.FinishedAt,
		"bytes_added", hctx.BytesAdded,
		"files_new", hctx.FilesNew,
		"files_changed", hctx.FilesChanged,
		"snapshot_id", hctx.SnapshotID,
		"error", hctx.Error,
	)

	// Execute via sh -c.
	cmd := exec.CommandContext(hookCtx, "sh", "-c", expanded)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		result.Status = "failed"
		errMsg := err.Error()
		if stderrBuf.Len() > 0 {
			errMsg = strings.TrimSpace(stderrBuf.String())
		}
		result.Error = errMsg
	}

	// Capture output.
	var output strings.Builder
	if stdoutBuf.Len() > 0 {
		output.WriteString(strings.TrimSpace(stdoutBuf.String()))
	}
	if stderrBuf.Len() > 0 {
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString(strings.TrimSpace(stderrBuf.String()))
	}
	result.Output = output.String()

	result.DurationMs = time.Since(start).Milliseconds()
	return result
}

// RunHooks filters hooks by event, sorts by sort_order, and runs them sequentially.
// If a hook with on_error="abort" fails, execution stops and aborted=true is returned.
func RunHooks(ctx context.Context, hooks []*backupv1.ResolvedHook, event string, hctx *HookContext, jlog *slog.Logger) (results []*HookResult, aborted bool) {
	// Filter hooks by event.
	var matching []*backupv1.ResolvedHook
	for _, h := range hooks {
		if h.GetOnEvent() == event {
			matching = append(matching, h)
		}
	}

	// Sort by sort_order.
	sort.Slice(matching, func(i, j int) bool {
		return matching[i].GetSortOrder() < matching[j].GetSortOrder()
	})

	for _, hook := range matching {
		jlog.Info("running hook", "source", "hook", "phase", event, "hook", hook.GetName())
		result := RunHook(ctx, hook, hctx, jlog)
		results = append(results, result)

		if result.Status == "failed" {
			jlog.Error("hook failed", "source", "hook", "phase", event, "hook", hook.GetName(), "error", result.Error)
			if hook.GetOnError() == "abort" {
				jlog.Warn("hook aborted execution", "source", "hook", "phase", event, "hook", hook.GetName())
				return results, true
			}
		} else {
			jlog.Info("hook succeeded", "source", "hook", "phase", event, "hook", hook.GetName(), "duration_ms", result.DurationMs)
		}

		if result.Output != "" {
			jlog.Info("hook output", "source", "hook", "phase", event, "hook", hook.GetName(), "output", result.Output)
		}
	}

	return results, false
}

// expandTemplate replaces HookContext placeholder variables in the command string.
// Placeholders use {{.FieldName}} syntax and are replaced with their corresponding
// HookContext field values. Unknown placeholders are left unchanged.
func expandTemplate(cmdStr string, hctx *HookContext) string {
	r := strings.NewReplacer(
		"{{.PlanName}}", hctx.PlanName,
		"{{.Hostname}}", hctx.Hostname,
		"{{.Status}}", hctx.Status,
		"{{.Duration}}", hctx.Duration,
		"{{.BytesAdded}}", hctx.BytesAdded,
		"{{.FilesNew}}", hctx.FilesNew,
		"{{.FilesChanged}}", hctx.FilesChanged,
		"{{.SnapshotID}}", hctx.SnapshotID,
		"{{.Error}}", hctx.Error,
		"{{.StartedAt}}", hctx.StartedAt,
		"{{.FinishedAt}}", hctx.FinishedAt,
	)
	return r.Replace(cmdStr)
}
