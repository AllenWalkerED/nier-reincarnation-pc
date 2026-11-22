package pcsave

// Moving a save between the phone (Bunker) and the PC, in the format of the
// phone app's Export save backup: a ZIP of game.db, auth.db and auth.key
// (the format of Bunker's `bunker-server saves`).
//
//	export: a slot's game.db (a consistent copy), with this PC's auth.db and
//	        auth.key, to a ZIP the phone app imports.
//	import: a phone backup (ZIP) or a bare game.db becomes a NEW slot; the
//	        saves already here are not touched. The PC keeps its own sign-in
//	        files: switching to the slot gives its player this PC's ID
//	        (adoptDevice), as Bunker's own import of a bare game.db does.

import (
	"archive/zip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s store) exportPhone(f *slotFile, id, out string) error {
	if out == "" {
		return errors.New("no file to write")
	}
	p := s.dbPath(f, id)
	if _, err := os.Stat(p); err != nil {
		return errors.New("This save has not been played yet: there is nothing to export.")
	}
	tmpDir, err := os.MkdirTemp(s.dir, "export-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	copyDB := filepath.Join(tmpDir, "game.db")
	if err := vacuumInto(p, copyDB); err != nil {
		return err
	}
	temp := out + ".writing"
	file, err := os.Create(temp)
	if err != nil {
		return err
	}
	w := zip.NewWriter(file)
	add := func(name, from string) error {
		src, err := os.Open(from)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		defer src.Close()
		hdr := &zip.FileHeader{Name: name, Method: zip.Deflate}
		hdr.Modified = time.Now()
		dst, err := w.CreateHeader(hdr)
		if err != nil {
			return err
		}
		_, err = io.Copy(dst, src)
		return err
	}
	err = add("game.db", copyDB)
	if err == nil {
		err = add("auth.db", filepath.Join(s.dir, "auth.db"))
	}
	if err == nil {
		err = add("auth.key", filepath.Join(s.dir, "auth.key"))
	}
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(temp)
		return err
	}
	os.Remove(out)
	return os.Rename(temp, out)
}

func (s store) importPhone(f *slotFile, from, name string) (string, error) {
	if from == "" {
		return "", errors.New("no file to import")
	}
	if name == "" {
		name = "From phone"
	}
	name, err := cleanName(name)
	if err != nil {
		return "", err
	}
	id := f.nextID()
	dir := s.slotDir(id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	target := filepath.Join(dir, "game.db")
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()
	if strings.EqualFold(filepath.Ext(from), ".zip") {
		if err := extractGameDB(from, target); err != nil {
			return "", err
		}
	} else if err := copyFile(from, target); err != nil {
		return "", err
	}
	if err := checkSave(target); err != nil {
		return "", err
	}
	f.Slots = append(f.Slots, slot{ID: id, Name: name, Created: time.Now().UnixMilli()})
	if err := s.save(f); err != nil {
		f.Slots = f.Slots[:len(f.Slots)-1]
		return "", err
	}
	ok = true
	// Bring it up to this server's tables now, so it lists and edits like the others.
	if err := s.upgrade(f, id, target); err != nil {
		return id, err
	}
	return id, nil
}

// extractGameDB takes game.db (at the root or in a folder) out of a backup ZIP.
func extractGameDB(zipPath, target string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("this is not a save backup: %w", err)
	}
	defer r.Close()
	for _, e := range r.File {
		if strings.EqualFold(filepath.Base(filepath.ToSlash(e.Name)), "game.db") && !e.FileInfo().IsDir() {
			src, err := e.Open()
			if err != nil {
				return err
			}
			defer src.Close()
			dst, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(dst, src); err != nil {
				dst.Close()
				return err
			}
			return dst.Close()
		}
	}
	return errors.New("This backup has no game.db")
}

func copyFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		return err
	}
	return dst.Close()
}

// checkSave makes sure a file is a game save: an SQLite database with a player.
func checkSave(p string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(p)+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	var n int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return errors.New("This file is not a NieR Re[in]carnation save")
	}
	if n == 0 {
		return errors.New("This save has no player yet")
	}
	return nil
}

func vacuumInto(from, to string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(from)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`VACUUM INTO ?`, to); err != nil {
		return fmt.Errorf("copy the save: %w", err)
	}
	return nil
}
