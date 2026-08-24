// Command migrate-turso-sync copies an existing modernc SQLite server database
// into a new Turso Sync–managed local file and pushes it to Turso Cloud.
//
// Usage:
//
//	go run ./cmd/migrate-turso-sync -from=old.db -to=new.db
//
// Reads BACKUP_DB_URL and BACKUP_DB_AUTH_TOKEN from the environment only
// (do not pass tokens on the command line — they appear in process listings).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/tryy3/backup-orchestrator/server/internal/database/migrate"
)

func main() {
	from := flag.String("from", "", "path to existing modernc SQLite server.db")
	to := flag.String("to", "", "path for new turso-sync local database (must be empty/missing)")
	force := flag.Bool("force", false, "allow DROP of existing remote app tables (required if Turso DB is not empty)")
	flag.Parse()

	if strings.TrimSpace(*from) == "" || strings.TrimSpace(*to) == "" {
		flag.Usage()
		os.Exit(2)
	}

	url := strings.TrimSpace(os.Getenv("BACKUP_DB_URL"))
	token := strings.TrimSpace(os.Getenv("BACKUP_DB_AUTH_TOKEN"))
	if url == "" || token == "" {
		log.Fatalf("migrate-turso-sync: BACKUP_DB_URL and BACKUP_DB_AUTH_TOKEN must be set in the environment")
	}

	ctx := context.Background()
	err := migrate.SQLiteToTursoSync(ctx, migrate.Options{
		FromPath:  *from,
		ToPath:    *to,
		URL:       url,
		AuthToken: token,
		Force:     *force,
	})
	if err != nil {
		log.Fatalf("migrate-turso-sync: %v", err)
	}
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("  1. Set BACKUP_DB_PATH=%s\n", *to)
	fmt.Printf("  2. Set BACKUP_DB_DRIVER=turso-sync\n")
	fmt.Printf("  3. Keep BACKUP_DB_URL / BACKUP_DB_AUTH_TOKEN / BACKUP_ENCRYPTION_KEY\n")
	fmt.Printf("  4. Start the server\n")
}
