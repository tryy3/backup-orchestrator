package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/tryy3/backup-orchestrator/server/internal/database"
	_ "modernc.org/sqlite"
	tursogo "turso.tech/database/tursogo"
	turso "turso.tech/database/tursogo-serverless"
)

// Tables in FK-safe order (parents before children).
var copyTables = []string{
	"agents",
	"repositories",
	"settings",
	"scripts",
	"backup_plans",
	"backup_plan_repositories",
	"plan_hooks",
	"jobs",
	"job_repository_results",
	"job_hook_results",
}

// Options for sqlite → turso-sync data copy.
type Options struct {
	FromPath  string
	ToPath    string
	URL       string
	AuthToken string
	// Force allows DROP of existing remote app tables. Without Force, a remote
	// that already has any app table is refused so a wrong URL cannot wipe data.
	Force bool
}

// SQLiteToTursoSync copies rows from a modernc SQLite file into Turso Cloud
// (remote-only), then bootstraps a new empty turso-sync local file via Pull().
//
// Do not CREATE TABLE locally and Push: that path has produced MVCC logical
// logs that panic on reopen ("negative root page and a positive root page").
// Remote import + Pull bootstrap is the supported cutover.
func SQLiteToTursoSync(ctx context.Context, opts Options) error {
	if opts.FromPath == "" || opts.ToPath == "" {
		return fmt.Errorf("from and to paths are required")
	}
	if opts.FromPath == opts.ToPath {
		return fmt.Errorf("from and to paths must differ (move the old file aside first)")
	}
	if opts.URL == "" || opts.AuthToken == "" {
		return fmt.Errorf("url and auth token are required")
	}
	if _, err := os.Stat(opts.FromPath); err != nil {
		return fmt.Errorf("source database: %w", err)
	}
	if info, err := os.Stat(opts.ToPath); err == nil && info.Size() > 0 {
		return fmt.Errorf("destination %q already exists and is non-empty; refuse to overwrite", opts.ToPath)
	}
	removeSyncSidecars(opts.ToPath)

	src, err := sql.Open("sqlite", opts.FromPath)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer func() { _ = src.Close() }()

	fmt.Println("step 1/2: import into Turso remote…")
	if err := importToRemote(ctx, src, opts); err != nil {
		return err
	}

	fmt.Println("step 2/2: bootstrap local turso-sync file via Pull…")
	if err := bootstrapSyncLocal(ctx, opts); err != nil {
		return err
	}
	fmt.Println("local sync file ready")
	return nil
}

func importToRemote(ctx context.Context, src *sql.DB, opts Options) error {
	remote := sql.OpenDB(turso.NewConnector(opts.URL, opts.AuthToken))
	remote.SetMaxOpenConns(4)
	remote.SetConnMaxLifetime(5 * time.Minute)
	defer func() { _ = remote.Close() }()

	if err := remote.PingContext(ctx); err != nil {
		return fmt.Errorf("ping turso remote: %w", err)
	}

	existing, err := remoteAppTables(ctx, remote)
	if err != nil {
		return fmt.Errorf("inspect remote tables: %w", err)
	}
	if len(existing) > 0 && !opts.Force {
		return fmt.Errorf("remote already has app tables %v; refusing to DROP without -force (wrong URL would destroy data)", existing)
	}

	// Clear prior app data so re-runs are idempotent (FK-safe reverse order).
	for i := len(copyTables) - 1; i >= 0; i-- {
		table := copyTables[i]
		if _, err := remote.ExecContext(ctx, "DROP TABLE IF EXISTS "+table); err != nil {
			return fmt.Errorf("drop remote %s: %w", table, err)
		}
	}

	if err := database.MigrateConn(ctx, remote); err != nil {
		return fmt.Errorf("migrate remote schema: %w", err)
	}

	for _, table := range copyTables {
		n, err := copyTable(ctx, src, remote, table)
		if err != nil {
			return fmt.Errorf("copy table %s: %w", table, err)
		}
		fmt.Printf("  copied %s: %d rows\n", table, n)
	}
	return nil
}

func bootstrapSyncLocal(ctx context.Context, opts Options) error {
	bootstrap := true
	syncDB, err := tursogo.NewTursoSyncDb(ctx, tursogo.TursoSyncDbConfig{
		Path:             opts.ToPath,
		RemoteUrl:        opts.URL,
		AuthToken:        opts.AuthToken,
		BootstrapIfEmpty: &bootstrap,
	})
	if err != nil {
		return fmt.Errorf("open turso-sync destination: %w", err)
	}

	sqlDB, err := syncDB.Connect(ctx)
	if err != nil {
		return fmt.Errorf("connect turso-sync destination: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	// BootstrapIfEmpty may already have pulled; call Pull again to be sure.
	if _, err := syncDB.Pull(ctx); err != nil {
		return fmt.Errorf("pull bootstrap: %w", err)
	}

	var n int
	if err := sqlDB.QueryRowContext(ctx, "SELECT count(*) FROM agents").Scan(&n); err != nil {
		return fmt.Errorf("verify local agents after pull: %w", err)
	}
	fmt.Printf("  local agents after pull: %d\n", n)
	return nil
}

func removeSyncSidecars(path string) {
	_ = os.Remove(path)
	for _, suffix := range []string{"-wal", "-shm", "-changes", "-info", "-log", "-wal-revert"} {
		_ = os.Remove(path + suffix)
	}
}

func copyTable(ctx context.Context, src, dst *sql.DB, table string) (int, error) {
	exists, err := tableExists(ctx, src, table)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}

	rows, err := src.QueryContext(ctx, "SELECT * FROM "+table)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	if len(cols) == 0 {
		return 0, nil
	}

	placeholders := make([]string, len(cols))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	insertSQL := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	n := 0
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return n, fmt.Errorf("scan: %w", err)
		}
		if _, err := dst.ExecContext(ctx, insertSQL, raw...); err != nil {
			return n, fmt.Errorf("insert: %w", err)
		}
		n++
	}
	return n, rows.Err()
}

func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, name,
	).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func remoteAppTables(ctx context.Context, db *sql.DB) ([]string, error) {
	var found []string
	for _, table := range copyTables {
		ok, err := tableExists(ctx, db, table)
		if err != nil {
			return nil, err
		}
		if ok {
			found = append(found, table)
		}
	}
	return found, nil
}
