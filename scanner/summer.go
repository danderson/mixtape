package scanner

import (
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"sync"

	"go.universe.tf/mixtape/db"
	"go.universe.tf/mixtape/timing"
	"golang.org/x/crypto/blake2s"
)

var done = errors.New("done")

var hashBuf = sync.Pool{
	New: func() interface{} { return make([]byte, 10*1024*1024) },
}

func Sum(d *db.DB, fsys fs.FS) error {
	for {
		err := sum(d, fsys)
		if errors.Is(err, done) {
			return nil
		} else if err != nil {
			return err
		}
	}
}

func sum(d *db.DB, fsys fs.FS) error {
	var t timing.Rec
	defer func() {
		log.Print(t.Done().DebugString())
	}()

	t.Phase("db-read")
	tx, err := d.Beginx()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	var f struct {
		ID        int64  `db:"id"`
		Path      string `db:"path"`
		FirstSeen int64  `db:"mtime_sec"`
	}
	err = tx.Get(&f, "SELECT id,path,mtime_sec FROM dirty WHERE dirty=1 LIMIT 1")
	if errors.Is(err, sql.ErrNoRows) {
		return done
	} else if err != nil {
		return fmt.Errorf("getting dirty file to sum: %w", err)
	}

	log.Printf("hashing %q", f.Path)

	t.Phase("sum")
	h, sz, err := sumOne(fsys, f.Path)
	if err != nil {
		return fmt.Errorf("hashing %q: %w", f.Path, err)
	}

	t.Phase("db-write")
	if _, err := tx.Exec("INSERT INTO files (path, size, hash, firstseen_sec) VALUES (?,?,?,?) ON CONFLICT DO NOTHING", f.Path, sz, h, f.FirstSeen); err != nil {
		return fmt.Errorf("recording file hash for %q: %w", f.Path, err)
	}
	if _, err := tx.Exec("UPDATE dirty SET dirty=0 WHERE id=?", f.ID); err != nil {
		return fmt.Errorf("clearing dirty bit on %q: %w", f.Path, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func sumOne(fsys fs.FS, path string) (h string, sz int64, err error) {
	f, err := fsys.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("opening %q: %w", path, err)
	}
	defer f.Close()
	hasher, _ := blake2s.New256(nil)
	buf := hashBuf.Get().([]byte)

	sz, err = io.CopyBuffer(hasher, f, buf)
	if err != nil {
		return "", 0, err
	}

	return hex.EncodeToString(hasher.Sum(nil)), sz, nil
}
