package pcsave

import (
	"fmt"
	"sort"

	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/utils"
)

// The editor's lists, from the game data (loaded by lunarbase.PCRead first).

type character struct {
	ID      int32  `json:"id"`
	Name    string `json:"name"`
	Costume int32  `json:"costume"` // its first costume (unlocks it)
	Weapon  int32  `json:"weapon"`
	Img     string `json:"img,omitempty"` // its first costume's picture key (images.go)
}

type entry struct {
	ID     int32  `json:"id"`
	Name   string `json:"name"`
	Rarity int32  `json:"rarity,omitempty"`
	Char   int32  `json:"char,omitempty"`
	Kind   int32  `json:"kind,omitempty"`
	Img    string `json:"img,omitempty"` // picture key (images.go), when the game files have it
}

type catalog struct {
	Characters  []character `json:"characters"`
	Costumes    []entry     `json:"costumes"`
	Weapons     []entry     `json:"weapons"` // one per evolution chain: its first form
	Companions  []entry     `json:"companions"`
	Materials   []entry     `json:"materials"`
	Consumables []entry     `json:"consumables"`
	GoldID      int32       `json:"goldId"`
}

// weaponNameOverrides are first forms whose name is filed under another asset
// (Bunker's extract_names.py).
var weaponNameOverrides = map[int32]string{
	101001: "wp005501", 101011: "wp001501", 101021: "wp006001", 101031: "wp003039", 101041: "wp002046",
}

func buildCatalog(dataRoot string) (*catalog, error) {
	tx := loadTexts(dataRoot, "character", "possession/costume", "possession/weapon", "possession/companion",
		"possession/material", "possession/consumable_item")
	name := func(key, fallback string) string {
		if v := tx[key]; v != "" {
			return v
		}
		return fallback
	}
	c := &catalog{}
	pics := newBundles(dataRoot)
	if cfg, err := masterdata.LoadGameConfig(); err == nil {
		c.GoldID = cfg.ConsumableItemIdForGold
	}

	chars, err := utils.ReadTable[masterdata.EntityMCharacter]("m_character")
	if err != nil {
		return nil, err
	}
	charName := map[int32]string{}
	for _, x := range chars {
		n := name(fmt.Sprintf("character.name.%d", x.NameCharacterTextId), "")
		if n == "" {
			n = name(fmt.Sprintf("character.name.%d.1", x.NameCharacterTextId), fmt.Sprintf("Character %d", x.CharacterId))
		}
		charName[x.CharacterId] = n
		if x.DefaultCostumeId > 0 {
			c.Characters = append(c.Characters, character{ID: x.CharacterId, Name: n, Costume: x.DefaultCostumeId, Weapon: x.DefaultWeaponId})
		}
	}
	playable := map[int32]bool{}
	for _, x := range c.Characters {
		playable[x.ID] = true
	}

	costumes, err := utils.ReadTable[masterdata.EntityMCostume]("m_costume")
	if err != nil {
		return nil, err
	}
	for _, x := range costumes {
		if !playable[x.CharacterId] {
			continue
		}
		prefix := "mt"
		if x.CostumeAssetCategoryType == 1 {
			prefix = "ch"
		}
		asset := fmt.Sprintf("%s%03d%03d", prefix, x.ActorSkeletonId, x.AssetVariationId)
		c.Costumes = append(c.Costumes, entry{ID: x.CostumeId, Name: name("costume.name."+asset, fmt.Sprintf("%s costume %d", charName[x.CharacterId], x.CostumeId)),
			Rarity: x.RarityType, Char: x.CharacterId, Img: pics.key("costume", asset)})
	}

	weapons, err := utils.ReadTable[masterdata.EntityMWeapon]("m_weapon")
	if err != nil {
		return nil, err
	}
	evo, err := utils.ReadTable[masterdata.EntityMWeaponEvolutionGroup]("m_weapon_evolution_group")
	if err != nil {
		return nil, err
	}
	// Weapons outside the evolution chains are the game's test and enemy rows.
	first := map[int32]bool{}
	for _, x := range evo {
		if x.EvolutionOrder == 1 {
			first[x.WeaponId] = true
		}
	}
	for _, w := range weapons {
		if !first[w.WeaponId] {
			continue
		}
		prefix := "mw"
		if w.WeaponCategoryType == 1 {
			prefix = "wp"
		}
		assets := []string{}
		if o := weaponNameOverrides[w.WeaponId]; o != "" {
			assets = append(assets, o)
		}
		assets = append(assets, fmt.Sprintf("%s%03d%03d", prefix, w.WeaponType, w.AssetVariationId),
			fmt.Sprintf("%s%03d%03d", prefix, w.WeaponType, w.WeaponId%1000))
		n, img := "", ""
		for _, a := range assets {
			if img == "" {
				img = pics.key("weapon", a)
			}
		}
		for _, a := range assets {
			if n = name("weapon.name."+a+".1", ""); n != "" {
				break
			}
			if n = name("weapon.name."+a+".01", ""); n != "" {
				break
			}
		}
		if n == "" {
			continue // no name: not a weapon the game shows (test rows)
		}
		c.Weapons = append(c.Weapons, entry{ID: w.WeaponId, Name: n, Rarity: w.RarityType, Kind: w.WeaponType, Img: img})
	}

	companions, err := utils.ReadTable[masterdata.EntityMCompanion]("m_companion")
	if err != nil {
		return nil, err
	}
	for _, x := range companions {
		asset := fmt.Sprintf("cm%03d%03d", x.ActorSkeletonId, x.AssetVariationId)
		c.Companions = append(c.Companions, entry{ID: x.CompanionId,
			Name: name("companion.name."+asset, fmt.Sprintf("Companion %d", x.CompanionId)), Img: pics.key("companion", asset)})
	}

	materials, err := utils.ReadTable[masterdata.EntityMMaterial]("m_material")
	if err != nil {
		return nil, err
	}
	for _, x := range materials {
		n := name(fmt.Sprintf("material.name.%03d%03d", x.AssetCategoryId, x.AssetVariationId), "")
		if n == "" {
			continue
		}
		c.Materials = append(c.Materials, entry{ID: x.MaterialId, Name: n, Rarity: x.RarityType, Kind: x.MaterialType,
			Img: pics.key("material", fmt.Sprintf("material%03d%03d", x.AssetCategoryId, x.AssetVariationId))})
	}
	items, err := utils.ReadTable[masterdata.EntityMConsumableItem]("m_consumable_item")
	if err != nil {
		return nil, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].SortOrder < items[j].SortOrder })
	for _, x := range items {
		n := name(fmt.Sprintf("consumable_item.name.%03d%03d", x.AssetCategoryId, x.AssetVariationId), "")
		if n == "" || x.ConsumableItemId == c.GoldID {
			continue
		}
		c.Consumables = append(c.Consumables, entry{ID: x.ConsumableItemId, Name: n, Kind: x.ConsumableItemType,
			Img: pics.key("consumable", fmt.Sprintf("consumable%03d%03d", x.AssetCategoryId, x.AssetVariationId))})
	}
	costumeImg := map[int32]string{}
	for _, x := range c.Costumes {
		costumeImg[x.ID] = x.Img
	}
	for i := range c.Characters {
		c.Characters[i].Img = costumeImg[c.Characters[i].Costume]
	}
	sort.SliceStable(c.Characters, func(i, j int) bool { return c.Characters[i].ID < c.Characters[j].ID })
	return c, nil
}
