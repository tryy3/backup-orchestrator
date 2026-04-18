package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/tryy3/backup-orchestrator/agent/internal/redact"
)

// Bounds applied to subprocess output capture so that very long-running
// restic invocations (e.g. backups of millions of files) cannot grow the
// agent's resident memory unboundedly.
const (
	// maxStdoutBytes caps the stdout retained by runRestic for callers
	// that consume the output as a single string (snapshots, ls, …).
	// Backup streams output and never retains all stdout in memory.
	maxStdoutBytes = 1 << 20 // 1 MiB
	// maxStderrBytes caps the stderr retained from any restic invocation.
	maxStderrBytes = 4 << 10 // 4 KiB
	// maxScannerLineBytes is the maximum length of a single line read
	// from restic's stdout/stderr. restic's `backup --json` status
	// messages can include the current file paths and may exceed the
	// default bufio.Scanner limit (64 KiB).
	maxScannerLineBytes = 1 << 20 // 1 MiB
	// maxBackupTailLines caps the number of non-status stdout lines
	// retained from a `restic backup --json` run for diagnostics when
	// the summary message is missing (e.g. on failure).
	maxBackupTailLines = 200
)

// tailBuffer is an io.Writer that retains only the last `max` bytes of
// what is written to it. It is safe for concurrent writes, which is
// required because we drain stdout and stderr from separate goroutines.
type tailBuffer struct {
	mu        sync.Mutex
	max       int
	buf       []byte
	truncated bool
}

func newTailBuffer(max int) *tailBuffer {
	return &tailBuffer{max: max}
}

// Write appends p, keeping at most `max` trailing bytes.
func (t *tailBuffer) Write(p []byte) (int, error) {
	n := len(p)
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.buf)+n <= t.max {
		t.buf = append(t.buf, p...)
		return n, nil
	}
	t.truncated = true
	if n >= t.max {
		// p alone exceeds the cap; keep only its tail.
		t.buf = append(t.buf[:0], p[n-t.max:]...)
		return n, nil
	}
	// Drop oldest bytes from t.buf to make room for p.
	keep := t.max - n
	copy(t.buf, t.buf[len(t.buf)-keep:])
	t.buf = t.buf[:keep]
	t.buf = append(t.buf, p...)
	return n, nil
}

// String returns the captured tail, prefixed with a truncation marker
// if any data was discarded.
func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.truncated {
		return "...[truncated]...\n" + string(t.buf)
	}
	return string(t.buf)
}

// Repository represents a restic backup repository.
type Repository struct {
	ID       string
	Name     string
	Type     string
	Path     string
	Password string
}

// BackupResult holds statistics from a restic backup run.
type BackupResult struct {
	SnapshotID      string
	FilesNew        int64
	FilesChanged    int64
	FilesUnmodified int64
	BytesAdded      int64
	TotalBytes      int64
	DurationMs      int64
	Stderr          string // stderr output from restic (may contain warnings)
}

// SnapshotInfo represents a restic snapshot.
type SnapshotInfo struct {
	ID       string   `json:"short_id"`
	LongID   string   `json:"id"`
	Time     string   `json:"time"`
	Hostname string   `json:"hostname"`
	Tags     []string `json:"tags"`
	Paths    []string `json:"paths"`
}

// FileEntry represents a file or directory listed from a snapshot.
type FileEntry struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	MTime string `json:"mtime"`
}

// RetentionPolicy maps to the retention flags for restic forget.
type RetentionPolicy struct {
	KeepLast    int32
	KeepHourly  int32
	KeepDaily   int32
	KeepWeekly  int32
	KeepMonthly int32
	KeepYearly  int32
}

// ResticExecutor runs restic CLI commands.
type ResticExecutor struct {
	RcloneConfigPath string // path to rclone.conf
}

// resticSummary is the JSON structure restic outputs for backup summary.
type resticSummary struct {
	MessageType     string  `json:"message_type"`
	FilesNew        int64   `json:"files_new"`
	FilesChanged    int64   `json:"files_changed"`
	FilesUnmodified int64   `json:"files_unmodified"`
	DataAdded       int64   `json:"data_added"`
	TotalBytesProc  int64   `json:"total_bytes_processed"`
	TotalDuration   float64 `json:"total_duration"`
	SnapshotID      string  `json:"snapshot_id"`
}

