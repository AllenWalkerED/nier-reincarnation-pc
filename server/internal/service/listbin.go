package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// listBinEntry holds path (')' as segment separator) and size from list.bin; Size is 0 when not present.
type listBinEntry struct {
	Path string
	Size int64
	MD5  string
}

// listBinIndex finds a revision's list.bin entries by object_id. list.bin has about
// 200,000 entries, and a map of their paths and md5s took ~36 MB in a server that
// shares the phone with the game; the index keeps only where each entry is in the
// file (16 bytes) and reads the entry back when asked.
type listBinIndex struct {
	file    *os.File
	records []listBinRecord          // ids of up to 8 bytes, sorted
	long    map[string]listBinRecord // longer ids, or ones with a zero byte (none in the 3.7.1 dump)
}

type listBinRecord struct {
	id  [8]byte
	off uint32
	n   uint32
}

func shortListBinID(id string) (key [8]byte, ok bool) {
	if len(id) > len(key) || strings.IndexByte(id, 0) >= 0 {
		return key, false
	}
	copy(key[:], id)
	return key, true
}

func (x *listBinIndex) lookup(objectId string) (listBinEntry, bool) {
	var r listBinRecord
	if key, short := shortListBinID(objectId); short {
		i := sort.Search(len(x.records), func(i int) bool { return bytes.Compare(x.records[i].id[:], key[:]) >= 0 })
		if i == len(x.records) || x.records[i].id != key {
			return listBinEntry{}, false
		}
		r = x.records[i]
	} else if r = x.long[objectId]; r.n == 0 {
		return listBinEntry{}, false
	}
	data := make([]byte, r.n)
	if _, err := x.file.ReadAt(data, int64(r.off)); err != nil {
		return listBinEntry{}, false
	}
	id, entry, ok := parseListBinEntry(data)
	if !ok || id != objectId {
		return listBinEntry{}, false
	}
	return entry, true
}

func (x *listBinIndex) len() int { return len(x.records) + len(x.long) }

type infoAlias struct {
	ToName     string
	ToRevision string
	MD5        string
}

type assetCandidate struct {
	Path        string
	Revision    string
	Source      string
	ExpectedMD5 string
}

type listBinLoad struct {
	done chan struct{}
	idx  *listBinIndex
	ok   bool
}

type infoLoad struct {
	done chan struct{}
	m    map[string]infoAlias
}

var (
	listBinCache    = make(map[string]*listBinIndex) // revision/platform → index
	listBinInflight = make(map[string]*listBinLoad)
	listBinCacheMu  sync.RWMutex
	infoCache       = make(map[string]map[string]infoAlias) // revision/platform → from-name → duplicate target
	infoInflight    = make(map[string]*infoLoad)
	infoCacheMu     sync.RWMutex
)

func cacheKey(revision, platform string) string {
	if platform == "" {
		return revision + "/_shared"
	}
	return revision + "/" + platform
}

// infoJSONEntry is one entry from assets/revisions/{rev}/info.json (duplicate files: serve to-name when asked for from-name).
type infoJSONEntry struct {
	FromName   string `json:"from-name"`
	ToName     string `json:"to-name"`
	ToRevision *int   `json:"to-revision"`
	MD5        string `json:"md5"`
}

// readVarint reads a protobuf varint from b, returns value and number of bytes consumed.
func readVarint(b []byte) (value int, n int) {
	for i := 0; i < len(b) && i < 10; i++ {
		value |= int(b[i]&0x7f) << (7 * i)
		n = i + 1
		if b[i]&0x80 == 0 {
			return value, n
		}
	}
	return 0, 0
}

