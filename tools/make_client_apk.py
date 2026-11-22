"""Builds the loopback client: the original 3.7.1 APK with Bunker's checked patches.

Only four entries change; every other entry is copied byte for byte:
  - lib/arm64-v8a/libil2cpp.so: game_patch (plain gRPC to 127.0.0.1:8003, no
    request encryption, offline reachability, ...) and privacy (telemetry off)
  - global-metadata.dat: the API, web, master-data and resource hosts -> 127.0.0.1
  - globalgamemanagers: Unity Connect analytics off
  - lib/arm64-v8a/libil2cpp.so also: the game's 30 fps ceiling lifted (fps_patch.py);
    win.apk's WINAPK_FPS_CAP sets the rate
  - lib/arm64-v8a/libunity.so: presents frames with eglSwapBuffers instead of
    Swappy (Optimized Frame Pacing), whose swap fails on win.apk's EGL; Unity
    then recreated its window surface every frame and never showed one
The manifest and the old signature files are left as they are (win.apk does
not enforce them).

usage: python make_client_apk.py <original.apk> <output.apk>
Run with PYTHONUTF8=1.
"""
import hashlib
import shutil
import sys
import tempfile
import zipfile
from pathlib import Path

TOOLS = Path(__file__).resolve().parents[1] / "third_party" / "bunker" / "android" / "tools"
sys.path.insert(0, str(TOOLS))
import assemble_apk  # noqa: E402
import game_patch  # noqa: E402
import privacy  # noqa: E402

sys.path.insert(0, str(Path(__file__).resolve().parent))
import fps_patch  # noqa: E402

LIB, METADATA, SETTINGS = assemble_apk.LIB, assemble_apk.METADATA, assemble_apk.SETTINGS
UNITY = "lib/arm64-v8a/libunity.so"
UNITY_SHA256 = "3bc0bb2434171854839f9e4da00504f4ff413e0237ecea80b8a1cbbf6c8d3c9f"  # 3.7.1, Unity 2019.4.29f1
# In the swap helper at 0x412a00: "cbz w8, 0x412a40" (frame pacing off?) becomes
# "b 0x412a40", the plain eglSwapBuffers call. Text: file offset == address.
UNITY_PATCHES = ((0x412a1c, 0x34000128, 0x14000009),)


def patch_unity(data):
    if hashlib.sha256(data).hexdigest() != UNITY_SHA256:
        raise RuntimeError("Unsupported libunity.so: expected the 3.7.1 one")
    data = bytearray(data)
    for offset, old, new in UNITY_PATCHES:
        if int.from_bytes(data[offset:offset + 4], "little") != old:
            raise RuntimeError(f"libunity.so: unexpected instruction at 0x{offset:x}")
        data[offset:offset + 4] = new.to_bytes(4, "little")
    return bytes(data)


def main():
    source_path, output_path = Path(sys.argv[1]), Path(sys.argv[2])
    output_path.parent.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(source_path) as game, tempfile.TemporaryDirectory(dir=output_path.parent) as work:
        work = Path(work)
        lib, metadata = assemble_apk.patch_game_files(game, work)
        code = bytearray(lib.read_bytes())  # the 30 fps ceiling lifted (fps_patch.py)
        fps_patch.patch(code)
        lib.write_bytes(code)
        settings = assemble_apk.patch_unity_connect(game.read(SETTINGS))
        unity = patch_unity(game.read(UNITY))
        replaced = {LIB: lib, METADATA: metadata}
        with open(source_path, "rb") as raw, open(output_path, "wb") as out:
            writer = assemble_apk.ApkWriter(out)
            for info in game.infolist():
                name = info.filename
                if name in replaced:
                    writer.add_file(name, replaced[name], compress=info.compress_type != 0)
                elif name == SETTINGS:
                    writer.add(name, settings, compress=info.compress_type != 0)
                elif name == UNITY:
                    writer.add(name, unity, compress=info.compress_type != 0)
                else:
                    writer.copy(raw, info)
            writer.close()
    with zipfile.ZipFile(output_path) as check:
        bad = check.testzip()
        if bad:
            raise RuntimeError(f"Corrupt entry in the output: {bad}")
        digest = hashlib.sha256(check.read(LIB)).hexdigest()
    print(f"Wrote {output_path} ({output_path.stat().st_size / 1e6:.1f} MB); patched libil2cpp sha256 {digest}")


if __name__ == "__main__":
    main()
