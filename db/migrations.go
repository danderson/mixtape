package db

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

func migrate(db *sqlx.DB) error {
	tx, err := db.BeginTxx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("DB migration begin transaction: %w", err)
	}
	defer tx.Rollback()

	var idx int
	err = tx.Get(&idx, "PRAGMA user_version")
	if err != nil {
		return fmt.Errorf("getting latest applied migration: %w", err)
	}

	if idx == len(migrations) {
		// Already fully migrated, nothing needed.
	} else if idx > len(migrations) {
		return fmt.Errorf("database is at version %d, which is more recent than this binary understands", idx)
	}

	for i, f := range migrations[idx:] {
		if err := f(tx); err != nil {
			return fmt.Errorf("migration to version %d failed: %w", i+1, err)
		}
	}

	// For some reason, ? substitution doesn't work in PRAGMA
	// statements, sqlite reports a parse error.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", len(migrations))); err != nil {
		return fmt.Errorf("recording new DB version: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("DB migration commit transaction: %w", err)
	}

	return nil
}

func sql(idl ...string) func(*sqlx.Tx) error {
	return func(tx *sqlx.Tx) error {
		for _, stmt := range idl {
			if _, err := tx.Exec(stmt); err != nil {
				return err
			}
		}
		return nil
	}
}

var migrations = []func(*sqlx.Tx) error{
	sql(`CREATE TABLE dirty (
           id INTEGER PRIMARY KEY,
           path TEXT,
           mtime_sec INTEGER,
           mtime_nano INTEGER,
           size INTEGER,
           dev INTEGER,
           inode INTEGER,
           readable INTEGER,
           dirty INTEGER
         )`,
		`CREATE UNIQUE INDEX dirty_path_idx ON dirty (path)`,
		`CREATE TABLE files (
           id INTEGER PRIMARY KEY,
           path TEXT,
           size INTEGER,
           blake2s TEXT,
           firstseen_sec INTEGER
         )`,
		`CREATE UNIQUE INDEX files_path_hash_idx on files (path, blake2s)`),
	sql(`ALTER TABLE files RENAME COLUMN blake2s TO hash`),
}