func skipProtoField(wireType int, data []byte, offset int) (int, bool) {
	switch wireType {
	case 0:
		_, n := readVarint(data[offset:])
		if n == 0 {
			return 0, false
		}
		return offset + n, true
	case 1:
		if offset+8 > len(data) {
			return 0, false
		}
		return offset + 8, true
	case 2:
		length, n := readVarint(data[offset:])
		if n == 0 || length < 0 || offset+n+length > len(data) {
			return 0, false
		}
		return offset + n + length, true
	case 5:
		if offset+4 > len(data) {
			return 0, false
		}
		return offset + 4, true
	default:
		return 0, false
	}
}

func parseListBinEntry(data []byte) (objectId string, entry listBinEntry, ok bool) {
	i := 0
	for i < len(data) {
		tag, n := readVarint(data[i:])
		if n == 0 {
			return "", listBinEntry{}, false
		}
		i += n
		fieldNum := tag >> 3
		wireType := tag & 0x7

		switch fieldNum {
		case 3: // path
			if wireType != 2 {
				return "", listBinEntry{}, false
			}
			length, vn := readVarint(data[i:])
			if vn == 0 || length < 0 || i+vn+length > len(data) {
				return "", listBinEntry{}, false
			}
			entry.Path = string(data[i+vn : i+vn+length])
			i += vn + length
		case 4: // size
			if wireType != 0 {
				return "", listBinEntry{}, false
			}
			size, vn := readVarint(data[i:])
			if vn == 0 {
				return "", listBinEntry{}, false
			}
			if size >= 256 {
				entry.Size = int64(size)
			}
			i += vn
		case 10: // md5
			if wireType != 2 {
				return "", listBinEntry{}, false
			}
			length, vn := readVarint(data[i:])
			if vn == 0 || length < 0 || i+vn+length > len(data) {
				return "", listBinEntry{}, false
			}
			entry.MD5 = string(data[i+vn : i+vn+length])
			i += vn + length
		case 11: // object_id
			if wireType != 2 {
				return "", listBinEntry{}, false
			}
			length, vn := readVarint(data[i:])
			if vn == 0 || length <= 0 || i+vn+length > len(data) {
				return "", listBinEntry{}, false
			}
			objectId = string(data[i+vn : i+vn+length])
			i += vn + length
		default:
			next, ok := skipProtoField(wireType, data, i)
			if !ok {
				return "", listBinEntry{}, false
			}
			i = next
		}
	}

	if objectId == "" || entry.Path == "" {
		return "", listBinEntry{}, false
	}
	return objectId, entry, true
}

// indexListBin indexes list.bin by object_id (6-byte string) → where the entry is.
// The file is a protobuf message with repeated nested entry messages, so we parse each entry
// boundary first instead of doing a flat scan across the whole file. It reads the file as a
// stream, and a later entry with the same object_id replaces an earlier one. It stops at
// the first damaged field, keeping the entries before it.
func indexListBin(f *os.File) (*listBinIndex, error) {
	idx := &listBinIndex{file: f}
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > math.MaxUint32 {
		return nil, fmt.Errorf("list.bin is %d bytes", info.Size())
	}
	in := bufio.NewReaderSize(io.NewSectionReader(f, 0, info.Size()), 64<<10)
	var off int64 // in's position in the file
	readVarint := func() (v uint64, ok bool) {
		for i := 0; i < 10; i++ {
			b, err := in.ReadByte()
			if err != nil {
				return 0, false
			}
			off++
			v |= uint64(b&0x7f) << (7 * i)
			if b&0x80 == 0 {
				return v, true
			}
		}
		return 0, false
	}
	skip := func(n int64) bool {
		if n < 0 || off+n > info.Size() {
			return false
		}
		d, err := in.Discard(int(n))
		off += int64(d)
		return err == nil
	}
	var entry []byte
	for {
		tag, ok := readVarint()
		if !ok {
			break
		}
		switch tag & 7 {
		case 0:
			ok = func() bool { _, ok := readVarint(); return ok }()
		case 1:
			ok = skip(8)
		case 5:
			ok = skip(4)
		case 2:
			var length uint64
			if length, ok = readVarint(); !ok || length > uint64(info.Size()-off) {
				ok = false
				break
			}
			start := off
			if cap(entry) < int(length) {
				entry = make([]byte, length)
			}
			entry = entry[:length]
			if _, err := io.ReadFull(in, entry); err != nil {
				ok = false
				break
			}
			off += int64(length)
			if objectId, _, valid := parseListBinEntry(entry); valid {
				r := listBinRecord{off: uint32(start), n: uint32(length)}
				if key, short := shortListBinID(objectId); short {
					r.id = key
					idx.records = append(idx.records, r)
				} else {
					if idx.long == nil {
						idx.long = map[string]listBinRecord{}
					}
					idx.long[objectId] = r
				}
			}
		default:
			ok = false
		}
		if !ok {
			break
		}
	}
	// Sort by id, keeping each id's last entry (its records stay in file order).
	sort.SliceStable(idx.records, func(i, j int) bool {
		return bytes.Compare(idx.records[i].id[:], idx.records[j].id[:]) < 0
	})
	kept := idx.records[:0]
	for i, r := range idx.records {
		if i+1 < len(idx.records) && idx.records[i+1].id == r.id {
			continue
		}
		kept = append(kept, r)
	}
	idx.records = slices.Clip(kept)
	return idx, nil
}

