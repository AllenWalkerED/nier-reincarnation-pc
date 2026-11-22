package pcsave

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"lunar-tear/server/internal/database"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store/sqlite"
	"lunar-tear/server/migrations"
)

func makeSave(t *testing.T, p, uuid string) {
	t.Helper()
	db, err := database.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := migrations.Up(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlite.New(db, nil).CreateUser(uuid, model.DefaultPlatform); err != nil {
		t.Fatal(err)
	}
	database.Checkpoint(db)
}

func call(t *testing.T, dir, req string) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal([]byte(Run(dir, "", "", []byte(req))), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func uuidOf(t *testing.T, p string) string {
	t.Helper()
	db, err := database.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var id string
	db.QueryRow(`SELECT uuid FROM users LIMIT 1`).Scan(&id)
	return id
}

func TestSlots(t *testing.T) {
	if serverRunning() {
		t.Skip("a server holds 127.0.0.1:8003")
	}
	dir := t.TempDir()
	makeSave(t, filepath.Join(dir, "game.db"), "device")

	r := call(t, dir, `{"op":"list"}`)
	if r["ok"] != true || r["active"] != "1" || len(r["slots"].([]any)) != 1 {
		t.Fatalf("list: %v", r)
	}
	// Start over: a new slot, in use, with no game.db yet; the old one kept.
	r = call(t, dir, `{"op":"new","name":"Fresh","switch":true}`)
	if r["ok"] != true || r["active"] != "2" {
		t.Fatalf("new: %v", r)
	}
	if _, err := os.Stat(filepath.Join(dir, "game.db")); !os.IsNotExist(err) {
		t.Fatal("a new game must have no game.db")
	}
	if _, err := os.Stat(filepath.Join(dir, "slots", "1", "game.db")); err != nil {
		t.Fatal("the old save must be kept")
	}
	if r = call(t, dir, `{"op":"delete","id":"2"}`); r["ok"] != false {
		t.Fatal("the save in use must not be deleted")
	}
	// A slot made on another install takes this PC's player ID when used.
	makeSave(t, filepath.Join(dir, "slots", "2", "game.db"), "elsewhere")
	if r = call(t, dir, `{"op":"switch","id":"1"}`); r["ok"] != true {
		t.Fatalf("switch: %v", r)
	}
	if got := uuidOf(t, filepath.Join(dir, "game.db")); got != "device" {
		t.Fatalf("slot 1 uuid %q", got)
	}
	if r = call(t, dir, `{"op":"switch","id":"2"}`); r["ok"] != true {
		t.Fatalf("switch: %v", r)
	}
	if got := uuidOf(t, filepath.Join(dir, "game.db")); got != "device" {
		t.Fatalf("slot 2 must take this PC's ID, has %q", got)
	}
	if r = call(t, dir, `{"op":"switch","id":"1"}`); r["ok"] != true {
		t.Fatalf("switch back: %v", r)
	}
	if r = call(t, dir, `{"op":"rename","id":"2","name":"  Other  "}`); r["ok"] != true {
		t.Fatalf("rename: %v", r)
	}
	r = call(t, dir, `{"op":"delete","id":"2"}`)
	if r["ok"] != true || r["backup"] == "" || len(r["slots"].([]any)) != 1 {
		t.Fatalf("delete: %v", r)
	}
	if kept, _ := filepath.Glob(filepath.Join(dir, "slot-backups", "*-slot2-deleted.db")); len(kept) != 1 {
		t.Fatalf("a deleted save must leave a copy: %v", kept)
	}
	if _, err := os.Stat(filepath.Join(dir, "slots", "2")); !os.IsNotExist(err) {
		t.Fatal("the deleted slot's folder must be gone")
	}
}
