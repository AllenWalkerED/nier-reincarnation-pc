package pcsave

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"lunar-tear/server/internal/database"
	"lunar-tear/server/internal/lunarbase"
	"lunar-tear/server/migrations"
)

type request struct {
	Op     string           `json:"op"`
	ID     string           `json:"id"`
	Name   string           `json:"name"`
	Switch bool             `json:"switch"`
	Path   string           `json:"path"` // export / import: the phone backup
	Edit   lunarbase.PCEdit `json:"edit"`
}

// Run answers one request: saveDir is the save folder, masterPath the game
// data the server loads, dataRoot the folder with assets/revisions.
func Run(saveDir, dataRoot, masterPath string, raw []byte) string {
	out, err := run(saveDir, dataRoot, masterPath, raw)
	if err != nil {
		b, _ := json.Marshal(map[string]any{"ok": false, "error": err.Error()})
		return string(b)
	}
	out["ok"] = true
	b, err := json.Marshal(out)
	if err != nil {
		b, _ = json.Marshal(map[string]any{"ok": false, "error": err.Error()})
	}
	return string(b)
}

// serverRunning reports whether the game server holds its port (the game is
// open): saves are changed only while it is closed.
func serverRunning() bool {
	l, err := net.Listen("tcp", "127.0.0.1:8003")
	if err != nil {
		return true
	}
	l.Close()
	return false
}

func run(saveDir, dataRoot, masterPath string, raw []byte) (map[string]any, error) {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("bad request: %w", err)
	}
	if isContentOp(req.Op) {
		// Settings › Events: a choice for the next start, fine while the game runs.
		return runContent(saveDir, dataRoot, masterPath, raw)
	}
	if err := os.MkdirAll(saveDir, 0700); err != nil {
		return nil, err
	}
	s := store{dir: saveDir}
	if req.Op != "list" && serverRunning() {
		return nil, errors.New("The game is running. Close it before changing saves.")
	}
	f, err := s.load()
	if err != nil {
		return nil, err
	}
	listing := func(extra map[string]any) map[string]any {
		if extra == nil {
			extra = map[string]any{}
		}
		extra["active"] = f.Active
		extra["slots"] = s.list(f)
		extra["running"] = req.Op == "list" && serverRunning()
		return extra
	}
	slotID := req.ID
	if slotID == "" {
		slotID = f.Active
	}
	switch req.Op {
	case "list":
		return listing(nil), nil

	case "new":
		name, err := cleanName(req.Name)
		if err != nil {
			return nil, err
		}
		id := f.nextID()
		if err := os.MkdirAll(s.slotDir(id), 0700); err != nil {
			return nil, err
		}
		f.Slots = append(f.Slots, slot{ID: id, Name: name, Created: time.Now().UnixMilli()})
		if err := s.save(f); err != nil {
			return nil, err
		}
		if req.Switch {
			if err := s.switchTo(f, id); err != nil {
				return nil, err
			}
		}
		return listing(map[string]any{"id": id}), nil

	case "switch":
		if err := s.switchTo(f, req.ID); err != nil {
			return nil, err
		}
		return listing(nil), nil

	case "rename":
		i := f.find(req.ID)
		if i < 0 {
			return nil, errors.New("no such save")
		}
		name, err := cleanName(req.Name)
		if err != nil {
			return nil, err
		}
		f.Slots[i].Name = name
		if err := s.save(f); err != nil {
			return nil, err
		}
		return listing(nil), nil

	case "delete":
		i := f.find(req.ID)
		if i < 0 {
			return nil, errors.New("no such save")
		}
		if req.ID == f.Active {
			return nil, errors.New("This save is in use. Switch to another save before deleting it.")
		}
		kept, err := s.backup(f, req.ID, "deleted")
		if err != nil {
			return nil, err
		}
		if err := os.RemoveAll(s.slotDir(req.ID)); err != nil {
			return nil, err
		}
		f.Slots = append(f.Slots[:i], f.Slots[i+1:]...)
		if err := s.save(f); err != nil {
			return nil, err
		}
		return listing(map[string]any{"backup": kept}), nil

	case "backup":
		if f.find(slotID) < 0 {
			return nil, errors.New("no such save")
		}
		kept, err := s.backup(f, slotID, "manual")
		if err != nil {
			return nil, err
		}
		if kept == "" {
			return nil, errors.New("This save has not been played yet: there is nothing to back up.")
		}
		return listing(map[string]any{"backup": kept}), nil

	case "export": // to the phone (Bunker's backup format)
		if f.find(slotID) < 0 {
			return nil, errors.New("no such save")
		}
		if slotID != f.Active {
			if _, err := s.started(f, slotID); err != nil { // up to date first
				return nil, err
			}
		}
		if err := s.exportPhone(f, slotID, req.Path); err != nil {
			return nil, err
		}
		return listing(map[string]any{"exported": req.Path}), nil

	case "import": // from the phone: a new slot
		id, err := s.importPhone(f, req.Path, req.Name)
		if err != nil && id == "" {
			return nil, err
		}
		if err == nil && req.Switch {
			err = s.switchTo(f, id)
		}
		if err != nil {
			return nil, err
		}
		return listing(map[string]any{"id": id}), nil

	case "read":
		p, err := s.started(f, slotID)
		if err != nil {
			return nil, err
		}
		player, err := lunarbase.PCRead(p, masterPath)
		if err != nil {
			return nil, err
		}
		cat, err := buildCatalog(dataRoot)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": slotID, "player": player, "catalog": cat}, nil

	case "apply":
		p, err := s.started(f, slotID)
		if err != nil {
			return nil, err
		}
		kept, err := s.backup(f, slotID, "edit")
		if err != nil {
			return nil, err
		}
		res, err := lunarbase.PCApply(p, masterPath, req.Edit)
		if err != nil {
			return nil, err
		}
		player, err := lunarbase.PCRead(p, masterPath)
		if err != nil {
			return nil, err
		}
		return map[string]any{"id": slotID, "result": res, "player": player, "backup": kept}, nil
	}
	return nil, fmt.Errorf("unknown request %q", req.Op)
}

