// PC port: the launcher's Settings › Saves editor (internal/pcsave). It reads
// what a player owns and applies one batch of changes (own these costumes,
// weapons and companions, at these levels; hold these amounts of gems, gold
// and items) in one UpdateUser transaction, with the same granter and
// catalogs as the grant actions above. Costs are not taken.

package lunarbase

import (
	"errors"
	"fmt"
	"math"
	"time"

	"lunar-tear/server/internal/database"
	"lunar-tear/server/internal/gameutil"
	"lunar-tear/server/internal/masterdata"
	"lunar-tear/server/internal/masterdata/memorydb"
	"lunar-tear/server/internal/model"
	"lunar-tear/server/internal/questflow"
	"lunar-tear/server/internal/store"
)

// PCOwned is one owned piece: its id and level (the highest when several
// rows share the id), and its ascension (limit break) count.
type PCOwned struct {
	Level      int32 `json:"level"`
	LimitBreak int32 `json:"lb"`
	Count      int   `json:"count"`
}

// PCPlayer is what the editor shows of a save's player.
type PCPlayer struct {
	UserID      int64                  `json:"userId"`
	Name        string                 `json:"name"`
	Level       int32                  `json:"level"`
	FreeGems    int32                  `json:"freeGems"`
	PaidGems    int32                  `json:"paidGems"`
	Gold        int32                  `json:"gold"`
	Costumes    map[int32]PCOwned      `json:"costumes"`
	Weapons     map[int32]PCOwned      `json:"weapons"`
	Companions  map[int32]PCOwned      `json:"companions"`
	Characters  map[int32]int32        `json:"characters"` // id -> rank
	Materials   map[int32]int32        `json:"materials"`
	Consumables map[int32]int32        `json:"consumables"`
	Weapons999  int                    `json:"weaponRows"`
	Limits      store.PossessionLimits `json:"limits"`
}

// PCLevel asks for one costume, weapon or companion: owned, and at Level
// when Level > 0 (-1: the highest level it can reach; 0: keep).
type PCLevel struct {
	ID    int32 `json:"id"`
	Level int32 `json:"level"`
}

// PCEdit is one batch of changes. Amounts are what the player will hold
// (nil: unchanged; 0 removes the item).
type PCEdit struct {
	Costumes    []PCLevel       `json:"costumes"`
	Weapons     []PCLevel       `json:"weapons"`
	Companions  []PCLevel       `json:"companions"`
	FreeGems    *int32          `json:"freeGems"`
	PaidGems    *int32          `json:"paidGems"`
	Gold        *int32          `json:"gold"`
	Materials   map[int32]int32 `json:"materials"`
	Consumables map[int32]int32 `json:"consumables"`
}

// PCResult says what a batch did.
type PCResult struct {
	Granted int      `json:"granted"`
	Leveled int      `json:"leveled"`
	Items   int      `json:"items"`
	Notes   []string `json:"notes,omitempty"`
}

const pcWeaponRowCap = 999 // the game's weapon inventory limit (Pod Programs' too)

type pcCatalogs struct {
	granter    *store.PossessionGranter
	costumes   *masterdata.CostumeCatalog
	rebirth    *masterdata.CharacterRebirthCatalog
	weapons    *masterdata.WeaponCatalog
	goldID     int32
	prevWeapon map[int32]int32 // evolution: weapon -> the form before it
}

func loadPCCatalogs(masterPath string) (*pcCatalogs, error) {
	if err := memorydb.Init(masterPath); err != nil {
		return nil, fmt.Errorf("init master data: %w", err)
	}
	parts, err := masterdata.LoadPartsCatalog()
	if err != nil {
		return nil, fmt.Errorf("load parts catalog: %w", err)
	}
	quests, err := masterdata.LoadQuestCatalog(parts)
	if err != nil {
		return nil, fmt.Errorf("load quest catalog: %w", err)
	}
	cfg, err := masterdata.LoadGameConfig()
	if err != nil {
		return nil, fmt.Errorf("load game config: %w", err)
	}
	mats, err := masterdata.LoadMaterialCatalog()
	if err != nil {
		return nil, fmt.Errorf("load material catalog: %w", err)
	}
	costumes, err := masterdata.LoadCostumeCatalog(mats)
	if err != nil {
		return nil, fmt.Errorf("load costume catalog: %w", err)
	}
	rebirth, err := masterdata.LoadCharacterRebirthCatalog()
	if err != nil {
		return nil, fmt.Errorf("load character rebirth catalog: %w", err)
	}
	weapons, err := masterdata.LoadWeaponCatalog(mats)
	if err != nil {
		return nil, fmt.Errorf("load weapon catalog: %w", err)
	}
	c := &pcCatalogs{
		granter:    questflow.BuildGranter(quests, cfg),
		costumes:   costumes,
		rebirth:    rebirth,
		weapons:    weapons,
		goldID:     cfg.ConsumableItemIdForGold,
		prevWeapon: map[int32]int32{},
	}
	if c.granter.GoldConsumableItemId != 0 {
		c.goldID = c.granter.GoldConsumableItemId
	}
	for from, to := range weapons.EvolutionNextWeaponId {
		c.prevWeapon[to] = from
	}
	return c, nil
}