func loadListBinIndex(baseDir, revision, platform string) (*listBinIndex, bool) {
	key := cacheKey(revision, platform)
	listBinCacheMu.RLock()
	idx, ok := listBinCache[key]
	listBinCacheMu.RUnlock()
	if ok {
		return idx, true
	}

	listBinCacheMu.Lock()
	if idx, ok := listBinCache[key]; ok {
		listBinCacheMu.Unlock()
		return idx, true
	}
	if load := listBinInflight[key]; load != nil {
		listBinCacheMu.Unlock()
		<-load.done
		return load.idx, load.ok
	}
	load := &listBinLoad{done: make(chan struct{})}
	listBinInflight[key] = load
	listBinCacheMu.Unlock()

	filePath := filepath.Join(baseDir, "assets", "revisions", revision, platform, "list.bin")
	f, err := os.Open(filePath)
	if err == nil {
		if idx, err = indexListBin(f); err != nil {
			f.Close()
		}
	}
	if err != nil {
		listBinCacheMu.Lock()
		delete(listBinInflight, key)
		close(load.done)
		listBinCacheMu.Unlock()
		return nil, false
	}
	load.idx = idx
	load.ok = true
	listBinCacheMu.Lock()
	listBinCache[key] = idx
	delete(listBinInflight, key)
	close(load.done)
	listBinCacheMu.Unlock()
	return idx, true
}

func loadInfoIndex(baseDir, revision, platform string) map[string]infoAlias {
	key := cacheKey(revision, platform)
	infoCacheMu.RLock()
	m, ok := infoCache[key]
	infoCacheMu.RUnlock()
	if ok {
		return m
	}

	infoCacheMu.Lock()
	if m, ok := infoCache[key]; ok {
		infoCacheMu.Unlock()
		return m
	}
	if load := infoInflight[key]; load != nil {
		infoCacheMu.Unlock()
		<-load.done
		return load.m
	}
	load := &infoLoad{done: make(chan struct{})}
	infoInflight[key] = load
	infoCacheMu.Unlock()

	filePath := filepath.Join(baseDir, "assets", "revisions", revision, platform, "info.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		infoCacheMu.Lock()
		infoCache[key] = nil
		delete(infoInflight, key)
		close(load.done)
		infoCacheMu.Unlock()
		return nil
	}
	var entries []infoJSONEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		infoCacheMu.Lock()
		infoCache[key] = nil
		delete(infoInflight, key)
		close(load.done)
		infoCacheMu.Unlock()
		return nil
	}
	m = make(map[string]infoAlias)
	for _, e := range entries {
		if e.FromName != "" && e.ToName != "" {
			aliasRevision := revision
			if e.ToRevision != nil {
				aliasRevision = strconv.Itoa(*e.ToRevision)
			}
			m[e.FromName] = infoAlias{
				ToName:     e.ToName,
				ToRevision: aliasRevision,
				MD5:        e.MD5,
			}
		}
	}
	load.m = m
	infoCacheMu.Lock()
	infoCache[key] = m
	delete(infoInflight, key)
	close(load.done)
	infoCacheMu.Unlock()
	return m
}

