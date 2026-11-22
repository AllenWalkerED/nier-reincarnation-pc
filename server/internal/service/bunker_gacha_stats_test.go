package service

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

// TestGachaDrawStats draws every premium summon (or GACHA_STATS_IDS) many
// times in memory and prints, per summon, its featured ★4 characters, its
// ★4 character pool, the share of ★4 characters that are featured and the
// share drawn from outside its featured + standard lineup.
// BUNKER_TEST_MASTER=<master.bin.e> GACHA_STATS=20000 go test -run TestGachaDrawStats ./internal/service
func TestGachaDrawStats(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("GACHA_STATS"))
	if n == 0 {
		t.Skip("GACHA_STATS not set")
	}
	byId, handler := catalogById(t)
	pool := handler.Pool
	var ids []int32
	if s := os.Getenv("GACHA_STATS_IDS"); s != "" {
		for _, f := range strings.Split(s, ",") {
			v, _ := strconv.Atoi(f)
			ids = append(ids, int32(v))
		}
	} else {
		for id, e := range byId {
			if e.GachaLabelType == model.GachaLabelPremium && len(e.PricePhases) > 0 {
				ids = append(ids, id)
			}
		}
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	std := map[int32]bool{}
	for _, c := range pool.StandardCostumesByRarity[model.RaritySSRare] {
		std[c.PossessionId] = true
	}
	pairedCostume := map[int32]int32{}
	for c, w := range pool.CostumeWeaponMap {
		pairedCostume[w] = c
	}
	var totOrphan, totOrphanFeat int
	var totFeat, totFour, totForeign, banners, noFeat, totDraws, neverBanners int
	for _, id := range ids {
		e := byId[id]
		bp := pool.BannerPools[id]
		if bp == nil {
			continue
		}
		feat := map[int32]bool{}
		var featList []string
		for _, f := range bp.Featured {
			if f.PossessionType == int32(model.PossessionTypeCostume) && f.RarityType == model.RaritySSRare {
				feat[f.PossessionId] = true
				featList = append(featList, fmt.Sprint(f.PossessionId))
			}
		}
		// The single draw phase (DrawCount 1) if any, else the first.
		phase := e.PricePhases[0]
		for _, p := range e.PricePhases {
			if p.DrawCount == 1 && p.LimitExecCount == 0 {
				phase = p
				break
			}
		}
		four, featHit, foreign, draws, orphan, orphanFeat := 0, 0, 0, 0, 0, 0
		got := map[int32]int{}
		anyGot := map[int32]int{}
		foreignIds := map[int32]int{}
		// One session: a fresh player draws the phase once (a step-up: every
		// step in order), so use limits never stop the count.
		var phases []store.GachaPricePhaseEntry
		if e.GachaModeType == model.GachaModeStepup {
			phases = e.PricePhases
		} else {
			phases = []store.GachaPricePhaseEntry{phase}
		}
		for i := 0; i < n; i++ {
			u := store.UserState{}
			u.EnsureMaps()
			for _, ph := range phases {
				u.Gem.FreeGem, u.Gem.PaidGem = 1<<29, 1<<29
				u.ConsumableItems[ph.PriceId] = 1 << 29
				res, err := handler.HandleDraw(&u, e, ph.PhaseId, 1)
				if err != nil {
					t.Logf("%d: %v", id, err)
					i = n
					break
				}
				draws += len(res.Items)
				for _, it := range res.Items {
					if c, ok := pairedCostume[it.PossessionId]; ok && it.PossessionType == int32(model.PossessionTypeWeapon) && it.RarityType == model.RaritySSRare {
						orphan++
						if feat[c] {
							orphanFeat++
						}
					}
					if it.PossessionType == int32(model.PossessionTypeCostume) {
						anyGot[it.PossessionId]++
					}
					if it.PossessionType != int32(model.PossessionTypeCostume) || it.RarityType != model.RaritySSRare {
						continue
					}
					four++
					got[it.PossessionId]++
					if feat[it.PossessionId] {
						featHit++
					} else if !std[it.PossessionId] {
						foreign++
						foreignIds[it.PossessionId]++
					}
				}
			}
		}
		banners++
		if len(feat) == 0 {
			noFeat++
		}
		totFeat += featHit
		totFour += four
		totDraws += draws
		totOrphan += orphan
		totOrphanFeat += orphanFeat
		totForeign += foreign
		pct := func(a, b int) float64 {
			if b == 0 {
				return 0
			}
			return 100 * float64(a) / float64(b)
		}
		var never []int32
		for _, pi := range e.PromotionItems {
			if pi.PossessionType == int32(model.PossessionTypeCostume) && anyGot[pi.PossessionId] == 0 {
				never = append(never, pi.PossessionId)
			}
		}
		if len(never) > 0 {
			neverBanners++
			t.Logf("NEVER gacha %d (%s): panel characters never drawn in %d draws: %v", id, e.BannerAssetName, draws, never)
		}
		t.Logf("gacha %d (%s) phase %d x%d: featured4★=[%s] pool4★=%d | draws %d 4★chars %d, featured %.1f%% (%.2f%% of draws), foreign %.1f%% | 4★ character weapons without their character %d (featured characters' %d) | distinct %d foreign %v",
			id, e.BannerAssetName, phase.PhaseId, phase.DrawCount, strings.Join(featList, ","), len(bp.CostumesByRarity[model.RaritySSRare]),
			draws, four, pct(featHit, four), pct(featHit, draws), pct(foreign, four), orphan, orphanFeat, len(got), foreignIds)
	}
	t.Logf("TOTAL 4★ character weapons drawn without their character: %d (%.2f%% of draws), of featured characters: %d (%.2f%% of draws); featured 4★ characters drawn: %d (%.2f%% of draws)",
		totOrphan, 100*float64(totOrphan)/float64(totDraws), totOrphanFeat, 100*float64(totOrphanFeat)/float64(totDraws), totFeat, 100*float64(totFeat)/float64(totDraws))
	t.Logf("TOTAL %d banners whose panel shows a character never drawn", neverBanners)
	t.Logf("TOTAL %d banners (%d with no featured ★4 character): ★4 characters %d, featured %.1f%%, outside featured+standard %.1f%%, draws %d",
		banners, noFeat, totFour, 100*float64(totFeat)/float64(max(totFour, 1)), 100*float64(totForeign)/float64(max(totFour, 1)), totDraws)
}
