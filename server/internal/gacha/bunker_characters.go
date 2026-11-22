package gacha

import (
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/model"
)

// withCharacters turns every character weapon a summon drew into its
// character. The game gave a character with its weapon ("you can get this
// character by acquiring the ... weapon", "★★★★ weapons (w/ character)");
// the weapon still arrives, as the character's bonus. Weapon lineups,
// guarantees and swaps can name a character weapon; so did the weapon tier,
// which gave featured characters' weapons alone about as often as the
// characters themselves.
func withCharacters(items []DrawnItem, pool *masterdata.GachaCatalog) {
	if pool == nil {
		return
	}
	for i, it := range items {
		if it.PossessionType != int32(model.PossessionTypeWeapon) {
			continue
		}
		if c, ok := pool.CharacterOfWeapon(it.PossessionId); ok {
			items[i] = DrawnItem{PossessionType: c.PossessionType, PossessionId: c.PossessionId, RarityType: c.RarityType, CharacterId: c.CharacterId, Count: it.Count}
		}
	}
}
