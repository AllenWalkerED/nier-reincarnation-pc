"""Lifts the game's 30 fps ceiling (libil2cpp, Global 3.7.1): the three places
that fix 30 take 240, and win.apk's WINAPK_FPS_CAP (the launcher's frame
rate) decides the real rate at each frame.

  Dark.Entrypoint.Generator.OnlyWhenStarting   Application.targetFrameRate = 30
  Dark.Conductor.UserInterface.QualitySetup    Application.targetFrameRate = <its asset's value>
  Dark.Component.PerformanceSetting.Config     UnSavePowerTargetRenderFrameRate = 30
                                               (renderFrameInterval = targetFrameRate / this)

Low and Power saving take SavePowerTargetRenderFrameRate (10) here, which at
240 drew one frame in 24; the runtime raises their rate to the others' once
the game runs (game_effects.cpp, "Low and Power saving at the full frame
rate"), which leaves this library, and so its translation, as it is.

usage: python fps_patch.py <libil2cpp.so>     (keeps <file>.orig; idempotent)
"""
import shutil
import struct
import sys
from pathlib import Path

CEILING = 240
MOVZ_W0 = 0x52800000 | (CEILING << 5)       # movz w0, #240
MOVZ_W9 = 0x52800009 | (CEILING << 5)       # movz w9, #240
PATCHES = (
    (0x2E5BE0C, 0x321F0FE0, MOVZ_W0),          # orr w0, wzr, #0x1e
    (0x29E4640, 0xB9405260, MOVZ_W0),          # ldr w0, [x19, #0x50]
    (0x2C7AC80, 0x321F0FE9, MOVZ_W9),          # orr w9, wzr, #0x1e
)


def patch(data: bytearray) -> int:
    done = 0
    for offset, old, new in PATCHES:
        word = struct.unpack_from("<I", data, offset)[0]
        if word == new:
            continue
        if word != old:
            raise SystemExit(f"0x{offset:x}: 0x{word:08x}, not 0x{old:08x}: another game version")
        struct.pack_into("<I", data, offset, new)
        done += 1
    return done


def main():
    path = Path(sys.argv[1])
    data = bytearray(path.read_bytes())
    n = patch(data)
    if n:
        backup = path.with_name(path.name + ".orig")
        if not backup.exists():
            shutil.copy2(path, backup)
        path.write_bytes(data)
    print(f"{path.name}: {n} patched" if n else f"{path.name}: already patched")


if __name__ == "__main__":
    main()