type pathCandidate struct {
	Path             string
	IsLocaleFallback bool
}

// utf8ToMojibake re-encodes non-ASCII runes as if each of their UTF-8 bytes
// were a Latin-1 codepoint. This matches filenames extracted by tools that
// misinterpret UTF-8 paths as Latin-1 (double-encoding). For example,
// U+FF12 (fullwidth ２, bytes EF BC 92) becomes U+00EF U+00BC U+0092
// (bytes C3 AF C2 BC C2 92).
func utf8ToMojibake(s string) string {
	var b strings.Builder
	changed := false
	for _, r := range s {
		if r >= 0x80 {
			var buf [4]byte
			n := utf8.EncodeRune(buf[:], r)
			for i := 0; i < n; i++ {
				b.WriteRune(rune(buf[i]))
			}
			changed = true
		} else {
			b.WriteRune(r)
		}
	}
	if !changed {
		return s
	}
	return b.String()
}

// normalizeFullwidth replaces fullwidth Unicode characters (U+FF01–U+FF5E)
// with their ASCII equivalents (U+0021–U+007E).
func normalizeFullwidth(s string) string {
	var b strings.Builder
	changed := false
	for _, r := range s {
		if r >= 0xFF01 && r <= 0xFF5E {
			b.WriteByte(byte(r - 0xFF01 + 0x21))
			changed = true
		} else {
			b.WriteRune(r)
		}
	}
	if !changed {
		return s
	}
	return b.String()
}

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r >= 0x80 {
			return true
		}
	}
	return false
}

// pathStrToFullPaths converts a list.bin path string (using ')' separators) into filesystem
// candidates. The original locale path is returned first; if the path contains ja or ko,
// an en locale fallback is appended (marked IsLocaleFallback so callers can skip MD5 validation).
// For paths with non-ASCII characters, mojibake (double-encoded) and fullwidth-to-ASCII
// variants are also tried.
func pathStrToFullPaths(baseDir, revision, platform, assetType, pathStr string) []pathCandidate {
	fsPath := strings.ReplaceAll(pathStr, ")", "/")
	if strings.Contains(fsPath, "..") || filepath.IsAbs(fsPath) || strings.HasPrefix(fsPath, "/") {
		return nil
	}
	fsPath = filepath.Clean(fsPath)
	if strings.Contains(fsPath, "..") {
		return nil
	}
	type tagged struct {
		pathStr  string
		fallback bool
	}
	entries := []tagged{{pathStr, false}}
	if hasNonASCII(pathStr) {
		entries = append(entries, tagged{utf8ToMojibake(pathStr), false})
		entries = append(entries, tagged{normalizeFullwidth(pathStr), false})
	}
	if strings.Contains(pathStr, ")ja)") {
		entries = append(entries, tagged{strings.ReplaceAll(pathStr, ")ja)", ")en)"), true})
	}
	if strings.Contains(pathStr, ")ko)") {
		entries = append(entries, tagged{strings.ReplaceAll(pathStr, ")ko)", ")en)"), true})
	}
	base := filepath.Join(baseDir, "assets", "revisions", revision, platform)
	var out []pathCandidate
	seen := make(map[string]bool)
	for _, e := range entries {
		cleaned := filepath.Clean(strings.ReplaceAll(e.pathStr, ")", "/"))
		if seen[cleaned] {
			continue
		}
		seen[cleaned] = true
		var fullPath string
		switch assetType {
		case "assetbundle":
			fullPath = filepath.Join(base, "assetbundle", cleaned+".assetbundle")
		case "resources":
			fullPath = filepath.Join(base, "resources", cleaned)
		default:
			return nil
		}
		out = append(out, pathCandidate{Path: fullPath, IsLocaleFallback: e.fallback})
	}
	return out
}