// pcChainBase is the first form of a weapon's evolution chain.
func (c *pcCatalogs) chainBase(id int32) int32 {
	for i := 0; i < 32; i++ {
		prev, ok := c.prevWeapon[id]
		if !ok {
			break
		}
		id = prev
	}
	return id
}

// PCRead reads the player of a save (the most recently started one).
func PCRead(dbPath, masterPath string) (*PCPlayer, error) {
	c, err := loadPCCatalogs(masterPath)
	if err != nil {
		return nil, err
	}
	db, st, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	uid, err := pcPlayerID(dbPath)
	if err != nil {
		return nil, err
	}
	u, err := st.LoadUser(uid)
	if err != nil {
		return nil, err
	}
	p := &PCPlayer{
		UserID: uid, Name: u.Profile.Name, Level: u.Status.Level,
		FreeGems: u.Gem.FreeGem, PaidGems: u.Gem.PaidGem, Gold: u.ConsumableItems[c.goldID],
		Costumes: map[int32]PCOwned{}, Weapons: map[int32]PCOwned{}, Companions: map[int32]PCOwned{},
		Characters: map[int32]int32{}, Materials: map[int32]int32{}, Consumables: map[int32]int32{},
		Weapons999: len(u.Weapons), Limits: c.granter.Limits,
	}
	add := func(m map[int32]PCOwned, id, level, lb int32) {
		o := m[id]
		o.Count++
		if level > o.Level {
			o.Level = level
		}
		if lb > o.LimitBreak {
			o.LimitBreak = lb
		}
		m[id] = o
	}
	for _, x := range u.Costumes {
		add(p.Costumes, x.CostumeId, x.Level, x.LimitBreakCount)
	}
	// Weapons are listed by their chain's first form.
	for _, x := range u.Weapons {
		add(p.Weapons, c.chainBase(x.WeaponId), x.Level, x.LimitBreakCount)
	}
	for _, x := range u.Companions {
		add(p.Companions, x.CompanionId, x.Level, 0)
	}
	for id, x := range u.Characters {
		p.Characters[id] = x.Level
	}
	for id, n := range u.Materials {
		p.Materials[id] = n
	}
	for id, n := range u.ConsumableItems {
		if id != c.goldID {
			p.Consumables[id] = n
		}
	}
	return p, nil
}

// pcPlayerID is the player the game signs in as: the one of the newest
// session, else the most recently started.
func pcPlayerID(dbPath string) (int64, error) {
	db, err := database.Open(dbPath)
	if err != nil {
		return 0, fmt.Errorf("open db: %w", err)
	}
	defer db.Close()
	var uid int64
	err = db.QueryRow(`SELECT u.user_id FROM sessions s JOIN users u ON u.uuid = s.uuid ORDER BY s.rowid DESC LIMIT 1`).Scan(&uid)
	if err != nil {
		err = db.QueryRow(`SELECT user_id FROM users ORDER BY game_start_datetime DESC, user_id DESC LIMIT 1`).Scan(&uid)
	}
	if err != nil {
		return 0, errors.New("this save has no player yet: play it once first")
	}
	return uid, nil
}

// PCApply applies one batch to the player of the save at dbPath.
func PCApply(dbPath, masterPath string, e PCEdit) (*PCResult, error) {
	c, err := loadPCCatalogs(masterPath)
	if err != nil {
		return nil, err
	}
	uid, err := pcPlayerID(dbPath)
	if err != nil {
		return nil, err
	}
	db, st, err := openDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	res := &PCResult{}
	now := time.Now().UnixMilli()
	var fail error
	_, err = st.UpdateUser(uid, func(u *store.UserState) {
		fail = c.apply(u, e, now, res)
	})
	if fail != nil {
		return nil, fail
	}
	if err != nil {
		return nil, fmt.Errorf("save: %w", err)
	}
	return res, nil
}

