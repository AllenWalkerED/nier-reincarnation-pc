"""Assembles the release folder, ready to zip:

  NieR.exe, WebView2Loader.dll     the launcher (launcher/build.sh)
  launcher\\ui                      its page (the game's art is made at setup)
  runtime\\                         the win.apk runtime, built with winapk/winapk-nier.patch:
                                   bin, lib, jre, clang (the translator's compiler)
  server\\bin                       Lunar Tear (with Bunker's changes) for Windows: nier-server (server/PC-PORT.md)
  tools\\                           setup_game.py and the game patchers, patch_content.py
                                   with masterdata_patcher (the shops and events), Bunker's
                                   APK tools, a Python with their packages, 7-Zip
  licenses\\                        every component's license
  README.md, LICENSE, NOTICE.md

Nothing of the game is in it: the player's own files are prepared at the
first start (tools/setup_game.py).

usage: python scripts/build_dist.py --out <folder> --runtime <built win.apk folder: bin, lib, jre, clang>
       --server-bin <folder with nier-server.exe> --launcher-bin <folder with NieR.exe> --python-embed <zip>
       --site-packages <folder> --sevenzip <folder with 7za.exe>
"""
import argparse
import shutil
from pathlib import Path

SRC = Path(__file__).resolve().parents[1]


def copytree(src: Path, dst: Path, ignore=None):
    shutil.copytree(src, dst, dirs_exist_ok=True, ignore=ignore)


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--out", type=Path, required=True)
    p.add_argument("--runtime", type=Path, required=True)
    p.add_argument("--server-bin", type=Path, required=True)
    p.add_argument("--launcher-bin", type=Path, required=True)
    p.add_argument("--python-embed", type=Path, required=True)
    p.add_argument("--site-packages", type=Path, required=True)
    p.add_argument("--sevenzip", type=Path, required=True)
    a = p.parse_args()
    out = a.out
    out.mkdir(parents=True, exist_ok=True)

    # The launcher.
    for name in ("NieR.exe", "WebView2Loader.dll"):
        shutil.copy2(a.launcher_bin / name, out / name)
    copytree(SRC / "launcher" / "ui", out / "launcher" / "ui", ignore=shutil.ignore_patterns("game"))
    # The game window's cursor (an original design: launcher/cursors/make_cursor.py).
    (out / "cursors").mkdir(exist_ok=True)
    shutil.copy2(SRC / "launcher" / "cursors" / "nier.cur", out / "cursors" / "nier.cur")

    # The runtime: what runs an app, not win.apk's own launcher or its logo.
    rt = out / "runtime"
    for part in ("bin", "lib", "jre", "clang"):
        copytree(a.runtime / part, rt / part, ignore=shutil.ignore_patterns("winapk.png"))

    # The server.
    (out / "server" / "bin").mkdir(parents=True, exist_ok=True)
    for exe in ("nier-server.exe",):
        shutil.copy2(a.server_bin / exe, out / "server" / "bin" / exe)

    # Setup and the game patchers, with what they need.
    tools = out / "tools"
    tools.mkdir(exist_ok=True)
    for name in ("setup_game.py", "make_client_apk.py", "fps_patch.py", "hide_slidepad.py", "patch_content.py"):
        shutil.copy2(SRC / "tools" / name, tools / name)
    copytree(SRC / "tools" / "masterdata_patcher", tools / "masterdata_patcher", ignore=shutil.ignore_patterns("__pycache__"))
    copytree(SRC / "tools" / "runtime-overlay", tools / "runtime-overlay")
    copytree(SRC / "third_party" / "bunker", out / "third_party" / "bunker",
             ignore=shutil.ignore_patterns("__pycache__"))
    py = tools / "python"
    shutil.unpack_archive(a.python_embed, py)
    copytree(a.site_packages, py / "Lib" / "site-packages", ignore=shutil.ignore_patterns("__pycache__"))
    # UnityPy imports fmod_toolkit, which bundles FMOD (not ours to ship): a stand-in.
    copytree(SRC / "tools" / "fmod_toolkit_stub", py / "Lib" / "site-packages" / "fmod_toolkit")
    pth = next(py.glob("python3*._pth"))
    pth.write_text(pth.read_text().replace("#import site", "import site") + "Lib\\site-packages\n", encoding="utf-8")
    (tools / "7zip").mkdir(exist_ok=True)
    for name in ("7za.exe", "7za.dll", "7zxa.dll"):
        if (a.sevenzip / name).exists():
            shutil.copy2(a.sevenzip / name, tools / "7zip" / name)

    # Licenses and the documents.
    lic = out / "licenses"
    copytree(SRC / "licenses", lic)
    for name in ("README.md", "LICENSE", "NOTICE.md"):
        shutil.copy2(SRC / name, out / name)
    for d in ("data", "save", "logs"):
        (out / d).mkdir(exist_ok=True)
    print(f"Assembled {out}")


if __name__ == "__main__":
    main()
