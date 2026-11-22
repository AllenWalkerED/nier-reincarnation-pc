package pcsave

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"lunar-tear/server/internal/gametime"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/pccontent"
	"lunar-tear/server/internal/runtime"
	"lunar-tear/server/internal/service"
	"lunar-tear/server/internal/utils"
)

// Settings › Events: the summons and events the player can choose from, by
// their in-game names and pictures, and the choice (pccontent,
// active-content.json). Nothing is open until the player chooses.
//
//	{"op":"content-list"}
//	{"op":"content-set","banners":["limited_59",...],"events":[509,...]}
//	{"op":"images","keys":[...]}   (images.go)
//
// The summons are those the server can open today (its catalog by its dates,
// one per banner as the Summons screen lists them); the events are the event
// chapters the content patch can open (tools\masterdata_patcher's
// config.json, the "Full" preset of patch_content.py).

type contentRequest struct {
	Op      string   `json:"op"`
	Banners []string `json:"banners"`
	Events  []int32  `json:"events"`
}

type contentBanner struct {
	Key   string `json:"key"` // what the choice names (pccontent.Key)
	ID    int32  `json:"id"`  // the first summon with it
	Name  string `json:"name"`
	Kind  string `json:"kind"`
	Note  string `json:"note,omitempty"`
	Img   string `json:"img,omitempty"` // picture key (images.go), when the game files have it
	Start int64  `json:"start,omitempty"`
	// Featured: the characters (and weapons) its promotion shows.
	Featured []string `json:"featured,omitempty"`
	On       bool     `json:"on"`
	Always   bool     `json:"always,omitempty"` // a permanent summon: open whatever the choice (pccontent.AlwaysOpen)
}

