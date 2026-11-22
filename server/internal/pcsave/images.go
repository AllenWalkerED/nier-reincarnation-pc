package pcsave

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"

	"lunar-tear/server/mobile/unityasset"
)

// Pictures for the launcher's lists (Settings › Events, the save editor),
// made on the player's PC from the game's own asset bundles: nothing of the
// game ships with the port. They are cached as PNG files in the launcher's
// page folder, launcher\ui\game\icons\<kind>\<name>.png (served to the page as
// game/icons/...), beside the title art setup takes from the APK.
//
//	{"op":"images","keys":["banner/limited_59","costume/ch001001",...]}
//	-> {"have":[keys with a picture],"made":n,"missing":[keys without]}
//
// A key is "<kind>/<name>"; imageKinds maps the kind to the bundle that holds
// the picture. Pictures made before are kept and not made again.

type imageKind struct {
	path    func(name string) string // the bundle, under the newest revision's assetbundle folder
	maxSide int                      // shrink larger pictures to this (0: as they are)
}

var imageKinds = map[string]imageKind{
	// A summon's banner in the Summons list (234×118).
	"banner": {path: func(n string) string { return filepath.Join("ui", "gacha", "en", n, "banner.assetbundle") }},
	// An event's banner in the Events list (402×490) or its list banner (218×110).
	"event":     {path: func(n string) string { return filepath.Join("ui", "quest", "en", "banner", n+".assetbundle") }, maxSide: 240},
	"eventlist": {path: func(n string) string { return filepath.Join("ui", "quest", "en", "list_banner", n+".assetbundle") }},
	"eventmom":  {path: func(n string) string { return filepath.Join("ui", "quest", "en", "mom_banner", n+".assetbundle") }, maxSide: 240},
	// Icons (102×102).
	"costume":    {path: standardIcon("costume")},
	"weapon":     {path: standardIcon("weapon")},
	"companion":  {path: standardIcon("companion")},
	"material":   {path: standardIcon("material")},
	"consumable": {path: standardIcon("consumable_item")},
}

func standardIcon(folder string) func(string) string {
	return func(n string) string { return filepath.Join("ui", folder, n, n+"_standard.assetbundle") }
}

var imageKeyPattern = regexp.MustCompile(`^([a-z]+)/([a-z0-9_]{1,64})$`)

// bundleRoot is the newest revision's assetbundle folder ("" if none).
func bundleRoot(dataRoot string) string {
	revs, _ := filepath.Glob(filepath.Join(dataRoot, "assets", "revisions", "*"))
	best, bestN := "", -1
	for _, r := range revs {
		n, err := strconv.Atoi(filepath.Base(r))
		if err != nil {
			continue
		}
		p := filepath.Join(r, "assetbundle")
		if st, err := os.Stat(p); err == nil && st.IsDir() && n > bestN {
			best, bestN = p, n
		}
	}
	return best
}

// bundles finds the bundles behind image keys.
type bundles struct{ root string }

func newBundles(dataRoot string) bundles { return bundles{root: bundleRoot(dataRoot)} }

// path is the bundle of a key ("" for a bad key).
func (b bundles) path(key string) string {
	m := imageKeyPattern.FindStringSubmatch(key)
	if m == nil || b.root == "" {
		return ""
	}
	k, ok := imageKinds[m[1]]
	if !ok {
		return ""
	}
	return filepath.Join(b.root, k.path(m[2]))
}

// key is kind/name when that bundle is in the game files, else "".
func (b bundles) key(kind, name string) string {
	key := kind + "/" + name
	p := b.path(key)
	if p == "" {
		return ""
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return key
}

// iconDir is the launcher's picture cache beside the data folder.
func iconDir(dataRoot string) string {
	return filepath.Join(filepath.Dir(filepath.Clean(dataRoot)), "launcher", "ui", "game", "icons")
}

func runImages(dataRoot string, raw []byte) (map[string]any, error) {
	var req struct {
		Keys []string `json:"keys"`
	}
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("bad request: %w", err)
	}
	b := newBundles(dataRoot)
	if b.root == "" {
		return nil, errors.New("The game files were not found (data\\assets\\revisions).")
	}
	out := iconDir(dataRoot)
	have := make([]string, 0, len(req.Keys))
	missing := []string{}
	var todo []string
	seen := map[string]bool{}
	for _, key := range req.Keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		if b.path(key) == "" {
			missing = append(missing, key)
			continue
		}
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(key)+".png")); err == nil {
			have = append(have, key)
			continue
		}
		todo = append(todo, key)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	jobs := make(chan string)
	made := 0
	var firstErr error
	for w := 0; w < max(1, min(runtime.NumCPU(), 8)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for key := range jobs {
				err := makeImage(b, key, filepath.Join(out, filepath.FromSlash(key)+".png"))
				mu.Lock()
				if err != nil {
					missing = append(missing, key)
					if firstErr == nil && !errors.Is(err, os.ErrNotExist) {
						firstErr = fmt.Errorf("%s: %w", key, err)
					}
				} else {
					have = append(have, key)
					made++
				}
				mu.Unlock()
			}
		}()
	}
	for _, key := range todo {
		jobs <- key
	}
	close(jobs)
	wg.Wait()
	res := map[string]any{"have": have, "made": made, "missing": missing}
	if firstErr != nil {
		res["warning"] = firstErr.Error()
	}
	return res, nil
}

// makeImage writes the bundle's largest texture as a PNG.
func makeImage(b bundles, key, target string) error {
	m := imageKeyPattern.FindStringSubmatch(key)
	kind := imageKinds[m[1]]
	bundle, err := unityasset.OpenBundle(b.path(key))
	if err != nil {
		return err
	}
	textures, err := bundle.Textures()
	if err != nil {
		return err
	}
	if len(textures) == 0 {
		return errors.New("no picture in this bundle")
	}
	best := textures[0]
	for _, t := range textures[1:] {
		if t.Width*t.Height > best.Width*best.Height {
			best = t
		}
	}
	if kind.maxSide > 0 {
		best = best.Shrink(kind.maxSide)
	}
	data, err := best.PNG()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}
