package db

import (
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type DB struct {
	db *sqlx.DB
}

var schema = `
CREATE TABLE IF NOT EXISTS dirty (
  id INTEGER PRIMARY KEY,
  path TEXT,
  mtime INTEGER,
  size INTEGER,
  inode INTEGER,
  canread INTEGER,

  dirty INTEGER
);

CREATE UNIQUE INDEX IF NOT EXISTS dirty_path_idx on dirty (path)
`

func Open(path string) (*DB, error) {
	db, err := sqlx.Connect("sqlite", "file:"+path)
	if err != nil {
		return nil, fmt.Errorf("opening %q: %v", path, err)
	}

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("applying schema to %q: %v", path, err)
	}

	return &DB{db}, nil
}

func (db *DB) Close() error {
	return db.db.Close()
}
