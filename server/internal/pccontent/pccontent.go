// Package pccontent holds the PC port's choice of which summons and events
// are open (the launcher's Events page). The launcher writes it to
// active-content.json in the save folder:
//
//	{"banners": ["limited_59", "limited_612", ...], "events": [509, 302, ...]}
//
// Nothing is open until the player chooses: a missing file, a missing list or
// an empty one closes every summon and event, except the game's permanent
// summons (AlwaysOpen), which no choice closes. A summon is chosen by its
// banner (its BannerAssetName, "gacha_<id>" when it has none), so its reruns,
// which share the banner, open with it. Events are event quest chapters; they
// are applied to the game data by tools\patch_content.py, which the launcher
// runs before each start.
//
// The server reads the file again when it changes (within a second), so a
// summon chosen while the game runs shows the next time Summons is opened.
// The choice is only in force once Use names the save folder (nier-server
// does it); without it every summon is open by its dates, as in Bunker.
package pccontent

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"lunar-tear/server/internal/model"
)

// FileName is the selection file in the save folder.
const FileName = "active-content.json"

// Selection is the player's choice; nil and empty lists both mean none.
type Selection struct {
	Banners []string `json:"banners"`
	Events  []int32  `json:"events"`
}

// AlwaysOpen reports the game's permanent summons, open whatever the choice
// (the launcher shows them as "Always on"): the chapter summons (label
// Chapter: chapter_1 to chapter_24 and Dark Memory, chapter_ex), the only
// summons the game data gives no dates, which still show only once their
// chapter is cleared; and the ticket summons (confirm_*: the ★4 and ★3
// guaranteed and element tickets), where the game's summon tickets are used.
func AlwaysOpen(label int32, bannerAssetName string) bool {
	return label == model.GachaLabelChapter || strings.HasPrefix(bannerAssetName, "confirm_")
}

// Key is the name a summon is chosen by: its banner, or "gacha_<id>".
func Key(bannerAssetName string, gachaId int32) string {
	if bannerAssetName != "" {
		return bannerAssetName
	}
	return "gacha_" + strconv.Itoa(int(gachaId))
}

// Read reads the selection in saveDir; a missing file is the default (none).
func Read(saveDir string) (Selection, error) {
	var s Selection
	b, err := os.ReadFile(filepath.Join(saveDir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Selection{}, fmt.Errorf("%s: %w", FileName, err)
	}
	return s, nil
}

// Write replaces the selection in saveDir.
func Write(saveDir string, s Selection) error {
	if s.Banners == nil {
		s.Banners = []string{}
	}
	if s.Events == nil {
		s.Events = []int32{}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(saveDir, FileName)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

var current struct {
	sync.Mutex
	path    string // "" : not in force, every summon open
	modTime time.Time
	size    int64
	checked time.Time
	banners map[string]bool
	test    bool // SetBanners: no file
}

// Recheck is how often BannerOpen looks at the file again (tests set 0).
var Recheck = time.Second

// Use puts the choice in saveDir in force for this run of the server.
func Use(saveDir string) {
	current.Lock()
	current.path = filepath.Join(saveDir, FileName)
	current.modTime, current.size, current.checked, current.banners, current.test = time.Time{}, -1, time.Time{}, nil, false
	current.Unlock()
	s, err := Read(saveDir)
	switch {
	case err != nil:
		log.Printf("[Content] %v: every summon and event stays closed", err)
	default:
		log.Printf("[Content] Settings › Events: %d summons and %d events chosen (none chosen: all closed)", len(s.Banners), len(s.Events))
	}
}

// Load is Use under its first name.
func Load(saveDir string) { Use(saveDir) }

// reload reads the file when it changed; the lock is held.
func reload() {
	now := time.Now()
	if current.banners != nil && now.Sub(current.checked) < Recheck {
		return
	}
	current.checked = now
	info, err := os.Stat(current.path)
	if err != nil {
		current.modTime, current.size, current.banners = time.Time{}, -1, map[string]bool{}
		return
	}
	if current.banners != nil && info.ModTime().Equal(current.modTime) && info.Size() == current.size {
		return
	}
	current.modTime, current.size, current.banners = info.ModTime(), info.Size(), map[string]bool{}
	s, err := Read(filepath.Dir(current.path))
	if err != nil {
		log.Printf("[Content] %v: every summon stays closed", err)
		return
	}
	for _, k := range s.Banners {
		current.banners[k] = true
	}
	log.Printf("[Content] summons chosen in Settings › Events: %d", len(current.banners))
}

// BannerOpen reports whether the player's choice leaves the summon with this
// key (Key) open. It is always true while no choice is in force.
func BannerOpen(key string) bool {
	current.Lock()
	defer current.Unlock()
	if current.test {
		return current.banners[key]
	}
	if current.path == "" {
		return true
	}
	reload()
	return current.banners[key]
}

// SetBanners sets the chosen summons directly, without a file (nil: no
// choice in force, everything open); for tests.
func SetBanners(keys []string) {
	current.Lock()
	defer current.Unlock()
	current.path, current.test, current.banners = "", keys != nil, nil
	if keys != nil {
		current.banners = map[string]bool{}
		for _, k := range keys {
			current.banners[k] = true
		}
	}
}