// Backup runs restic backup with the given parameters and returns parsed results.
//
// Stdout is parsed line-by-line incrementally so that the agent does not
// retain restic's per-file `status` messages in memory; only the final
// `summary` struct (and a small bounded tail of recent diagnostic lines
// in case of failure) is kept.
func (r *ResticExecutor) Backup(ctx context.Context, repo Repository, paths, excludes, tags []string, logger *slog.Logger) (*BackupResult, error) {
	args := make([]string, 0, 4+2*len(tags)+2*len(excludes)+len(paths))
	args = append(args, "backup", "--json", "--repo", repo.Path)

	for _, tag := range tags {
		args = append(args, "--tag", tag)
	}
	for _, exc := range excludes {
		args = append(args, "--exclude", exc)
	}
	args = append(args, paths...)

	var (
		summary    resticSummary
		foundSum   bool
		tail       = make([]string, 0, maxBackupTailLines)
		linesSeen  int
		statusSeen int
	)
	addTail := func(line string) {
		if len(tail) < maxBackupTailLines {
			tail = append(tail, line)
			return
		}
		// Ring: drop the oldest entry.
		copy(tail, tail[1:])
		tail[len(tail)-1] = line
	}

	start := time.Now()
	stderr, err := r.streamRestic(ctx, repo, args, logger, func(line []byte) {
		linesSeen++
		// Try to interpret the line as a JSON message from restic.
		var msg struct {
			MessageType string `json:"message_type"`
		}
		if jerr := json.Unmarshal(line, &msg); jerr != nil {
			// Non-JSON line (e.g. plain progress); keep a small tail.
			addTail(string(line))
			return
		}
		switch msg.MessageType {
		case "status":
			// High-volume per-file progress messages — discard.
			statusSeen++
		case "summary":
			if jerr := json.Unmarshal(line, &summary); jerr == nil {
				foundSum = true
			} else {
				addTail(string(line))
			}
		default:
			// "error", "verbose_status", unknown types → keep for diagnostics.
			addTail(string(line))
		}
	})
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		return nil, fmt.Errorf("restic backup: %w\nstderr: %s", err, stderr)
	}

	if !foundSum {
		tailStr := strings.Join(tail, "\n")
		return nil, fmt.Errorf("restic backup did not produce a summary message (%d lines, %d status updates)\nstdout tail: %s",
			linesSeen, statusSeen, tailStr)
	}

	return &BackupResult{
		SnapshotID:      summary.SnapshotID,
		FilesNew:        summary.FilesNew,
		FilesChanged:    summary.FilesChanged,
		FilesUnmodified: summary.FilesUnmodified,
		BytesAdded:      summary.DataAdded,
		TotalBytes:      summary.TotalBytesProc,
		DurationMs:      durationMs,
		Stderr:          strings.TrimSpace(stderr),
	}, nil
}

// Forget runs restic forget with the given retention policy and tag filters.
func (r *ResticExecutor) Forget(ctx context.Context, repo Repository, retention RetentionPolicy, tags []string, logger *slog.Logger) error {
	args := []string{"forget", "--repo", repo.Path}

	if retention.KeepLast > 0 {
		args = append(args, "--keep-last", fmt.Sprintf("%d", retention.KeepLast))
	}
	if retention.KeepHourly > 0 {
		args = append(args, "--keep-hourly", fmt.Sprintf("%d", retention.KeepHourly))
	}
	if retention.KeepDaily > 0 {
		args = append(args, "--keep-daily", fmt.Sprintf("%d", retention.KeepDaily))
	}
	if retention.KeepWeekly > 0 {
		args = append(args, "--keep-weekly", fmt.Sprintf("%d", retention.KeepWeekly))
	}
	if retention.KeepMonthly > 0 {
		args = append(args, "--keep-monthly", fmt.Sprintf("%d", retention.KeepMonthly))
	}
	if retention.KeepYearly > 0 {
		args = append(args, "--keep-yearly", fmt.Sprintf("%d", retention.KeepYearly))
	}

	for _, tag := range tags {
		args = append(args, "--tag", tag)
	}

	_, stderr, err := r.runRestic(ctx, repo, args, logger)
	if err != nil {
		return fmt.Errorf("restic forget: %w\nstderr: %s", err, stderr)
	}
	return nil
}

// Prune runs restic prune to reclaim space.
func (r *ResticExecutor) Prune(ctx context.Context, repo Repository, logger *slog.Logger) error {
	args := []string{"prune", "--repo", repo.Path}

	_, stderr, err := r.runRestic(ctx, repo, args, logger)
	if err != nil {
		return fmt.Errorf("restic prune: %w\nstderr: %s", err, stderr)
	}
	return nil
}

// Snapshots lists all snapshots in the repository.
func (r *ResticExecutor) Snapshots(ctx context.Context, repo Repository, logger *slog.Logger) ([]SnapshotInfo, error) {
	args := []string{"snapshots", "--json", "--repo", repo.Path}

	stdout, stderr, err := r.runRestic(ctx, repo, args, logger)
	if err != nil {
		return nil, fmt.Errorf("restic snapshots: %w\nstderr: %s", err, stderr)
	}

	var snapshots []SnapshotInfo
	if err := json.Unmarshal([]byte(stdout), &snapshots); err != nil {
		return nil, fmt.Errorf("parsing restic snapshots: %w", err)
	}
	return snapshots, nil
}

