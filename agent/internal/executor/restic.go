package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"

	"github.com/tryy3/backup-orchestrator/agent/internal/redact"
)

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

	start := time.Now()
	stdout, stderr, err := r.runRestic(ctx, repo, args, logger)
	durationMs := time.Since(start).Milliseconds()

	if err != nil {
		return nil, fmt.Errorf("restic backup: %w\nstderr: %s", err, stderr)
	}

	// Parse JSON summary from stdout. Restic outputs multiple JSON lines;
	// the summary line has message_type "summary".
	var summary resticSummary
	found := false
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var msg struct {
			MessageType string `json:"message_type"`
		}
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if msg.MessageType == "summary" {
			if err := json.Unmarshal([]byte(line), &summary); err != nil {
				return nil, fmt.Errorf("parsing restic summary: %w", err)
			}
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("restic backup did not produce a summary message\nstdout: %s", stdout)
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

// runRestic executes a restic command with the correct environment variables.
func (r *ResticExecutor) runRestic(ctx context.Context, repo Repository, args []string, logger *slog.Logger) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, "restic", args...)

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

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

	err = cmd.Run()
	return stdoutBuf.String(), stderrBuf.String(), err
}
