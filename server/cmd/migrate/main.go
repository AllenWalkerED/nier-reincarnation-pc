// migrate applies the embedded schema migrations to a game database, the
// same step the wizard runs before starting the server.
package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"lunar-tear/server/migrations"
)

func main() {
	dbPath := flag.String("db", "db/game.db", "SQLite database path")
	flag.Parse()
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0755); err != nil {
		log.Fatalf("create db dir: %v", err)
	}
	db, err := sql.Open("sqlite", *dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if err := migrations.Up(context.Background(), db); err != nil {
		log.Fatalf("migration failed: %v", err)
	}
	log.Printf("migrations applied to %s", *dbPath)
}
