package db

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/tailscale/sqlite"
)

type DB struct {
	*sqlx.DB
}

func Open(path string) (*DB, error) {
	db, err := sqlx.Connect("sqlite3", "file:"+path)
	if err != nil {
		return nil, fmt.Errorf("opening %q: %v", path, err)
	}

	// Limit to a single Conn that never expires, so per-Conn state
	// remains the same.
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	db.SetMaxOpenConns(0)

	conn, err := db.Conn(context.Background())
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("getting DB conn: %w", err)
	}
	const init = `
PRAGMA journal_mode=WAL;
PRAGMA temp_store=MEMORY;
`
	if err := sqlite.ExecScript(conn, init); err != nil {
		db.Close()
		return nil, fmt.Errorf("initializing %q: %w", path, err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying migrations to %q: %w", path, err)
	}
	if err := conn.Close(); err != nil {
		db.Close()
		return nil, fmt.Errorf("returning Conn to pool: %w", err)
	}

	return &DB{db}, nil
}
