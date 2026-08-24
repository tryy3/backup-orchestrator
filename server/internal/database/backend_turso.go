package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	tursogo "turso.tech/database/tursogo"
	turso "turso.tech/database/tursogo-serverless"
)

func init() {
	newTursoSync = openTursoSyncSDK
}

func openTursoRemote(opts Options) (*DB, error) {
	if opts.URL == "" || opts.AuthToken == "" {
		return nil, fmt.Errorf("BACKUP_DB_URL and BACKUP_DB_AUTH_TOKEN are required for driver %q", DriverTurso)
	}
	ctx := context.Background()
	sqlDB := sql.OpenDB(turso.NewConnector(opts.URL, opts.AuthToken))
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("ping turso: %w", err)
	}
	if err := enableForeignKeys(ctx, sqlDB); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	db := &DB{DB: sqlDB, encryptionKey: opts.EncryptionKey}
	if err := db.migrate(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	if len(opts.EncryptionKey) == 32 {
		if err := db.migrateEncryption(ctx); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("encryption migration: %w", err)
		}
	}
	return db, nil
}

func openTursoSyncSDK(opts Options) (*sql.DB, remoteSync, error) {
	ctx := context.Background()
	bootstrap := false
	syncDB, err := tursogo.NewTursoSyncDb(ctx, tursogo.TursoSyncDbConfig{
		Path:             opts.Path,
		RemoteUrl:        opts.URL,
		AuthToken:        opts.AuthToken,
		BootstrapIfEmpty: &bootstrap,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open turso-sync: %w", err)
	}

	sqlDB, err := syncDB.Connect(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("connect turso-sync: %w", err)
	}
	sqlDB.SetMaxOpenConns(8)
	sqlDB.SetMaxIdleConns(2)

	return sqlDB, tursoSyncAdapter{db: syncDB}, nil
}

type tursoSyncAdapter struct {
	db *tursogo.TursoSyncDb
}

func (a tursoSyncAdapter) Pull(ctx context.Context) error {
	_, err := a.db.Pull(ctx)
	return err
}

func (a tursoSyncAdapter) Push(ctx context.Context) error {
	return a.db.Push(ctx)
}
