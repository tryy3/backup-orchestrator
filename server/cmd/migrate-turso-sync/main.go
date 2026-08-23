// Command migrate-turso-sync copies an existing modernc SQLite server database
// into a new Turso Sync–managed local file and pushes it to Turso Cloud.
//
// Usage:
//
//	go run ./cmd/migrate-turso-sync -from=old.db -to=new.db
//
// Reads BACKUP_DB_URL and BACKUP_DB_AUTH_TOKEN from the environment (or flags).
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
	url := flag.String("url", os.Getenv("BACKUP_DB_URL"), "Turso database URL")
	token := flag.String("token", os.Getenv("BACKUP_DB_AUTH_TOKEN"), "Turso auth token")
	flag.Parse()

	if strings.TrimSpace(*from) == "" || strings.TrimSpace(*to) == "" {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()
	err := migrate.SQLiteToTursoSync(ctx, migrate.Options{
		FromPath:  *from,
		ToPath:    *to,
		URL:       strings.TrimSpace(*url),
		AuthToken: strings.TrimSpace(*token),
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
