package pcsave

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"lunar-tear/server/internal/translation"
)

// texts is the game's English text ("costume.name.ch001001" -> "...") read
// from the text bundles the game itself loads (revisions/<n>/assetbundle/text/en).
type texts map[string]string

// loadTexts reads the named bundles (paths under text/en without the
// extension, such as "possession/costume") and, for each, the bundles in the
// folder of the same name. Unreadable bundles are skipped: a name then falls
// back to its id.
func loadTexts(dataRoot string, bundles ...string) texts {
	out := texts{}
	root := textRoot(dataRoot)
	if root == "" {
		return out
	}
	for _, b := range bundles {
		files := []string{filepath.Join(root, filepath.FromSlash(b)+".assetbundle")}
		more, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(b), "*.assetbundle"))
		sort.Strings(more)
		files = append(files, more...)
		for _, f := range files {
			readBundle(root, f, out)
		}
	}
	return out
}

// textRoot is the newest revision's English text folder.
func textRoot(dataRoot string) string {
	revs, _ := filepath.Glob(filepath.Join(dataRoot, "assets", "revisions", "*"))
	best, bestN := "", -1
	for _, r := range revs {
		n, err := strconv.Atoi(filepath.Base(r))
		if err != nil {
			continue
		}
		p := filepath.Join(r, "assetbundle", "text", "en")
		if st, err := os.Stat(p); err == nil && st.IsDir() && n > bestN {
			best, bestN = p, n
		}
	}
	return best
}

func readBundle(root, file string, out texts) {
	raw, err := os.ReadFile(file)
	if err != nil || len(raw) == 0 {
		return
	}
	rel, err := filepath.Rel(root, strings.TrimSuffix(file, ".assetbundle"))
	if err != nil {
		return
	}
	name := "text)en)" + strings.ReplaceAll(filepath.ToSlash(rel), "/", ")")
	// RebuildText hands every TextAsset's text to the callback; nothing is
	// changed, so nothing is written.
	_, _, _, _ = translation.RebuildText(raw, name, func(text string) (string, bool) {
		for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "//") {
				continue
			}
			i := strings.Index(line, ":")
			if i <= 0 {
				continue
			}
			if _, seen := out[line[:i]]; !seen {
				out[line[:i]] = line[i+1:]
			}
		}
		return text, false
	})
}