type contentEvent struct {
	ID    int32  `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
	Img   string `json:"img,omitempty"`
	On    bool   `json:"on"`
}

// isContentOp: requests answered here rather than by the save slots; they
// are fine while the game runs.
func isContentOp(op string) bool { return strings.HasPrefix(op, "content-") || op == "images" }

func runContent(saveDir, dataRoot, masterPath string, raw []byte) (map[string]any, error) {
	var req contentRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, fmt.Errorf("bad request: %w", err)
	}
	switch req.Op {
	case "images":
		return runImages(dataRoot, raw)
	case "content-list":
		return contentList(saveDir, dataRoot, masterPath)
	case "content-set":
		if err := os.MkdirAll(saveDir, 0700); err != nil {
			return nil, err
		}
		sel := pccontent.Selection{Banners: req.Banners, Events: req.Events}
		if err := pccontent.Write(saveDir, sel); err != nil {
			return nil, err
		}
		return map[string]any{"saved": true, "banners": len(sel.Banners), "events": len(sel.Events)}, nil
	}
	return nil, fmt.Errorf("unknown request %q", req.Op)
}

// bannerKinds orders the summon groups in the list.
var bannerKinds = map[string]int{"Premium": 0, "Chapter": 1, "Limited": 2, "Multi-step": 3, "Event": 4}

func contentList(saveDir, dataRoot, masterPath string) (map[string]any, error) {
	sel, err := pccontent.Read(saveDir)
	if err != nil {
		return nil, err
	}
	holder, err := runtime.NewHolder(masterPath)
	if err != nil {
		return nil, fmt.Errorf("load master data: %w", err)
	}
	tx := loadTexts(dataRoot, "gacha_title", "quest", "quest/event_quest")
	notices := service.NewNotices(service.NoticePackPath(dataRoot), gametime.NowMillis)
	pics := newBundles(dataRoot)
	now := gametime.NowMillis()
	// Names of characters and weapons, for the featured cards and character quests.
	cat, err := buildCatalog(dataRoot)
	if err != nil {
		cat = &catalog{}
	}
	chars := map[int32]character{}
	for _, c := range cat.Characters {
		chars[c.ID] = c
	}
	costumeChar, weaponName := map[int32]int32{}, map[int32]string{}
	for _, c := range cat.Costumes {
		costumeChar[c.ID] = c.Char
	}
	for _, w := range cat.Weapons {
		weaponName[w.ID] = w.Name
	}

	chosenBanner := map[string]bool{}
	for _, k := range sel.Banners {
		chosenBanner[k] = true
	}
	var banners []contentBanner
	seen := map[string]bool{}
	for _, e := range holder.Get().GachaEntries {
		if e.StartDatetime != 0 && now < e.StartDatetime || e.EndDatetime != 0 && now >= e.EndDatetime {
			continue
		}
		if e.GachaLabelType == model.GachaLabelPortalCage || e.GachaLabelType == model.GachaLabelRecycle || !e.IsUserGachaUnlock {
			continue
		}
		// One summon per banner, as the Summons screen lists them (bannerSeen);
		// the choice opens every summon with the banner (its reruns).
		key := pccontent.Key(e.BannerAssetName, e.GachaId)
		if seen[key] {
			continue
		}
		seen[key] = true
		name := masterdata.SummonTitle(e.BannerAssetName)
		if name == "" {
			name = tx["gacha.title."+e.BannerAssetName]
		}
		if name == "" {
			id := e.InformationId
			if id == 0 {
				id = notices.GachaNotice(e.GachaId)
			}
			if id != 0 {
				name = strings.TrimSuffix(strings.TrimSuffix(notices.Title(id), " On Now"), " Updated")
			}
		}
		b := contentBanner{Key: key, ID: e.GachaId, Name: name, Kind: bannerKind(e.GachaLabelType, e.BannerAssetName),
			Start: e.StartDatetime, On: chosenBanner[key]}
		if pccontent.AlwaysOpen(e.GachaLabelType, e.BannerAssetName) {
			b.On, b.Always = true, true
		}
		switch e.UnlockConditionType {
		case model.GachaUnlockMainQuestClear:
			b.Note = "Shows once its story chapter is cleared"
		case model.GachaUnlockDarkMemories:
			b.Note = "Shows once Dark Memories is open"
		case model.GachaUnlockEventRun:
			b.Note = "Shows while its event runs on the calendar"
		}
		if e.BannerAssetName != "" {
			b.Img = pics.key("banner", e.BannerAssetName)
		}
		named := map[string]bool{}
		for _, p := range e.PromotionItems {
			n := ""
			switch model.PossessionType(p.PossessionType) {
			case model.PossessionTypeCostume:
				n = chars[costumeChar[p.PossessionId]].Name
			case model.PossessionTypeWeapon:
				n = weaponName[p.PossessionId]
			}
			if n != "" && !named[n] && len(b.Featured) < 5 {
				named[n] = true
				b.Featured = append(b.Featured, n)
			}
		}
		banners = append(banners, b)
	}
	sort.SliceStable(banners, func(i, j int) bool {
		if bannerKinds[banners[i].Kind] != bannerKinds[banners[j].Kind] {
			return bannerKinds[banners[i].Kind] < bannerKinds[banners[j].Kind]
		}
		return banners[i].ID < banners[j].ID
	})

	groups, order := patcherEvents(dataRoot)
	chosenEvent := map[int32]bool{}
	for _, id := range sel.Events {
		chosenEvent[id] = true
	}
	chapters, err := utils.ReadTable[masterdata.EntityMEventQuestChapter]("m_event_quest_chapter")
	if err != nil {
		return nil, fmt.Errorf("m_event_quest_chapter: %w", err)
	}
	byID := map[int32]masterdata.EntityMEventQuestChapter{}
	for _, c := range chapters {
		byID[c.EventQuestChapterId] = c
	}
	// Character quests name their character and show its picture.
	chapterChar := map[int32]int32{}
	if rows, err := utils.ReadTable[masterdata.EntityMEventQuestChapterCharacter]("m_event_quest_chapter_character"); err == nil {
		for _, r := range rows {
			if _, ok := chapterChar[r.EventQuestChapterId]; !ok {
				chapterChar[r.EventQuestChapterId] = r.CharacterId
			}
		}
	}
	var events []contentEvent
	for _, id := range order {
		c, ok := byID[id]
		if !ok {
			continue
		}
		ev := contentEvent{ID: id, Name: eventName(tx, c), Group: groups[id], On: chosenEvent[id]}
		ev.Img = eventImage(pics, c)
		if ch, ok := chars[chapterChar[id]]; ok {
			if ev.Name == "" || ev.Name == "Character Quest" {
				ev.Name = "Character Quest: " + ch.Name
			} else {
				ev.Name += ": " + ch.Name
			}
			if ev.Img == "" {
				ev.Img = ch.Img
			}
		}
		events = append(events, ev)
	}
	return map[string]any{"banners": banners, "events": events}, nil
}

// eventName is an event chapter's title in the game's text ("" if none).
func eventName(tx texts, c masterdata.EntityMEventQuestChapter) string {
	for _, id := range []int32{c.NameEventQuestTextId, c.EventQuestChapterId} {
		for _, k := range []string{"quest.event.chapter_title.", "quest.event.dungeon.chapter_title."} {
			if n := tx[k+strconv.Itoa(int(id))]; n != "" {
				n = strings.ReplaceAll(strings.ReplaceAll(n, `\n`, " "), "  ", " ")
				return strings.TrimSpace(n)
			}
		}
	}
	return ""
}

// eventImage is the picture key of an event chapter's banner: the Events
// list's banner, its list banner, or its home screen banner ("" if none).
func eventImage(pics bundles, c masterdata.EntityMEventQuestChapter) string {
	var tries [][2]string
	for _, id := range []int32{c.BannerAssetId, c.EventQuestChapterId} {
		if id <= 0 {
			continue
		}
		tries = append(tries,
			[2]string{"event", fmt.Sprintf("event_banner_%03d", id)},
			[2]string{"eventlist", fmt.Sprintf("event_list_banner_%03d", id)},
			[2]string{"eventmom", fmt.Sprintf("event_mom_banner_%03d", id)})
	}
	for _, t := range tries {
		if k := pics.key(t[0], t[1]); k != "" {
			return k
		}
	}
	// A run of the chapter (event_mom_banner_<chapter>NN).
	if pics.root != "" {
		more, _ := filepath.Glob(filepath.Join(pics.root, "ui", "quest", "en", "mom_banner", fmt.Sprintf("event_mom_banner_%d??.assetbundle", c.EventQuestChapterId)))
		sort.Strings(more)
		if len(more) > 0 {
			return pics.key("eventmom", strings.TrimSuffix(filepath.Base(more[0]), ".assetbundle"))
		}
	}
	return ""
}

func bannerKind(label int32, asset string) string {
	switch {
	case label == model.GachaLabelChapter:
		return "Chapter"
	case label == model.GachaLabelEvent:
		return "Event"
	case strings.HasPrefix(asset, model.BannerPrefixStepUp):
		return "Multi-step"
	case strings.HasPrefix(asset, model.BannerPrefixLimited):
		return "Limited"
	}
	return "Premium"
}

// readPatcherConfig is tools\masterdata_patcher\config.json beside the
// data folder (nil if missing).
func readPatcherConfig(dataRoot string) map[string]any {
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Clean(dataRoot)), "tools", "masterdata_patcher", "config.json"))
	if err != nil {
		return nil
	}
	var cfg map[string]any
	if json.Unmarshal(raw, &cfg) != nil {
		return nil
	}
	return cfg
}

// expandIDs reads the patcher's id syntax ("1, 2, 10-20").
func expandIDs(text string) []int32 {
	var out []int32
	for _, part := range strings.Split(text, ",") {
		part = strings.TrimSpace(part)
		lo, hi, isRange := strings.Cut(part, "-")
		a, err := strconv.Atoi(strings.TrimSpace(lo))
		if err != nil {
			continue
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(strings.TrimSpace(hi)); err != nil || b < a || b-a > 10000 {
				continue
			}
		}
		for id := a; id <= b; id++ {
			out = append(out, int32(id))
		}
	}
	return out
}

// patcherEvents reads the event chapters the content patch opens, in its
// config's order, with their group (EVENTS_MAIN, EVENTS_REC_n, EVENTS_VAR_n,
// EVENTS_BONUS_n).
func patcherEvents(dataRoot string) (map[int32]string, []int32) {
	groups := map[int32]string{}
	var order []int32
	cfg := readPatcherConfig(dataRoot)
	var keys []string
	for k, v := range cfg {
		if _, text := v.(string); text && strings.HasPrefix(k, "EVENTS_") {
			keys = append(keys, k)
		}
	}
	rank := map[string]int{"MAIN": 0, "REC": 1, "VAR": 2, "BONUS": 3}
	sort.Slice(keys, func(i, j int) bool {
		gi, gj := strings.Split(keys[i], "_")[1], strings.Split(keys[j], "_")[1]
		if rank[gi] != rank[gj] {
			return rank[gi] < rank[gj]
		}
		return keys[i] < keys[j]
	})
	names := map[string]string{"MAIN": "Permanent", "REC": "Record", "VAR": "Variation", "BONUS": "Bonus"}
	for _, k := range keys {
		group := names[strings.Split(k, "_")[1]]
		for _, id := range expandIDs(cfg[k].(string)) {
			if _, dup := groups[id]; !dup {
				groups[id] = group
				order = append(order, id)
			}
		}
	}
	return groups, order
}
