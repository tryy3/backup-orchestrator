package executor

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

// safeEnvNames is the set of parent environment variable names that are safe
// to inherit in hook subprocesses. Everything else (including RESTIC_PASSWORD,
// RCLONE_CONFIG, and any other credentials) is stripped.
var safeEnvNames = map[string]bool{
	"PATH":      true,
	"HOME":      true,
	"USER":      true,
	"LOGNAME":   true,
	"SHELL":     true,
	"LANG":      true,
	"LC_ALL":    true,
	"TMP":       true,
	"TMPDIR":    true,
	"TERM":      true,
	"COLORTERM": true,
}

// hookEnv builds a minimal, safe environment for hook subprocesses.
// Only a known set of parent env vars is forwarded (LC_* prefix is also
// allowed). All BACKUP_* context variables are appended so that hook
// commands can reference them as ordinary environment variables.
func hookEnv(hctx *HookContext) []string {
	env := make([]string, 0, 20)

	for _, entry := range os.Environ() {
		k, _, _ := strings.Cut(entry, "=") // found bool not needed; we only need the key
		if safeEnvNames[k] || strings.HasPrefix(k, "LC_") {
			env = append(env, entry)
		}
	}

	env = append(env,
		"BACKUP_PLAN_NAME="+hctx.PlanName,
		"BACKUP_HOSTNAME="+hctx.Hostname,
		"BACKUP_STATUS="+hctx.Status,
		"BACKUP_DURATION="+hctx.Duration,
		"BACKUP_BYTES_ADDED="+hctx.BytesAdded,
		"BACKUP_FILES_NEW="+hctx.FilesNew,
		"BACKUP_FILES_CHANGED="+hctx.FilesChanged,
		"BACKUP_SNAPSHOT_ID="+hctx.SnapshotID,
		"BACKUP_ERROR="+hctx.Error,
		"BACKUP_STARTED_AT="+hctx.StartedAt,
		"BACKUP_FINISHED_AT="+hctx.FinishedAt,
	)

	return env
}

// shellQuote wraps s in POSIX single quotes so that it is treated as a
// literal string by the shell. Any single quote within s is handled by
// the standard end-quote/escaped-quote/begin-quote sequence.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

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

	// Execute via sh -c with a minimal, credential-free environment.
	cmd := exec.CommandContext(hookCtx, "sh", "-c", expanded)
	cmd.Env = hookEnv(hctx)
	cmd.Dir = os.TempDir()
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
// Placeholders use {{.FieldName}} syntax. Each substituted value is wrapped in
// POSIX single quotes (shellQuote) so that values containing shell metacharacters
// cannot break out of the argument and execute arbitrary commands.
// Unknown placeholders are left unchanged.
func expandTemplate(cmdStr string, hctx *HookContext) string {
	r := strings.NewReplacer(
		"{{.PlanName}}", shellQuote(hctx.PlanName),
		"{{.Hostname}}", shellQuote(hctx.Hostname),
		"{{.Status}}", shellQuote(hctx.Status),
		"{{.Duration}}", shellQuote(hctx.Duration),
		"{{.BytesAdded}}", shellQuote(hctx.BytesAdded),
		"{{.FilesNew}}", shellQuote(hctx.FilesNew),
		"{{.FilesChanged}}", shellQuote(hctx.FilesChanged),
		"{{.SnapshotID}}", shellQuote(hctx.SnapshotID),
		"{{.Error}}", shellQuote(hctx.Error),
		"{{.StartedAt}}", shellQuote(hctx.StartedAt),
		"{{.FinishedAt}}", shellQuote(hctx.FinishedAt),
	)
	return r.Replace(cmdStr)
}
