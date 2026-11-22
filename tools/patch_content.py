"""Opens the game's shops and events again (NieR.exe runs it before each start).

The master data still carries the game's original dates, and the game ended in
April 2024, so the client shows almost nothing in the shop. This applies the
"Full" preset of lunar-tear-masterdata-patcher (masterdata_patcher\\, from
Walter Sparrow's lunar-scripts, as the Bunker app applies it on a phone) to the
player's master data:

  data\\assets\\release\\origin.bin.e           the player's master data, kept as it was
  data\\assets\\release\\20240404193219.bin.e   the patched one the server serves
  data\\assets\\release\\content-patch.json     what made it

With --selection (the launcher's Settings > Events, save\\active-content.json)
only the events the player chose are opened (the summons are opened and closed
by the server; their banners stay as the Full preset has them). Nothing is chosen until they choose: a
missing file opens no event. The shops are opened either way. With
--selection, "hideSaleNotices" in mods.json beside it (the launcher's Mods ›
Hide limited-time sale notices; on unless false) also sells the limited-time
offers as ordinary items (patch_masterdata.py --plain-limited-offers), which
ends the home screen's endless "on sale for a limited time" messages. Without
--selection every event of the "Full" preset is opened, as before.

It runs when something changed: when content-patch.json names this patcher,
origin.bin.e and the same choice, it does nothing. Lines on stdout:
"note:<text>" and, at the end, "done" or "error:<text>" (exit code 1).

    python patch_content.py --data <the port's data folder> [--selection <active-content.json>]
"""
import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
PATCHER = HERE / "masterdata_patcher"
MASTER = "20240404193219.bin.e"
ORIGIN = "origin.bin.e"
MARKER = "content-patch.json"
PRESET = "Full"


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def get(cfg: dict, key: str) -> str:
    """A config value as text; a list of keys (*_ALL) is joined (as run_patches.py does)."""
    val = cfg.get(key, "")
    if isinstance(val, list):
        return ", ".join(p for p in (get(cfg, k) for k in val) if p)
    return str(val).strip() if val else ""


def id_list(text: str) -> list:
    """The patcher's id syntax ("1, 2, 10-20") as ids, in order, without repeats."""
    out, seen = [], set()
    for part in text.replace(";", ",").split(","):
        part = part.strip()
        if not part:
            continue
        lo, _, hi = part.partition("-")
        try:
            ids = range(int(lo), int(hi) + 1) if hi else (int(lo),)
        except ValueError:
            continue
        for i in ids:
            if i not in seen:
                seen.add(i)
                out.append(i)
    return out


def hide_sale_notices(mods: Path) -> bool:
    """The launcher's Mods › Hide limited-time sale notices (save\\mods.json
    "hideSaleNotices"; on unless it is false). On, the limited-time offers are
    sold as ordinary shop items, so the home screen stops announcing them
    (patch_masterdata.py --plain-limited-offers)."""
    try:
        return json.loads(mods.read_text(encoding="utf-8")).get("hideSaleNotices", True) is not False
    except (OSError, ValueError, AttributeError):
        return True


def read_selection(path: Path) -> dict:
    """The launcher's choice; a missing or broken file chooses nothing."""
    try:
        sel = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        return {"events": []}
    except (OSError, ValueError) as e:
        print(f"note:{path.name} could not be read ({e}): no event is opened.")
        return {"events": []}
    return {"events": [int(i) for i in (sel.get("events") or []) if isinstance(i, int)]}


