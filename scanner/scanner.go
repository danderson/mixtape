package scanner

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"sort"
	"syscall"

	"go.universe.tf/mixtape/db"
	"go.universe.tf/mixtape/timing"
)

// Scan adds all files in roots to the DB's dirty table, marking
// potentially-changed files dirty as it goes.
func Scan(d *db.DB, fsys fs.FS, roots []string) error {
	var t timing.Rec
	defer func() {
		log.Print(t.Done().DebugString())
	}()
	t.Phase("validate")

	for _, root := range roots {
		if !fs.ValidPath(root) {
			return fmt.Errorf("invalid root path %q", root)
		}
	}

	t.Phase("disk-scan")
	var disk []*fileInfo
	for _, root := range roots {
		err := fs.WalkDir(fsys, root, func(path string, ent fs.DirEntry, err error) error {
			inf, err := toInfo(path, ent, err)
			if err != nil {
				return err
			}
			if inf != nil {
				disk = append(disk, inf)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("scanning root %q: %v", root, err)
		}
	}
	t.Phase("disk-sort")
	sort.Slice(disk, func(i, j int) bool {
		return disk[i].Path < disk[j].Path
	})

	t.Phase("db-read")
	var last []*fileInfo
	tx, err := d.Beginx()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	err = tx.Select(&last, "SELECT path,mtime_sec,mtime_nano,size,dev,inode,readable FROM dirty")
	if err != nil {
		return fmt.Errorf("reading previous dirty state: %w", err)
	}

	t.Phase("db-sort")
	// Sort in Go rather than SQL, to avoid unicode collation differences.
	sort.Slice(last, func(i, j int) bool {
		return last[i].Path < last[j].Path
	})

	t.Phase("diff")
	insert, update, delete := diff(disk, last)

	t.Phase("db-write")
	for _, st := range insert {
		if _, err := tx.Exec("INSERT INTO dirty (path, mtime_sec, mtime_nano, size, dev, inode, readable, dirty) VALUES (?,?,?,?,?,?,?,1)", st.Path, st.Msec, st.Mnano, st.Size, st.Dev, st.Inode, st.Readable); err != nil {
			return fmt.Errorf("inserting fileinfo for %q: %w", st.Path, err)
		}
	}
	for _, st := range update {
		if _, err := tx.Exec("UPDATE dirty SET mtime_sec=?,mtime_nano=?,size=?,dev=?,inode=?,readable=?,dirty=1 WHERE path=?", st.Msec, st.Mnano, st.Size, st.Dev, st.Inode, st.Readable, st.Path); err != nil {
			return fmt.Errorf("updating fileinfo for %q: %w", st.Path, err)
		}
	}
	for _, st := range delete {
		if _, err := tx.Exec("DELETE FROM dirty WHERE path=?", st.Path); err != nil {
			return fmt.Errorf("deleting fileinfo for %q: %w", st.Path, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing dirty file set: %w", err)
	}

	return nil
}

func diff(disk, db []*fileInfo) (insert, update, delete []*fileInfo) {
	for len(disk) > 0 && len(db) > 0 {
		a, b := disk[0], db[0]
		switch {
		case *a == *b: // In sync
			disk = disk[1:]
			db = db[1:]
		case a.Path == b.Path: // metadata changed
			update = append(update, a)
			disk = disk[1:]
			db = db[1:]
		case a.Path < b.Path: // On disk but not in DB, fresh insert
			insert = append(insert, a)
			disk = disk[1:]
		default: // In DB but not on disk, delete
			delete = append(delete, b)
			db = db[1:]
		}
	}
	insert = append(insert, disk...)
	delete = append(delete, db...)
	return insert, update, delete
}

type fileInfo struct {
	Path     string
	Msec     int64 `db:"mtime_sec"`
	Mnano    int64 `db:"mtime_nano"`
	Size     uint64
	Dev      uint64
	Inode    uint64
	Readable bool
}

func toInfo(path string, ent fs.DirEntry, err error) (*fileInfo, error) {
	if ent.Type() != 0 {
		// Process all subdirs, ignore other irregular files.
		return nil, nil
	}

	info, err := ent.Info()
	if errors.Is(err, fs.ErrNotExist) {
		// Delete race, nothing to do.
		return nil, nil
	} else if errors.Is(err, fs.ErrPermission) {
		return &fileInfo{
			Path:     path,
			Readable: false,
		}, nil
	}

	inf := &fileInfo{
		Path:     path,
		Msec:     info.ModTime().Unix(),
		Mnano:    int64(info.ModTime().Nanosecond()),
		Size:     uint64(info.Size()),
		Readable: true,
	}
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		inf.Dev = sys.Dev
		inf.Inode = sys.Ino
	}
	return inf, nil
}