func (c *pcCatalogs) apply(u *store.UserState, e PCEdit, now int64, res *PCResult) error {
	// Amounts are checked before anything changes.
	limit := func(n int32, cap int32, what string) error {
		if n < 0 {
			return fmt.Errorf("%s cannot be negative", what)
		}
		if cap > 0 && n > cap {
			return fmt.Errorf("%s: the game holds at most %d", what, cap)
		}
		return nil
	}
	free, paid := int64(u.Gem.FreeGem), int64(u.Gem.PaidGem)
	if e.FreeGems != nil {
		if err := limit(*e.FreeGems, 0, "Gems"); err != nil {
			return err
		}
		free = int64(*e.FreeGems)
	}
	if e.PaidGems != nil {
		if err := limit(*e.PaidGems, 0, "Paid gems"); err != nil {
			return err
		}
		paid = int64(*e.PaidGems)
	}
	if free+paid > math.MaxInt32 {
		return errors.New("gems: free and paid together must stay under 2,147,483,647")
	}
	if e.Gold != nil {
		if err := limit(*e.Gold, c.granter.Limits.Money, "Gold"); err != nil {
			return err
		}
	}
	for id, n := range e.Materials {
		if err := limit(n, c.granter.Limits.Material, fmt.Sprintf("Material %d", id)); err != nil {
			return err
		}
	}
	for id, n := range e.Consumables {
		cap := c.granter.Limits.ConsumableItem
		if c.granter.GachaMedalItemIds[id] {
			cap = c.granter.Limits.GachaMedal
		}
		if id == c.goldID {
			return errors.New("gold is set with its own field")
		}
		if err := limit(n, cap, fmt.Sprintf("Item %d", id)); err != nil {
			return err
		}
	}
	newWeapons := 0
	for _, w := range e.Weapons {
		if c.ownedWeapon(u, w.ID) == "" {
			newWeapons++
		}
	}
	if len(u.Weapons)+newWeapons > pcWeaponRowCap {
		return fmt.Errorf("weapons: the game holds at most %d (you have %d, %d more asked)", pcWeaponRowCap, len(u.Weapons), newWeapons)
	}

	if e.FreeGems != nil {
		u.Gem.FreeGem = *e.FreeGems
		res.Items++
	}
	if e.PaidGems != nil {
		u.Gem.PaidGem = *e.PaidGems
		res.Items++
	}
	setCount := func(m map[int32]int32, id, n int32) {
		if n == 0 {
			delete(m, id)
		} else {
			m[id] = n
		}
		res.Items++
	}
	if e.Gold != nil {
		setCount(u.ConsumableItems, c.goldID, *e.Gold)
	}
	for id, n := range e.Materials {
		setCount(u.Materials, id, n)
	}
	for id, n := range e.Consumables {
		setCount(u.ConsumableItems, id, n)
	}

	for _, x := range e.Costumes {
		if _, ok := c.costumes.Costumes[x.ID]; !ok {
			return fmt.Errorf("costume %d is not in the game data", x.ID)
		}
		if c.granter.GrantCostume(u, x.ID, now) {
			res.Granted++
		}
		if x.Level != 0 {
			for key, row := range u.Costumes {
				if row.CostumeId == x.ID {
					if c.setCostumeLevel(u, key, x.Level, now) {
						res.Leveled++
					}
				}
			}
		}
	}
	for _, x := range e.Weapons {
		if _, ok := c.weapons.Weapons[x.ID]; !ok {
			return fmt.Errorf("weapon %d is not in the game data", x.ID)
		}
		key := c.ownedWeapon(u, x.ID)
		if key == "" {
			c.granter.GrantWeapon(u, x.ID, now)
			res.Granted++
			key = c.ownedWeapon(u, x.ID)
		}
		if x.Level != 0 && key != "" {
			if c.setWeaponLevel(u, key, x.Level, now) {
				res.Leveled++
			}
		}
	}
	for _, x := range e.Companions {
		if c.granter.GrantCompanion(u, x.ID, now) {
			res.Granted++
		}
		if x.Level == 0 {
			continue
		}
		level := x.Level
		if level < 0 || level > massCompanionMaxLevel {
			level = massCompanionMaxLevel
		}
		for key, row := range u.Companions {
			if row.CompanionId == x.ID && row.Level != level {
				row.Level = level
				row.LatestVersion = now
				u.Companions[key] = row
				res.Leveled++
			}
		}
	}
	return nil
}

