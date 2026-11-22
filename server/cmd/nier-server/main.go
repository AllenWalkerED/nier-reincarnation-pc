// nier-server runs Bunker's local server on Windows for the NieR
// Re[in]carnation PC port: the game (gRPC, 127.0.0.1:8003), the assets
// (Octo CDN, 8080) and the accounts (auth, 3000) in one process, set up as the
// Android app sets them up (mobile.Start): Bunker's summon catalog, mods
// (mods.json beside the save), calendar and fixes.
//
//	nier-server --save <folder with game.db> --data <folder with assets\release and assets\revisions>
//
// The save folder's auth.secret (the hex key the launcher keeps) is copied to
// auth.key, the name mobile.Start reads, so logins made before stay valid.
// Before a new version updates the save, a copy goes to <save>\backups.
// The process runs until it is ended (the launcher's job object) or gets
// Ctrl+C, and then flushes the save.
//
//	nier-server --save <folder> --data <folder> --admin < request.json
//
// answers one request of the launcher's Settings › Saves or Settings › Events (save slots, the
// editor, the open summons and events, pictures: internal/pcsave) on stdout and exits.
// It refuses changes while the game server is running.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"lunar-tear/server/internal/pccontent"
	"lunar-tear/server/internal/pcsave"
	"lunar-tear/server/mobile"
)

func main() {
	save := flag.String("save", "save", "the save folder (game.db, auth.db, mods.json)")
	data := flag.String("data", "data", "the game data folder (assets\\release, assets\\revisions)")
	admin := flag.Bool("admin", false, "answer one Settings › Saves request (JSON on stdin) and exit")
	flag.Parse()

	if *admin {
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("admin: %v", err)
		}
		fmt.Println(pcsave.Run(*save, *data, filepath.Join(*data, "assets", "release", mobile.MasterName), raw))
		return
	}

	if err := os.MkdirAll(*save, 0700); err != nil {
		log.Fatalf("save folder: %v", err)
	}
	key := filepath.Join(*save, "auth.key")
	if _, err := os.Stat(key); os.IsNotExist(err) {
		if secret, err := os.ReadFile(filepath.Join(*save, "auth.secret")); err == nil {
			if err := os.WriteFile(key, secret, 0600); err != nil {
				log.Fatalf("auth key: %v", err)
			}
		}
	}

	// A save a new server version updates is copied here first ("Before
	// update" snapshots, the last three kept).
	mobile.SetBackupDir(filepath.Join(*save, "backups"))
	// The launcher's Settings › Events (active-content.json): only the summons
	// the player chose are open; none until they choose.
	pccontent.Use(*save)
	if err := mobile.Start(*save, *data); err != nil {
		log.Fatalf("start: %v", err)
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	mobile.Stop()
}