// ListFiles lists files in a snapshot at the given path.
func (r *ResticExecutor) ListFiles(ctx context.Context, repo Repository, snapshotID, path string, logger *slog.Logger) ([]FileEntry, error) {
	args := []string{"ls", "--json", "--repo", repo.Path, snapshotID}
	if path != "" {
		args = append(args, path)
	}

	stdout, stderr, err := r.runRestic(ctx, repo, args, logger)
	if err != nil {
		return nil, fmt.Errorf("restic ls: %w\nstderr: %s", err, stderr)
	}

	var entries []FileEntry
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry FileEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Skip non-JSON lines (restic ls --json may output a header object first).
			continue
		}
		// Only include entries that have a path (skip the snapshot header).
		if entry.Path != "" {
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

// Restore restores files from a snapshot to the target directory.
func (r *ResticExecutor) Restore(ctx context.Context, repo Repository, snapshotID string, paths []string, target string, logger *slog.Logger) error {
	args := make([]string, 0, 5+2*len(paths))
	args = append(args, "restore", "--repo", repo.Path, "--target", target)

	for _, p := range paths {
		args = append(args, "--include", p)
	}
	args = append(args, snapshotID)

	_, stderr, err := r.runRestic(ctx, repo, args, logger)
	if err != nil {
		return fmt.Errorf("restic restore: %w\nstderr: %s", err, stderr)
	}
	return nil
}

// EnsureRepo initializes the repository if it doesn't exist.
func (r *ResticExecutor) EnsureRepo(ctx context.Context, repo Repository, logger *slog.Logger) error {
	// Try listing snapshots to see if the repo exists.
	args := []string{"snapshots", "--repo", repo.Path, "--json"}
	_, _, err := r.runRestic(ctx, repo, args, logger)
	if err == nil {
		return nil // repo already exists
	}

	// Repo inaccessible — attempt init. If already initialized, restic
	// exits non-zero with a recognizable message; treat that as success.
	initArgs := []string{"init", "--repo", repo.Path}
	_, initStderr, initErr := r.runRestic(ctx, repo, initArgs, logger)
	if initErr != nil {
		if strings.Contains(initStderr, "already initialized") ||
			strings.Contains(initStderr, "already exists") {
			return nil
		}
		return fmt.Errorf("restic init: %w\nstderr: %s", initErr, initStderr)
	}
	return nil
}

// runRestic executes a restic command and returns its stdout/stderr as
// strings, with each output stream capped to a fixed size to bound the
// agent's memory usage. Callers that need to consume large amounts of
// stdout incrementally should use streamRestic instead.
func (r *ResticExecutor) runRestic(ctx context.Context, repo Repository, args []string, logger *slog.Logger) (stdout, stderr string, err error) {
	stdoutBuf := newTailBuffer(maxStdoutBytes)
	stderr, err = r.streamRestic(ctx, repo, args, logger, func(line []byte) {
		// Re-append the newline so callers that re-split on "\n" still
		// work, and so JSON-per-line consumers see the original framing.
		_, _ = stdoutBuf.Write(line)
		_, _ = stdoutBuf.Write([]byte{'\n'})
	})
	return stdoutBuf.String(), stderr, err
}

// streamRestic runs a restic command, invoking onStdoutLine for each line
// of stdout. Stderr is captured into a bounded tail buffer (last
// maxStderrBytes bytes) and returned as a string. The caller is
// responsible for not retaining stdout lines beyond what they need.
//
// onStdoutLine is invoked from a single goroutine and the byte slice
// passed to it is only valid for the duration of the call.
func (r *ResticExecutor) streamRestic(ctx context.Context, repo Repository, args []string, logger *slog.Logger, onStdoutLine func(line []byte)) (stderr string, err error) {
	cmd := exec.CommandContext(ctx, "restic", args...)

	stderrBuf := newTailBuffer(maxStderrBytes)
	cmd.Stderr = stderrBuf

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("restic stdout pipe: %w", err)
	}

	// Set extra environment variables.
	var extraEnv []string
	extraEnv = append(extraEnv, "RESTIC_PASSWORD="+repo.Password)
	if r.RcloneConfigPath != "" {
		extraEnv = append(extraEnv, "RCLONE_CONFIG="+r.RcloneConfigPath)
	}
	cmd.Env = append(cmd.Environ(), extraEnv...)

	// Log the command with redacted sensitive data.
	logger.Info("executing command",
		"source", "restic",
		"command", "restic",
		"args", redact.Args(args),
		"env", redact.Env(extraEnv),
	)

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("restic start: %w", err)
	}

	scanner := bufio.NewScanner(stdoutPipe)
	scanner.Buffer(make([]byte, 64*1024), maxScannerLineBytes)
	for scanner.Scan() {
		if onStdoutLine != nil {
			onStdoutLine(scanner.Bytes())
		}
	}
	// Drain any remainder if the scanner stopped on a too-long line so the
	// child does not block forever writing to a full pipe. We do not
	// surface this content (it would defeat the bound), only discard it.
	if scanErr := scanner.Err(); scanErr != nil {
		_, _ = io.Copy(io.Discard, stdoutPipe)
	}

	waitErr := cmd.Wait()
	return stderrBuf.String(), waitErr
}
