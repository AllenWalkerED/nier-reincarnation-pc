# The server of the PC port

This folder is **[Lunar Tear](https://github.com/Walter-Sparrow/lunar-tear)**, the fan-made server
that brings NieR Re[in]carnation back after its shutdown, with the modifications
[Bunker](https://github.com/veritr1x/bunker) makes to it for its Android and iOS app, and three
additions of this port for Windows. The server is Lunar Tear's work; Bunker's part is changes on top.

## Where it comes from

| | |
|---|---|
| Bunker | [veritr1x/bunker](https://github.com/veritr1x/bunker) `07dfecc3a77acc0fa6cdc3f841328c34b50c5bec` (version 1.12.7): `server/` as `scripts/prepare.py` makes it |
| Lunar Tear | [Walter-Sparrow/lunar-tear](https://github.com/Walter-Sparrow/lunar-tear) `63df7d70556a7180f04312abbafc56494cf70545` (Bunker's pin), with Bunker's `overlays/server` and `patches/` |
| Lunar Base | [NMeliksah/lunar-base](https://github.com/NMeliksah/lunar-base) `b39154099b8f09f5926239e5838abc37260b9b9b`: `internal/lunarbase` (the grant tool) |
| Comfort Mod | [NavHobbyDev/lunar-tear-comfort-mod](https://github.com/NavHobbyDev/lunar-tear-comfort-mod): most of `internal/mods` |

`gen/proto` is generated from `proto/` (`make proto`, with protoc and protoc-gen-go /
protoc-gen-go-grpc), as Bunker's Makefile does.

## What Bunker's changes add, and why the PC port needs them

Plain Lunar Tear opens a summon only between its original dates. The game ended in April 2024,
so today none is open and the summon page hangs. Bunker's summon catalog keeps the permanent and
chapter summons open (153 of 592 on 19 November 2026, against 0 of 160). It also answers calls
Lunar Tear left unanswered (friends and others), adds shop purchases to the inventory, pays
missions, and has the mods (`internal/mods`).

## The additions for Windows

- `cmd/nier-server`: runs `mobile.Start` (the game on 8003, the assets on 8080, the accounts on
  3000, in one process, set up as the app sets them up) with the save folder and the game data
  folder. It copies the launcher's `auth.secret` to `auth.key`, and sets `<save>\backups` as the
  folder for the copy of the save taken before a new version updates it.
- `mobile/sys_unix.go`, `mobile/sys_windows.go`, `mobile/archive.go`: the two Unix calls of the
  phone app's archive import (`syscall.Sync`, `syscall.Dup`) behind `syncStorage` and `dupFD`, so
  the package builds on Windows. The Android and iOS builds are unchanged.
- `cmd/migrate` and `cmd/probe` (from version 1.3): a save migration and a check of the server's
  first calls, without the game.

## Save files and the save editor (launcher: Settings > Saves)

`nier-server --save <save> --data <data> --admin` reads one JSON request on stdin, writes one JSON
reply and exits without starting the server (`internal/pcsave`). The launcher uses it for its Saves
page. It refuses every change while 127.0.0.1:8003 is held (the game is open).

- Slots: `<save>\game.db` is the slot in use (the server's save, unchanged); the others are
  `<save>\slots\<id>\game.db`, listed in `<save>\slots.json`. A slot with no game.db is a new game:
  the server creates the database and, at the first sign-in, a new player for this PC's ID (Auth's
  "new player" path). auth.*, mods.json and the rest stay shared: they belong to the PC. Switching
  settles both databases (WAL checkpoint), swaps the files and gives the chosen save's player this
  PC's ID if it has another (as Bunker's Import does).
- Before every edit, switch and delete, the slot is copied (VACUUM INTO) to `<save>\slot-backups`
  (the newest 30 are kept); a deleted slot leaves its copy there.
- Requests: `list`, `new` (name, switch), `switch`, `rename`, `delete`, `backup`, `read` (the
  editor's lists: names from the game's English text bundles, owned state from the save), `apply`.
- `apply` is `lunarbase.PCApply` (`internal/lunarbase/pc_editor.go`): it grants costumes, weapons
  (by evolution chain, first form) and companions with the server's granter, sets their level
  (ascending as far as the level needs, no materials taken; weapon notes and story unlocks as
  `upgrade_all_weapons`), and sets gems, gold, materials and items to amounts within the game's
  limits, in one UpdateUser transaction. Nothing owned is removed.

## Open summons and events (launcher: the Events page)

The player chooses which summons and events are open; **nothing is open until they choose**,
except the game's permanent summons, which are always open.
The choice is `<save>\active-content.json` (`internal/pccontent`):

```
{"banners": ["limited_59", "limited_612", ...], "events": [11, 509, ...]}
```

- Summons are chosen by banner (`BannerAssetName`, or `gacha_<id>` without one), so a summon's
  reruns, which share its banner, open with it. `nier-server` calls `pccontent.Use(<save>)`;
  then `gachaActiveAt` (the summon list, Mama's banners, `GetGacha`) and `Draw` leave out every
  summon not chosen (`Draw` answers FailedPrecondition). The Portal Cage's daily draw and the
  Recycle summon are not in the list and are not affected. The file is read again when it
  changes (within a second), so summons chosen while the game runs show the next time Summons
  is opened. Without `Use` (Bunker's own builds, the tests) every summon is open by its dates.
- Always open, whatever the file says (`pccontent.AlwaysOpen`, checked by the server, so editing
  the file cannot close them): the game has no single standard premium summon (every premium
  summon in its data had dates), so these are its permanent summons: the **chapter summons**
  (label Chapter: `chapter_1`..`chapter_24`, ids 2100002-2100030, and Dark Memory `chapter_ex`,
  2100000; the only summons the game data gives no dates, still shown only once their chapter
  is cleared) and the **ticket summons** (`confirm_*`, 970001-970009: the guaranteed ★4/★3 and
  element ticket summons, where the summon tickets the game gives are used; Bunker lists them
  as always-open ticket summons). The launcher shows them as "Always on" with the toggle off.
- Events are event quest chapters. `tools\patch_content.py --selection <save>\active-content.json`
  (the launcher runs it before each start) gives the patcher only the chosen chapters
  (`--event-quests`, empty: every event closed); it patches again only when the choice changed
  (`content-patch.json` keeps a hash of the arguments). The shops and the summon banners
  (`--banners`, `m_mom_banner`) stay as the Full preset has them: the server builds its summon
  catalog from those banner rows' dates, so expiring the banners of summons not chosen (an
  earlier version did) dropped them from the catalog and from the launcher's list.
- `--admin` requests (fine while the game runs): `content-list` (the summons the catalog can
  open today, one per banner, with their names, featured characters, picture keys and
  `always`; the event chapters of the patcher's config, with their names and picture keys),
  `content-set` (writes the file).

## Limited-time sale notices (launcher: Settings > Mods, on by default)

The home screen announces every limited-time offer that is open ("The Sun VI Pack on sale for a
limited time. Ends In: 640866:10"): `HomeScreenPresenter.PlayAchievementLimited` toasts each open
`IUserLimitedOpen` row each time the home screen opens. The content patch moves the offers' cell
terms to 2099, and `userdata`'s `IUserLimitedOpen` (`masterdata.LimitedOffers`, from
`m_shop_item_cell_limited_open`) then opens all 34 of them, so the toasts never end. The shop
they belong to (Limited shop 1901) is closed by the patch's "Full" preset, so they could not be
bought anyway.

`save\mods.json` `"hideSaleNotices"` (not a server mod; on unless `false`) makes
`tools\patch_content.py` add `--plain-limited-offers` (a PC-port addition to
`masterdata_patcher\patch_masterdata.py`): `m_shop_item_cell_limited_open` is emptied and
`m_shop.LimitedOpenId` cleared on the shops holding those cells (none today). The server reads the
same patched game data, so `LimitedOffers` is empty and `IUserLimitedOpen` is `[]`; the shop
catalog is otherwise unchanged (10,032 items either way). Turning it off patches again from
`origin.bin.e` without the option, at the next start.

## Pictures from the player's game files

`--admin` `{"op":"images","keys":["banner/limited_59","costume/ch001001",...]}`
(`internal/pcsave/images.go`) makes PNGs from the game's own asset bundles (Bunker's
`mobile/unityasset`: unmask, LZ4, ASTC/ETC decoding) into `launcher\ui\game\icons\<kind>\<name>.png`
beside the data folder, which the launcher's page shows as `game/icons/...`. Nothing of the game
ships with the port. Pictures made before are kept. Kinds: `banner` (`ui/gacha/en/<asset>/banner`),
`event`, `eventlist`, `eventmom` (the event banners in `ui/quest/en`), `costume`, `weapon`,
`companion`, `material`, `consumable` (their `_standard` icons). The save editor's lists
(`read`) and `content-list` carry each item's picture key (`img`) when the game files have the
bundle. On the test PC all 2,869 pictures took 7.5 s the first time (62 MB).

## Building

```
cd server
make proto            (or the protoc line in the Makefile)
go build -trimpath -ldflags "-s -w -X lunar-tear/server/internal/bunkerversion.Version=1.12.7-pc1.4" -o nier-server.exe ./cmd/nier-server
```

The version (Bunker's, then the port's) is what the server records beside the save
(`bunker.version`); when it changes, the save is copied to `<save>\backups` before it is updated.
Raise it with each release.

The launcher starts `server\bin\nier-server.exe --save <save> --data <data>`.