def full_arguments(cfg: dict, selection=None) -> list:
    """run_patches.py's task 1, "Single file with ALL content"; with a selection,
    only the chosen events (in the config's order).

    The summon banners (m_mom_banner) are always the Full preset's: the server
    builds its summon catalog from those rows' dates, so expiring the banners
    of summons not chosen would drop them from the catalog for good (the
    launcher could no longer offer them). The server closes the summons not
    chosen itself."""
    events = ", ".join(get(cfg, k) for k in ("EVENTS_MAIN", "EVENTS_REC_ALL", "EVENTS_VAR_ALL", "EVENTS_BONUS_ALL"))
    banners = f'{get(cfg, "GACHA_PREM_ALL")}, {get(cfg, "GACHA_EVENT")}'
    if selection is not None:
        chosen = set(selection["events"])
        events = ", ".join(str(i) for i in id_list(events) if i in chosen)
    shops = ", ".join(get(cfg, k) for k in ("SHOPS_MAIN", "SHOPS_EXTRA", "SHOPS_GACHA_ALL", "SHOPS_EVENT_ALL"))
    content = f'{get(cfg, "SHOPCONTENT_COMP")}, {get(cfg, "SHOPCONTENT_MEDAL")}'
    return ["--event-quests", events, "--banners", banners, "--premium-shops", get(cfg, "SHOP_PREM"),
            "--exchange-shops", shops, "--exclude-consumables", get(cfg, "CONSUMABLES_ALL"),
            "--rewrite-shop-content", content, "--rewrite-shop-count", get(cfg, "SHOPCOUNT"),
            "--rewrite-shop-type", get(cfg, "SHOPTYPE")]


def main() -> int:
    p = argparse.ArgumentParser()
    p.add_argument("--data", type=Path, required=True)
    p.add_argument("--selection", type=Path, help="the launcher's active-content.json (Settings > Events)")
    a = p.parse_args()
    release = a.data / "assets" / "release"
    master, origin, marker = release / MASTER, release / ORIGIN, release / MARKER
    if not master.exists() and not origin.exists():
        print(f"error:The master data is missing ({master}).")
        return 1

    patcher_id = sha256(PATCHER / "patch_masterdata.py") + ":" + sha256(PATCHER / "config.json")
    cfg = json.loads((PATCHER / "config.json").read_text(encoding="utf-8"))
    selection = read_selection(a.selection) if a.selection else None
    arguments = full_arguments(cfg, selection)
    if selection is not None and hide_sale_notices(a.selection.parent / "mods.json"):
        arguments.append("--plain-limited-offers")
    preset = PRESET if selection is None else PRESET + ", chosen events"
    chosen = hashlib.sha256(json.dumps(arguments).encode("utf-8")).hexdigest()
    if marker.exists() and origin.exists() and master.exists():
        try:
            done = json.loads(marker.read_text(encoding="utf-8"))
            if done.get("patcher") == patcher_id and done.get("preset") == preset and done.get("arguments", chosen) == chosen:
                print("done")
                return 0
        except (OSError, ValueError):
            pass

    # The player's own file is kept once; every patch starts from it.
    if not origin.exists():
        shutil.copy2(master, origin)
        print(f"note:Your master data is kept as {ORIGIN}.")
    if selection is None:
        print("note:Opening the shops and events (lunar-tear-masterdata-patcher, Full)...")
    else:
        n = len([i for i in arguments[1].split(",") if i.strip()])
        print(f"note:Opening the shops and the {n} chosen events (lunar-tear-masterdata-patcher)...")
    out = release / (MASTER + ".new")
    cmd = [sys.executable, str(PATCHER / "patch_masterdata.py"), "--input", str(origin), "--output", str(out)]
    env = dict(os.environ, PYTHONUTF8="1", PYTHONIOENCODING="utf-8")
    run = subprocess.run(cmd + arguments, cwd=PATCHER, env=env, capture_output=True, text=True,
                         encoding="utf-8", errors="replace")
    if run.returncode != 0 or not out.exists():
        out.unlink(missing_ok=True)
        tail = (run.stderr or run.stdout or "").strip().splitlines()[-1:] or ["no output"]
        print(f"error:The shop patch failed: {tail[0]}")
        return 1
    os.replace(out, master)
    marker.write_text(json.dumps({"preset": preset, "patcher": patcher_id, "origin": sha256(origin), "arguments": chosen},
                                 indent=2) + "\n", encoding="utf-8")
    print("note:The shops and the chosen events are open." if selection is not None else "note:The shops and events are open.")
    print("done")
    return 0


if __name__ == "__main__":
    sys.exit(main())