func appendUniqueCandidate(candidates []assetCandidate, seen map[string]bool, candidate assetCandidate) []assetCandidate {
	key := candidate.Revision + ":" + candidate.Path
	if seen[key] {
		return candidates
	}
	seen[key] = true
	return append(candidates, candidate)
}

func duplicateCandidatePath(baseDir string, candidate assetCandidate, platform, assetType, targetRevision, targetBaseName string) string {
	root := filepath.Join(baseDir, "assets", "revisions", candidate.Revision, platform, assetType)
	rel, err := filepath.Rel(root, candidate.Path)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return ""
	}
	return filepath.Join(baseDir, "assets", "revisions", targetRevision, platform, assetType, filepath.Dir(rel), targetBaseName)
}

// objectIdToFilePathCandidates returns candidate file paths for the object: list.bin path, locale fallbacks
// (ja/ko -> en), then info.json duplicate mappings (from-name -> to-name, possibly in a different revision).
// The original locale path is tried first (with MD5 validation); locale fallbacks are tried after
// (without MD5 validation, since the hash in list.bin refers to the original locale's content).
//
// Two tiers are searched in order: the requested platform tree (e.g. revisions/0/ios/...) and then
// the un-split shared tree (revisions/0/...) which acts as a fallback for operators who deploy a
// single unified asset dump. Each tier carries its own list.bin md5 so corruption is still detected.
// Callers should try each path until one exists on disk.
func objectIdToFilePathCandidates(baseDir, revision, platform, assetType, objectId string) (candidates []assetCandidate, size int64, ok bool) {
	seen := make(map[string]bool)
	var firstSize int64
	var anyHit bool

	appendForPlatform := func(p, label string) {
		idx, idxOk := loadListBinIndex(baseDir, revision, p)
		if !idxOk || idx == nil {
			return
		}
		entry, entryOk := idx.lookup(objectId)
		if !entryOk || entry.Path == "" {
			return
		}
		paths := pathStrToFullPaths(baseDir, revision, p, assetType, entry.Path)
		if len(paths) == 0 {
			return
		}
		tierStart := len(candidates)
		for _, pc := range paths {
			md5 := entry.MD5
			if pc.IsLocaleFallback {
				md5 = ""
			}
			candidates = appendUniqueCandidate(candidates, seen, assetCandidate{
				Path:        pc.Path,
				Revision:    revision,
				Source:      "list.bin (" + label + ")",
				ExpectedMD5: md5,
			})
		}
		infoIndex := loadInfoIndex(baseDir, revision, p)
		if len(infoIndex) > 0 {
			tierCandidates := candidates[tierStart:]
			for _, c := range tierCandidates {
				alias, aliasOk := infoIndex[filepath.Base(c.Path)]
				if !aliasOk || alias.ToName == "" {
					continue
				}
				alt := duplicateCandidatePath(baseDir, c, p, assetType, alias.ToRevision, alias.ToName)
				if alt == "" {
					continue
				}
				candidates = appendUniqueCandidate(candidates, seen, assetCandidate{
					Path:        alt,
					Revision:    alias.ToRevision,
					Source:      "info.json redirect (" + label + ")",
					ExpectedMD5: alias.MD5,
				})
			}
		}
		if !anyHit {
			firstSize = entry.Size
			anyHit = true
		}
	}

	appendForPlatform(platform, platform)
	appendForPlatform("", "shared")

	if !anyHit {
		return nil, 0, false
	}
	return candidates, firstSize, true
}
