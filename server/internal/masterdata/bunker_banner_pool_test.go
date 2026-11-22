package masterdata

import (
	"testing"

	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/store"
)

// A banner's weapon tier holds weapons without a character; a featured
// character weapon makes its character featured; panel characters join the pool.
func TestBannerPoolCharactersComeWithTheirWeapons(t *testing.T) {
	costume := func(id int32) GachaPoolItem {
		return GachaPoolItem{PossessionType: int32(model.PossessionTypeCostume), PossessionId: id, RarityType: model.RaritySSRare, CharacterId: id / 10}
	}
	weapon := func(id int32) GachaPoolItem {
		return GachaPoolItem{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: id, RarityType: model.RaritySSRare}
	}
	std, feat, featByWeapon, card := costume(31000), costume(32000), costume(33000), costume(34000)
	plain := weapon(400001)
	pool := &GachaCatalog{
		CostumeById: map[int32]GachaPoolItem{std.PossessionId: std, feat.PossessionId: feat, featByWeapon.PossessionId: featByWeapon, card.PossessionId: card},
		WeaponById: map[int32]GachaPoolItem{plain.PossessionId: plain, 310001: weapon(310001), 320001: weapon(320001),
			330001: weapon(330001), 340001: weapon(340001)},
		CostumeWeaponMap:         map[int32]int32{31000: 310001, 32000: 320001, 33000: 330001, 34000: 340001},
		StandardCostumesByRarity: map[int32][]GachaPoolItem{model.RaritySSRare: {std}},
		StandardWeaponsByRarity:  map[int32][]GachaPoolItem{model.RaritySSRare: {plain, weapon(310001)}},
		FeaturedByGacha: map[int32]FeaturedSet{
			1: {Costumes: []GachaPoolItem{feat}, Weapons: []GachaPoolItem{weapon(330001)}},
		},
	}
	entries := []store.GachaCatalogEntry{{GachaId: 1, GachaLabelType: model.GachaLabelPremium,
		PromotionItems: []store.GachaPromotionItem{{PossessionType: int32(model.PossessionTypeCostume), PossessionId: 34000}}}}
	pool.BuildBannerPools(entries)
	pool.addPanelCardsToPools(entries)
	bp := pool.BannerPools[1]

	weapons := bp.WeaponsByRarity[model.RaritySSRare]
	if len(weapons) != 1 || weapons[0].PossessionId != plain.PossessionId {
		t.Fatalf("weapon tier %v, want only the weapon without a character", weapons)
	}
	var featured []int32
	for _, f := range bp.Featured {
		featured = append(featured, f.PossessionId)
	}
	if len(featured) != 2 || featured[0] != 32000 || featured[1] != 33000 {
		t.Fatalf("featured %v, want characters 32000 and 33000 (the latter from its weapon)", featured)
	}
	in := map[int32]bool{}
	for _, c := range bp.CostumesByRarity[model.RaritySSRare] {
		in[c.PossessionId] = true
	}
	for _, id := range []int32{31000, 32000, 33000, 34000} {
		if !in[id] {
			t.Errorf("character %d missing from the pool %v", id, bp.CostumesByRarity[model.RaritySSRare])
		}
	}
	if c, ok := pool.CharacterOfWeapon(320001); !ok || c.PossessionId != 32000 {
		t.Errorf("weapon 320001 comes with %v %v", c, ok)
	}
}
