package executor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTailBuffer_SmallWritesNotTruncated(t *testing.T) {
	tb := newTailBuffer(64)
	tb.Write([]byte("hello "))
	tb.Write([]byte("world"))
	if got, want := tb.String(), "hello world"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTailBuffer_KeepsTrailingBytesWhenOverflowing(t *testing.T) {
	tb := newTailBuffer(8)
	tb.Write([]byte("0123456789ABCDEF"))
	got := tb.String()
	if !strings.HasPrefix(got, "...[truncated]...\n") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
	tail := strings.TrimPrefix(got, "...[truncated]...\n")
	if tail != "89ABCDEF" {
		t.Fatalf("expected last 8 bytes 89ABCDEF, got %q", tail)
	}
}

func TestTailBuffer_IncrementalOverflow(t *testing.T) {
	tb := newTailBuffer(4)
	for _, s := range []string{"aa", "bb", "cc", "dd"} {
		tb.Write([]byte(s))
	}
	got := strings.TrimPrefix(tb.String(), "...[truncated]...\n")
	if got != "ccdd" {
		t.Fatalf("expected last 4 bytes ccdd, got %q", got)
	}
}

// newFakeRestic creates a temp directory containing an executable named
// "restic" that runs the given shell snippet, and prepends that directory
// to PATH for the duration of the test.
func newFakeRestic(t *testing.T, script string) *ResticExecutor {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("script-based fake restic not supported on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "restic")
	body := "#!/bin/sh\n" + script + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing fake restic: %v", err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	return &ResticExecutor{}
}

func safePrefix(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

func TestStreamRestic_StreamsLines(t *testing.T) {
	r := newFakeRestic(t, `printf 'line1\nline2\nline3\n'; printf 'an error\n' 1>&2; exit 0`)
	var lines []string
	stderr, err := r.streamRestic(context.Background(), Repository{Password: "x"}, []string{"snapshots"}, discardLogger(), func(line []byte) {
		lines = append(lines, string(line))
	})
	if err != nil {
		t.Fatalf("streamRestic: %v", err)
	}
	if got := strings.Join(lines, "|"); got != "line1|line2|line3" {
		t.Fatalf("unexpected lines: %q", got)
	}
	if !strings.Contains(stderr, "an error") {
		t.Fatalf("expected stderr to contain 'an error', got %q", stderr)
	}
}

func TestStreamRestic_StderrIsBounded(t *testing.T) {
	// Emit ~32 KiB of stderr; expect only the last ~4 KiB retained.
	// Use a password that does not appear in the generated content so that
	// the redact.String pass does not expand the already-truncated tail.
	r := newFakeRestic(t, `awk 'BEGIN{for(i=0;i<32000;i++)printf "x"}' 1>&2; exit 0`)
	stderr, err := r.streamRestic(context.Background(), Repository{Password: "TESTPASSWORD"}, []string{"snapshots"}, discardLogger(), func(line []byte) {})
	if err != nil {
		t.Fatalf("streamRestic: %v", err)
	}
	if !strings.HasPrefix(stderr, "...[truncated]...\n") {
		t.Fatalf("expected truncation marker, got prefix %q", safePrefix(stderr, 40))
	}
	if len(stderr) > maxStderrBytes+64 {
		t.Fatalf("stderr length %d exceeded cap %d", len(stderr), maxStderrBytes)
	}
}

func TestRunRestic_StdoutBoundedToTail(t *testing.T) {
	// Emit > 1 MiB of stdout; runRestic should retain only ~maxStdoutBytes.
	r := newFakeRestic(t, `awk 'BEGIN{for(i=0;i<200000;i++)print "0123456789"}'`)
	stdout, _, err := r.runRestic(context.Background(), Repository{Password: "x"}, []string{"snapshots"}, discardLogger())
	if err != nil {
		t.Fatalf("runRestic: %v", err)
	}
	if !strings.HasPrefix(stdout, "...[truncated]...\n") {
		t.Fatalf("expected truncation marker, got prefix %q", safePrefix(stdout, 40))
	}
	if len(stdout) > maxStdoutBytes+64 {
		t.Fatalf("stdout length %d exceeded cap %d", len(stdout), maxStdoutBytes)
	}
}

func TestBackup_DiscardsStatusLinesAndKeepsSummary(t *testing.T) {
	// Emit many "status" messages followed by a single "summary".
	script := `awk 'BEGIN{
		for(i=0;i<5000;i++)print "{\"message_type\":\"status\",\"current_files\":[\"/some/very/long/path/" i "\"],\"percent_done\":0.5}";
		print "{\"message_type\":\"summary\",\"files_new\":3,\"files_changed\":2,\"files_unmodified\":1,\"data_added\":100,\"total_bytes_processed\":200,\"total_duration\":1.5,\"snapshot_id\":\"abcdef\"}";
	}'`
	r := newFakeRestic(t, script)
	res, err := r.Backup(context.Background(), Repository{Password: "x", Path: "/tmp/repo"}, []string{"/data"}, nil, nil, discardLogger())
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if res.SnapshotID != "abcdef" {
		t.Errorf("snapshot id = %q, want abcdef", res.SnapshotID)
	}
	if res.FilesNew != 3 || res.FilesChanged != 2 || res.FilesUnmodified != 1 {
		t.Errorf("file counts = (%d,%d,%d), want (3,2,1)", res.FilesNew, res.FilesChanged, res.FilesUnmodified)
	}
	if res.BytesAdded != 100 || res.TotalBytes != 200 {
		t.Errorf("bytes = (%d,%d), want (100,200)", res.BytesAdded, res.TotalBytes)
	}
}

func TestBackup_NoSummaryReportsTail(t *testing.T) {
	script := `awk 'BEGIN{for(i=0;i<10;i++)print "{\"message_type\":\"status\"}"}'`
	r := newFakeRestic(t, script)
	_, err := r.Backup(context.Background(), Repository{Password: "x", Path: "/tmp/repo"}, []string{"/data"}, nil, nil, discardLogger())
	if err == nil {
		t.Fatal("expected error when no summary message is emitted")
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Errorf("error should mention summary: %v", err)
	}
}

func TestBackup_SummaryAmongOtherMessages(t *testing.T) {
	summary := map[string]any{
		"message_type":          "summary",
		"files_new":             10,
		"files_changed":         20,
		"files_unmodified":      30,
		"data_added":            12345,
		"total_bytes_processed": 67890,
		"total_duration":        2.5,
		"snapshot_id":           "deadbeef",
	}
	sumLine, _ := json.Marshal(summary)
	script := `printf '%s\n' '{"message_type":"verbose_status","action":"new"}' '{"message_type":"status","percent_done":0.1}' '` + string(sumLine) + `'`
	r := newFakeRestic(t, script)
	res, err := r.Backup(context.Background(), Repository{Password: "x", Path: "/tmp/repo"}, []string{"/data"}, nil, nil, discardLogger())
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if res.SnapshotID != "deadbeef" {
		t.Errorf("snapshot id = %q, want deadbeef", res.SnapshotID)
	}
}
