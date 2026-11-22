// Package pcsave is the PC port launcher's Settings › Saves: save slots
// (several game.db files, one of them in use) and the editor of what a
// player owns. It runs as `nier-server --admin` while the game is closed;
// the launcher sends one JSON request on stdin and reads one JSON reply.
//
// Layout in the save folder (auth.*, mods.json and the rest are shared by
// every slot, as they belong to this PC, not to a player):
//
//	game.db                  the slot in use (the server's save)
//	slots.json               {"active": id, "slots": [{id, name, created}]}
//	slots/<id>/game.db       the other slots (no game.db: a new game not started yet)
//	slot-backups/            a copy of a slot before each edit, switch and delete
package pcsave

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"lunar-tear/server/internal/database"
)

type slot struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Created int64  `json:"created"`
}

type slotFile struct {
	Active string `json:"active"`
	Slots  []slot `json:"slots"`
}

// slotInfo is a slot as the page lists it.
type slotInfo struct {
	slot
	Active   bool   `json:"active"`
	Started  bool   `json:"started"` // has a game.db
	Player   string `json:"player,omitempty"`
	Level    int64  `json:"level,omitempty"`
	Size     int64  `json:"size"`
	Modified int64  `json:"modified,omitempty"`
}

type store struct{ dir string }

func (s store) file() string             { return filepath.Join(s.dir, "slots.json") }
func (s store) slotDir(id string) string { return filepath.Join(s.dir, "slots", id) }
func (s store) backupDir() string        { return filepath.Join(s.dir, "slot-backups") }

// dbPath is where a slot's game.db is: the save folder's for the slot in use.
func (s store) dbPath(f *slotFile, id string) string {
	if id == f.Active {
		return filepath.Join(s.dir, "game.db")
	}
	return filepath.Join(s.slotDir(id), "game.db")
}

// load reads slots.json, creating it on first use with the current save as
// "Save 1".
func (s store) load() (*slotFile, error) {
	var f slotFile
	b, err := os.ReadFile(s.file())
	if err == nil {
		err = json.Unmarshal(b, &f)
	}
	if err == nil && f.Active != "" && f.find(f.Active) >= 0 {
		return &f, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("slots.json: %w", err)
	}
	f = slotFile{Active: "1", Slots: []slot{{ID: "1", Name: "Save 1", Created: time.Now().UnixMilli()}}}
	return &f, s.save(&f)
}

func (s store) save(f *slotFile) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.file() + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.file())
}

func (f *slotFile) find(id string) int {
	for i, x := range f.Slots {
		if x.ID == id {
			return i
		}
	}
	return -1
}

func (f *slotFile) nextID() string {
	n := 0
	for _, x := range f.Slots {
		if v, err := strconv.Atoi(x.ID); err == nil && v > n {
			n = v
		}
	}
	return strconv.Itoa(n + 1)
}

func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("give the save a name")
	}
	if r := []rune(name); len(r) > 40 {
		name = string(r[:40])
	}
	return name, nil
}

func (s store) list(f *slotFile) []slotInfo {
	out := make([]slotInfo, 0, len(f.Slots))
	for _, x := range f.Slots {
		info := slotInfo{slot: x, Active: x.ID == f.Active}
		p := s.dbPath(f, x.ID)
		if st, err := os.Stat(p); err == nil {
			info.Started = true
			info.Size = st.Size()
			info.Modified = st.ModTime().UnixMilli()
			if wal, err := os.Stat(p + "-wal"); err == nil && wal.ModTime().After(st.ModTime()) {
				info.Modified = wal.ModTime().UnixMilli()
			}
			var login int64
			info.Player, info.Level, login = peekPlayer(p)
			if login > 0 {
				info.Modified = login // the last sign-in: when it was last played
			}
		}
		out = append(out, info)
	}
	return out
}

// peekPlayer reads the player's name, level and last sign-in, read-only (so
// listing the saves changes none of them, not even their file times).
func peekPlayer(p string) (string, int64, int64) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(p)+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return "", 0, 0
	}
	defer db.Close()
	var name string
	var level, login int64
	uid, ok := playerID(db)
	if !ok {
		return "", 0, 0
	}
	db.QueryRow(`SELECT name FROM user_profile WHERE user_id = ?`, uid).Scan(&name)
	db.QueryRow(`SELECT level FROM user_status WHERE user_id = ?`, uid).Scan(&level)
	db.QueryRow(`SELECT last_login_datetime FROM user_login WHERE user_id = ?`, uid).Scan(&login)
	return name, level, login
}

