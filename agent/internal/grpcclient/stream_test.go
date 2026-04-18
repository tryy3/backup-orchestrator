package grpcclient

import (
	"testing"
	"time"

	backupv1 "github.com/tryy3/backup-orchestrator/agent/internal/gen/backup/v1"
)

func TestCommandTimeout(t *testing.T) {
	tests := []struct {
		name string
		cmd  *backupv1.Command
		want time.Duration
	}{
		{
			name: "trigger_backup gets long timeout",
			cmd: &backupv1.Command{
				Action: &backupv1.Command_TriggerBackup{TriggerBackup: &backupv1.TriggerBackup{}},
			},
			want: defaultBackupCommandTimeout,
		},
		{
			name: "trigger_restore gets long timeout",
			cmd: &backupv1.Command{
				Action: &backupv1.Command_TriggerRestore{TriggerRestore: &backupv1.TriggerRestore{}},
			},
			want: defaultRestoreCommandTimeout,
		},
		{
			name: "list_snapshots gets medium timeout",
			cmd: &backupv1.Command{
				Action: &backupv1.Command_ListSnapshots{ListSnapshots: &backupv1.ListSnapshots{}},
			},
			want: defaultListSnapshotsTimeout,
		},
		{
			name: "browse_snapshot gets medium timeout",
			cmd: &backupv1.Command{
				Action: &backupv1.Command_BrowseSnapshot{BrowseSnapshot: &backupv1.BrowseSnapshot{}},
			},
			want: defaultBrowseSnapshotTimeout,
		},
		{
			name: "browse_filesystem gets short timeout",
			cmd: &backupv1.Command{
				Action: &backupv1.Command_BrowseFilesystem{BrowseFilesystem: &backupv1.BrowseFilesystem{}},
			},
			want: defaultBrowseFSCommandTimeout,
		},
		{
			name: "unknown action gets default timeout",
			cmd:  &backupv1.Command{},
			want: defaultCommandTimeout,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commandTimeout(tt.cmd)
			if got != tt.want {
				t.Errorf("commandTimeout() = %v, want %v", got, tt.want)
			}
		})
	}

	// Sanity: backup/restore must be strictly longer than browse timeouts, so
	// a short browse-kind default can't ever apply to a long-running backup.
	if defaultBackupCommandTimeout <= defaultBrowseFSCommandTimeout {
		t.Errorf("backup timeout (%v) must be longer than browse_fs timeout (%v)",
			defaultBackupCommandTimeout, defaultBrowseFSCommandTimeout)
	}
}
