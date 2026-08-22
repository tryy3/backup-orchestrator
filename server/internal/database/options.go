package database

import (
	"fmt"
	"time"
)

type Driver string

const (
	DriverSQLite    Driver = "sqlite"
	DriverTurso     Driver = "turso"
	DriverTursoSync Driver = "turso-sync"
)

type Options struct {
	Driver        Driver
	Path          string
	URL           string
	AuthToken     string
	SyncInterval  time.Duration
	EncryptionKey []byte
}

func Open(opts Options) (*DB, error) {
	if opts.Driver == "" {
		opts.Driver = DriverSQLite
	}
	switch opts.Driver {
	case DriverSQLite:
		return openSQLite(opts)
	case DriverTurso, DriverTursoSync:
		return nil, fmt.Errorf("database driver %q is not implemented yet", opts.Driver)
	default:
		return nil, fmt.Errorf("unknown database driver %q", opts.Driver)
	}
}

func New(path string, encryptionKey []byte) (*DB, error) {
	return Open(Options{Driver: DriverSQLite, Path: path, EncryptionKey: encryptionKey})
}