// ownedWeapon is the player's highest weapon of the chain that starts with
// (or holds) id, or "".
func (c *pcCatalogs) ownedWeapon(u *store.UserState, id int32) string {
	base := c.chainBase(id)
	best, bestLevel := "", int32(-1)
	for key, w := range u.Weapons {
		if c.chainBase(w.WeaponId) == base && w.Level > bestLevel {
			best, bestLevel = key, w.Level
		}
	}
	return best
}

// setCostumeLevel sets a costume's level (-1: its highest), ascending it as
// far as the level needs. Reports whether it changed.
func (c *pcCatalogs) setCostumeLevel(u *store.UserState, key string, want int32, now int64) bool {
	row := u.Costumes[key]
	cm, ok := c.costumes.Costumes[row.CostumeId]
	if !ok {
		return false
	}
	maxFunc, ok := c.costumes.MaxLevelByRarity[cm.RarityType]
	if !ok {
		return false
	}
	bonus := costumeRebirthBonus(c.rebirth, cm.CharacterId, u.CharacterRebirths[cm.CharacterId].RebirthCount)
	capAt := func(lb int32) int32 { return maxFunc.Evaluate(lb) + bonus }
	lb := row.LimitBreakCount
	if want < 0 || want > capAt(massCostumeLimitBreakMax) {
		want = capAt(massCostumeLimitBreakMax)
	}
	for lb < massCostumeLimitBreakMax && capAt(lb) < want {
		lb++
	}
	if want < 1 {
		want = 1
	}
	level, exp := want, row.Exp
	if th, ok := c.costumes.ExpByRarity[cm.RarityType]; ok && len(th) > 0 {
		exp = th[min(int(want), len(th)-1)]
		level, exp = gameutil.LevelAndCap(exp, th)
		level = min(level, want)
	}
	if level == row.Level && lb == row.LimitBreakCount && exp == row.Exp {
		return false
	}
	row.Level, row.Exp, row.LimitBreakCount, row.LatestVersion = level, exp, lb, now
	u.Costumes[key] = row
	return true
}

// setWeaponLevel sets a weapon's level (-1: its highest in its current form),
// ascending it as far as the level needs. Reports whether it changed.
func (c *pcCatalogs) setWeaponLevel(u *store.UserState, key string, want int32, now int64) bool {
	row := u.Weapons[key]
	wm, ok := c.weapons.Weapons[row.WeaponId]
	if !ok {
		return false
	}
	maxFunc, ok := c.weapons.MaxLevelByEnhanceId[wm.WeaponSpecificEnhanceId]
	if !ok {
		return false
	}
	extra := int32(0)
	if aw, ok := c.weapons.AwakenByWeaponId[row.WeaponId]; ok {
		if _, refined := u.WeaponAwakens[key]; refined {
			extra = aw.LevelLimitUp
		}
	}
	capAt := func(lb int32) int32 { return maxFunc.Evaluate(lb) + extra }
	lb := row.LimitBreakCount
	if want < 0 || want > capAt(massWeaponLimitBreakMax) {
		want = capAt(massWeaponLimitBreakMax)
	}
	for lb < massWeaponLimitBreakMax && capAt(lb) < want {
		lb++
	}
	if want < 1 {
		want = 1
	}
	level, exp := want, row.Exp
	if th, ok := c.weapons.ExpByEnhanceId[c.weapons.LevelingEnhanceIdByWeaponId[row.WeaponId]]; ok && len(th) > 0 {
		exp = th[min(int(want), len(th)-1)]
		level, exp = gameutil.LevelAndCap(exp, th)
		level = min(level, want)
	}
	if level == row.Level && lb == row.LimitBreakCount && exp == row.Exp {
		return false
	}
	row.Level, row.Exp, row.LimitBreakCount, row.LatestVersion = level, exp, lb, now
	u.Weapons[key] = row
	note := u.WeaponNotes[row.WeaponId]
	if note.WeaponId == 0 {
		note.WeaponId = row.WeaponId
		note.FirstAcquisitionDatetime = now
	}
	note.MaxLevel = max(note.MaxLevel, level)
	note.MaxLimitBreakCount = max(note.MaxLimitBreakCount, lb)
	note.LatestVersion = now
	u.WeaponNotes[row.WeaponId] = note
	checkWeaponStoryUnlocksMass(c.weapons, u, row.WeaponId, level, now)
	return true
}

var _ = model.PossessionTypeCostume
