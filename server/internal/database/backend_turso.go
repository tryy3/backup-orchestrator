package database

import (
	"context"
	"database/sql"
	"fmt"

	turso "turso.tech/database/tursogo"
)

func init() {
	newTursoSync = openTursoSyncSDK
}

func openTursoSyncSDK(opts Options) (*sql.DB, remoteSync, error) {
	ctx := context.Background()
	bootstrap := false
	syncDB, err := turso.NewTursoSyncDb(ctx, turso.TursoSyncDbConfig{
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
	db *turso.TursoSyncDb
}

func (a tursoSyncAdapter) Pull(ctx context.Context) error {
	_, err := a.db.Pull(ctx)
	return err
}

func (a tursoSyncAdapter) Push(ctx context.Context) error {
	return a.db.Push(ctx)
}
