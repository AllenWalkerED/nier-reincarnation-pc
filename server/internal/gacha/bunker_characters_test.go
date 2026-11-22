package gacha

import (
	"testing"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
)

func TestCharacterWeaponComesWithItsCharacter(t *testing.T) {
	c := masterdata.GachaPoolItem{PossessionType: int32(model.PossessionTypeCostume), PossessionId: 32000, RarityType: model.RaritySSRare, CharacterId: 3}
	pool := &masterdata.GachaCatalog{
		CostumeById:      map[int32]masterdata.GachaPoolItem{32000: c},
		CostumeWeaponMap: map[int32]int32{32000: 320001},
	}
	items := []DrawnItem{
		{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 320001, RarityType: model.RaritySSRare},
		{PossessionType: int32(model.PossessionTypeWeapon), PossessionId: 400001, RarityType: model.RaritySSRare},
	}
	withCharacters(items, pool)
	if items[0].PossessionType != int32(model.PossessionTypeCostume) || items[0].PossessionId != 32000 || items[0].CharacterId != 3 {
		t.Errorf("character weapon drawn as %+v, want its character", items[0])
	}
	if items[1].PossessionType != int32(model.PossessionTypeWeapon) || items[1].PossessionId != 400001 {
		t.Errorf("weapon without a character changed: %+v", items[1])
	}
}