// started is a slot's game.db, which must exist (the slot was played).
func (s store) started(f *slotFile, id string) (string, error) {
	if f.find(id) < 0 {
		return "", errors.New("no such save")
	}
	p := s.dbPath(f, id)
	if _, err := os.Stat(p); err != nil {
		return "", errors.New("This save has not been played yet. Start the game with it once, then edit it.")
	}
	if id != f.Active { // the server brings the save in use up to date itself
		if err := s.upgrade(f, id, p); err != nil {
			return "", err
		}
	}
	return filepath.Clean(p), nil
}

// upgrade brings a save made by an older server up to this one's tables (the
// server does it itself for the save in use when it starts). A copy is kept
// in slot-backups when anything changed.
func (s store) upgrade(f *slotFile, id, p string) error {
	db, err := database.Open(p)
	if err != nil {
		return err
	}
	defer db.Close()
	version := func() int64 {
		var v int64
		db.QueryRow(`SELECT COALESCE(MAX(version_id), 0) FROM goose_db_version`).Scan(&v)
		return v
	}
	before := version()
	copyName := ""
	if before > 0 {
		if copyName, err = s.backup(f, id, "update"); err != nil {
			return err
		}
	}
	if err := migrations.Up(context.Background(), db); err != nil {
		return fmt.Errorf("this save could not be brought up to date: %w", err)
	}
	if copyName != "" && version() == before {
		os.Remove(filepath.Join(s.backupDir(), copyName)) // nothing changed: no copy needed
	} else if copyName != "" {
		s.prune(30)
	}
	return nil
}

// playerID is the player the game signs in as in an open save.
func playerID(db *sql.DB) (int64, bool) {
	var uid int64
	err := db.QueryRow(`SELECT u.user_id FROM sessions s JOIN users u ON u.uuid = s.uuid ORDER BY s.rowid DESC LIMIT 1`).Scan(&uid)
	if err != nil {
		err = db.QueryRow(`SELECT user_id FROM users ORDER BY game_start_datetime DESC, user_id DESC LIMIT 1`).Scan(&uid)
	}
	return uid, err == nil
}