// deviceID is the game's ID on this PC as a save knows it: the newest
// sign-in's, else the most recently started player's.
func deviceID(p string) string {
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	db, err := database.Open(p)
	if err != nil {
		return ""
	}
	defer db.Close()
	var id string
	if db.QueryRow(`SELECT uuid FROM sessions ORDER BY rowid DESC LIMIT 1`).Scan(&id) == nil && id != "" {
		return id
	}
	db.QueryRow(`SELECT uuid FROM users ORDER BY game_start_datetime DESC, user_id DESC LIMIT 1`).Scan(&id)
	return id
}

// adoptDevice makes a slot's player the one the game signs in as on this PC
// (as Bunker's Import save backup does): its most recently started player
// takes this PC's ID, unless one already has it. Old sign-ins are dropped;
// the game signs in again.
func adoptDevice(p, device string) error {
	if device == "" {
		return nil
	}
	if _, err := os.Stat(p); err != nil {
		return nil // a new game: the server makes the player on the first sign-in
	}
	db, err := database.Open(p)
	if err != nil {
		return err
	}
	defer func() {
		database.Checkpoint(db)
		db.Close()
	}()
	var mine int
	if err = db.QueryRow(`SELECT COUNT(*) FROM users WHERE uuid = ?`, device).Scan(&mine); err != nil || mine > 0 {
		return err
	}
	var uid int64
	if err = db.QueryRow(`SELECT user_id FROM users ORDER BY game_start_datetime DESC, user_id DESC LIMIT 1`).Scan(&uid); err != nil {
		return nil // no player yet
	}
	if _, err = db.Exec(`UPDATE users SET uuid = ? WHERE user_id = ?`, device, uid); err != nil {
		return fmt.Errorf("cannot give the save this PC's player ID: %w", err)
	}
	_, err = db.Exec(`DELETE FROM sessions`)
	return err
}

// settle writes a database's journal into it and closes it, so the file can
// be moved alone.
func settle(p string) error {
	if _, err := os.Stat(p); err != nil {
		return nil
	}
	db, err := database.Open(p)
	if err != nil {
		return err
	}
	if _, err = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		db.Close()
		return fmt.Errorf("cannot settle %s: %w", filepath.Base(p), err)
	}
	if err = db.Close(); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(p + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("%s%s is in use: close the game first", filepath.Base(p), suffix)
		}
	}
	return nil
}

// moveDB moves a settled game.db; a missing source is no error.
func moveDB(from, to string) error {
	if _, err := os.Stat(from); os.IsNotExist(err) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(to), 0700); err != nil {
		return err
	}
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("%s already exists", to)
	}
	return os.Rename(from, to)
}

// backup copies a slot's game.db to slot-backups (VACUUM INTO: a whole,
// consistent copy) and keeps the newest 30. Returns the copy's name, or ""
// for a slot that has no game.db.
func (s store) backup(f *slotFile, id, reason string) (string, error) {
	p := s.dbPath(f, id)
	if _, err := os.Stat(p); err != nil {
		return "", nil
	}
	if err := os.MkdirAll(s.backupDir(), 0700); err != nil {
		return "", err
	}
	name := fmt.Sprintf("%s-slot%s-%s.db", time.Now().Format("20060102-150405"), id, reason)
	out := filepath.Join(s.backupDir(), name)
	os.Remove(out)
	db, err := database.Open(p)
	if err != nil {
		return "", err
	}
	defer db.Close()
	if _, err := db.Exec(`VACUUM INTO ?`, out); err != nil {
		return "", fmt.Errorf("backup: %w", err)
	}
	if reason != "update" { // upgrade prunes once it knows the copy is kept
		s.prune(30)
	}
	return name, nil
}

func (s store) prune(keep int) {
	files, _ := filepath.Glob(filepath.Join(s.backupDir(), "*.db"))
	sort.Strings(files) // names start with the time
	for len(files) > keep {
		os.Remove(files[0])
		files = files[1:]
	}
}

// switchTo makes slot id the one in use.
func (s store) switchTo(f *slotFile, id string) error {
	if f.find(id) < 0 {
		return errors.New("no such save")
	}
	if id == f.Active {
		return nil
	}
	cur := filepath.Join(s.dir, "game.db")
	device := deviceID(cur)
	if err := settle(cur); err != nil {
		return err
	}
	target := filepath.Join(s.slotDir(id), "game.db")
	if err := settle(target); err != nil {
		return err
	}
	if _, err := s.backup(f, f.Active, "switch"); err != nil {
		return err
	}
	if err := moveDB(cur, filepath.Join(s.slotDir(f.Active), "game.db")); err != nil {
		return err
	}
	if err := moveDB(target, cur); err != nil {
		// put the slot in use back
		moveDB(filepath.Join(s.slotDir(f.Active), "game.db"), cur)
		return err
	}
	f.Active = id
	if err := s.save(f); err != nil {
		return err
	}
	if err := adoptDevice(cur, device); err != nil {
		return err
	}
	return settle(cur)
}
