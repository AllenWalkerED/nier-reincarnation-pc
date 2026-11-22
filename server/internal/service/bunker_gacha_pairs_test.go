package service

import (
	"os"
	"testing"

	"lunar-tear/server/internal/model"
)

func TestGachaPairedWeapons(t *testing.T) {
	if os.Getenv("GACHA_AUDIT") == "" {
		t.Skip()
	}
	_, handler := catalogById(t)
	pool := handler.Pool
	paired := map[int32]int32{}
	for c, w := range pool.CostumeWeaponMap {
		paired[w] = c
	}
	for _, r := range []int32{model.RaritySSRare, model.RaritySRare, model.RarityRare} {
		p, plain := 0, 0
		for _, w := range pool.StandardWeaponsByRarity[r] {
			if _, ok := paired[w.PossessionId]; ok {
				p++
			} else {
				plain++
			}
		}
		all, allPlain := 0, 0
		for _, w := range pool.WeaponsByRarity[r] {
			all++
			if _, ok := paired[w.PossessionId]; !ok {
				allPlain++
			}
		}
		t.Logf("rarity %d: standard weapons %d character weapons + %d plain; all gacha weapons %d (%d plain); standard costumes %d",
			r, p, plain, all, allPlain, len(pool.StandardCostumesByRarity[r]))
	}
}
