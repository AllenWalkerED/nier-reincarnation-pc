package service

import (
	"fmt"
	"os"
	"sort"
	"testing"

	"lunar-tear/server/internal/model"
)

// TestGachaFeaturedAudit prints, per premium summon, the ★4 characters its
// medal exchange sells, the ★4 characters the server features, and the
// difference. GACHA_AUDIT=1.
func TestGachaFeaturedAudit(t *testing.T) {
	if os.Getenv("GACHA_AUDIT") == "" {
		t.Skip("GACHA_AUDIT not set")
	}
	byId, handler := catalogById(t)
	pool := handler.Pool
	var ids []int32
	for id, e := range byId {
		if e.GachaLabelType == model.GachaLabelPremium {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(a, b int) bool { return ids[a] < ids[b] })
	four := func(id int32) bool {
		c, ok := pool.CostumeById[id]
		return ok && c.RarityType == model.RaritySSRare
	}
	mismatch, noShop, unob := 0, 0, 0
	for _, id := range ids {
		e := byId[id]
		shop := map[int32]bool{}
		for _, se := range pool.ShopFeaturedByMedal[e.MedalConsumableItemId] {
			if four(se.CostumeId) {
				shop[se.CostumeId] = true
			}
		}
		feat := map[int32]bool{}
		if bp := pool.BannerPools[id]; bp != nil {
			for _, f := range bp.Featured {
				if f.PossessionType == int32(model.PossessionTypeCostume) && f.RarityType == model.RaritySSRare {
					feat[f.PossessionId] = true
				}
			}
		}
		var missing, extra []int32
		for c := range shop {
			if !feat[c] {
				missing = append(missing, c)
			}
		}
		for c := range feat {
			if !shop[c] {
				extra = append(extra, c)
			}
		}
		sort.Slice(missing, func(a, b int) bool { return missing[a] < missing[b] })
		sort.Slice(extra, func(a, b int) bool { return extra[a] < extra[b] })
		if len(shop) == 0 {
			noShop++
		}
		inPool := map[int32]bool{}
		if bp := pool.BannerPools[id]; bp != nil {
			for _, l := range bp.CostumesByRarity {
				for _, c := range l {
					inPool[c.PossessionId] = true
				}
			}
			for _, l := range bp.WeaponsByRarity {
				for _, w := range l {
					inPool[w.PossessionId] = true
				}
			}
		}
		var unobtainable []string
		for _, pi := range e.PromotionItems {
			id := pi.PossessionId
			if c, ok := pool.CharacterOfWeapon(id); ok && pi.PossessionType == int32(model.PossessionTypeWeapon) {
				id = c.PossessionId // a character weapon comes as its character
			}
			if !inPool[id] {
				unobtainable = append(unobtainable, fmt.Sprintf("%d/%d(target=%v)", pi.PossessionType, pi.PossessionId, pi.IsTarget))
			}
		}
		if len(unobtainable) > 0 {
			unob++
			t.Logf("UNOBTAINABLE gacha %d %s mode %d: panel shows %d cards, not in pool: %v", id, e.BannerAssetName, e.GachaModeType, len(e.PromotionItems), unobtainable)
		}
		flag := ""
		if len(shop) > 0 && (len(missing) > 0 || len(extra) > 0) {
			flag = "MISMATCH"
			mismatch++
		}
		t.Logf("%s gacha %d %s medal %d start %d: shop4★ %d featured4★ %d missing %v extra %v",
			flag, id, e.BannerAssetName, e.MedalConsumableItemId, e.StartDatetime, len(shop), len(feat), missing, extra)
	}
	t.Logf("%s", fmt.Sprintf("TOTAL %d premium summons, %d without a medal shop, %d whose featured ★4 differ from their shop, %d with panel cards not in their pool", len(ids), noShop, mismatch, unob))
}
