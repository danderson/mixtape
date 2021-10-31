package scanner

import (
	"errors"
	"fmt"
	"io/fs"
	"syscall"

	"go.universe.tf/mixtape/db"
)

// Scan adds all files in roots to the DB's dirty table, marking
// potentially-changed files dirty as it goes.
func Scan(db *db.DB, fsys fs.FS, roots []string) error {
	if err := db.ResetSeen(); err != nil {
		return err
	}

	for _, root := range roots {
		if !fs.ValidPath(root) {
			return fmt.Errorf("invalid root path %q", root)
		}
	}

	for _, root := range roots {
		err := fs.WalkDir(fsys, root, func(path string, ent fs.DirEntry, err error) error {
			return look(db, path, ent, err)
		})
		if err != nil {
			return fmt.Errorf("scanning root %q: %v", root, err)
		}
	}

	if err := db.DeleteUnseen(); err != nil {
		return err
	}

	return nil
}

func look(db *db.DB, path string, ent fs.DirEntry, err error) error {
	if ent.Type() != 0 {
		// Process all subdirs, ignore other irregular files.
		return nil
	}

	fsinfo, err := ent.Info()
	if errors.Is(err, fs.ErrNotExist) {
		// Delete race, nothing to do.
		return nil
	} else if errors.Is(err, fs.ErrPermission) {
		if err := db.NoteUnreadableFile(path); err != nil {
			return err
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("stat(%q): %v", path, err)
	}
	actual := dbFileInfo(path, fsinfo)

	cached, err := db.FileInfo(path)
	if err != nil {
		return fmt.Errorf("getting cached info for %q: %v", path, err)
	}

	if actual == cached {
		// No changes, nothing required.
		if err := db.NoteSeen(path); err != nil {
			return err
		}
		return nil
	}

	//log.Print("dirty: ", path)
	if err := db.DirtyFile(actual); err != nil {
		return err
	}

	return nil
}

func dbFileInfo(path string, info fs.FileInfo) db.FileInfo {
	ret := db.FileInfo{
		Path:    path,
		Mtime:   info.ModTime(),
		Size:    uint64(info.Size()),
		CanRead: true,
	}
	sys, ok := info.Sys().(syscall.Stat_t)
	if !ok {
		return ret
	}
	ret.Dev = sys.Dev
	ret.Inode = sys.Ino
	return ret
}
