package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

type DB struct {
	db *sqlx.DB
}

var schema = `
PRAGMA journal_mode=WAL;

CREATE TABLE IF NOT EXISTS dirty (
  id INTEGER PRIMARY KEY,
  path TEXT,
  mtime DATETIME,
  size INTEGER,
  dev INTEGER,
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

func (db *DB) ResetSeen() error {
	const tempSeen = `
DROP TABLE IF EXISTS temp.seen;
CREATE TEMPORARY TABLE temp.seen (path TEXT);
CREATE UNIQUE INDEX temp.seen_idx on seen (path)
`
	if _, err := db.db.Exec(tempSeen); err != nil {
		return fmt.Errorf("resetting seen temporary set: %v", err)
	}
	return nil
}

type FileInfo struct {
	Path    string
	Mtime   time.Time
	Size    uint64
	Dev     uint64
	Inode   uint64
	CanRead bool
}

func (db *DB) FileInfo(path string) (FileInfo, error) {
	var ret FileInfo
	err := db.db.Get(&ret, "SELECT path,mtime,size,dev,inode,canread FROM dirty WHERE path = ?", path)
	if err == sql.ErrNoRows {
		return FileInfo{Path: path}, nil
	} else if err != nil {
		return FileInfo{}, err
	}
	return ret, nil
}

func (db *DB) NoteUnreadableFile(path string) (err error) {
	_, err = db.db.Exec("INSERT INTO dirty(path) VALUES(?) ON CONFLICT(path) DO UPDATE SET mtime=0,size=0,dev=0,inode=0,canread=0,dirty=0", path)
	if err != nil {
		return fmt.Errorf("noting %q as unreadable: %v", path, err)
	}
	if err := db.noteSeen(db.db, path); err != nil {
		return err
	}
	return nil
}

func (db *DB) NoteSeen(path string) error {
	return db.noteSeen(db.db, path)
}

func (db *DB) noteSeen(e sqlx.Execer, path string) error {
	_, err := db.db.Exec("INSERT INTO temp.seen(path) VALUES(?) ON CONFLICT DO NOTHING", path)
	if err != nil {
		return fmt.Errorf("marking %q seen: %v", path, err)
	}
	return nil
}

func (db *DB) DirtyFile(info FileInfo) error {
	_, err := db.db.Exec("INSERT INTO dirty(path,mtime,size,dev,inode,canread,dirty) VALUES(?,?,?,?,?,1,1) ON CONFLICT(path) DO UPDATE SET mtime=?,size=?,dev=?,inode=?,canread=1,dirty=1", info.Path, info.Mtime, info.Size, info.Dev, info.Inode, info.Mtime, info.Size, info.Dev, info.Inode)
	if err != nil {
		return fmt.Errorf("marking %q dirty: %v", info.Path, err)
	}
	if err = db.noteSeen(db.db, info.Path); err != nil {
		return err
	}
	return nil
}

func (db *DB) DeleteUnseen() error {
	_, err := db.db.Exec("DELETE FROM dirty WHERE NOT EXISTS (SELECT 1 FROM temp.seen WHERE temp.seen.path = dirty.path)")
	if err != nil {
		return fmt.Errorf("deleting unseen files from dirty: %v", err)
	}
	// Best-effort cleanup of the seen table, to save space.
	db.db.Exec("DELETE FROM temp.seen")
	return nil
}
