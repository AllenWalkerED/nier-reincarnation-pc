"""Hides the on-screen movement stick (the "slide pad") in the client APK.

On a computer the stick is only in the way: movement comes from WASD and the
controller (win.apk's input map), which still drive it as a finger would.
Its pictures become transparent; nothing else changes:

  common_atlas      parts_virtual_pad_in_base, _run_base, _dot, _direction:
                    the pixels inside each sprite's own outline (its mesh;
                    tight packing puts other sprites in a circle's corners)
  turnbattle_atlas  slide_pad_lock (the stick's lock, in battles)
  parts_virtual_pad_walk_base   its own texture, all of it

Those textures are ASTC. They are decoded here with the block size their
format names (UnityPy's own decode of the 6x6 atlas is wrong: it would ruin
it), cleared, and stored as RGBA32, so every other pixel stays exactly as the
GPU decoded it. The prefab, its animations and its touch handling stay.

The APK is written back into the same file (win.apk's installed base.apk is a
hard link to it) and keeps its modification time, so win.apk does not install
it again (it would extract the native libraries anew).

usage: python hide_slidepad.py <client.apk> [--check <dir>]
  (keeps <client.apk>.slidepad.orig; --check also saves before/after crops)
"""
import os
import shutil
import sys
import tempfile
import zipfile
from pathlib import Path

import texture2ddecoder
import UnityPy
from PIL import Image, ImageDraw, ImageFilter
from UnityPy.enums import TextureFormat
from UnityPy.export.SpriteHelper import MeshHandler

TOOLS = Path(__file__).resolve().parents[1] / "third_party" / "bunker" / "android" / "tools"
sys.path.insert(0, str(TOOLS))
import assemble_apk  # noqa: E402

DATA = "assets/bin/Data/"
ATLAS_SPRITES = {
    "parts_virtual_pad_in_base", "parts_virtual_pad_run_base", "parts_virtual_pad_dot",
    "parts_virtual_pad_direction", "slide_pad_lock",
}
WHOLE_TEXTURES = {"parts_virtual_pad_walk_base"}
ASTC_BLOCKS = {48: 4, 49: 5, 50: 6, 51: 8, 52: 10, 53: 12, 54: 4, 55: 5, 56: 6, 57: 8, 58: 10, 59: 12}


def load_data(apk: zipfile.ZipFile, work: Path) -> Path:
    data = work / "Data"
    for name in apk.namelist():
        if name.startswith(DATA) and "/Managed/" not in name:
            path = data / name[len(DATA):]
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(apk.read(name))
    return data


def decode(texture) -> Image.Image:
    """The texture as RGBA, rows bottom-up (Unity's order)."""
    fmt = int(texture.m_TextureFormat)
    if fmt not in ASTC_BLOCKS:
        raise SystemExit(f"{texture.m_Name}: format {fmt}, not ASTC")
    b = ASTC_BLOCKS[fmt]
    w, h = texture.m_Width, texture.m_Height
    size = ((w + b - 1) // b) * ((h + b - 1) // b) * 16
    data = bytes(texture.image_data)
    if len(data) < size:
        raise SystemExit(f"{texture.m_Name}: {len(data)} bytes, {size} expected")
    bgra = texture2ddecoder.decode_astc(data[:size], w, h, b, b)
    return Image.frombytes("RGBA", (w, h), bgra, "raw", "BGRA")


def store(texture, image: Image.Image):
    """RGBA32, one level, inside the serialized file."""
    data = image.tobytes("raw", "RGBA")
    texture.m_TextureFormat = TextureFormat.RGBA32
    texture.m_MipCount = 1
    texture.m_CompleteImageSize = len(data)
    texture.image_data = data
    if texture.m_StreamData:
        texture.m_StreamData.offset = 0
        texture.m_StreamData.size = 0
        texture.m_StreamData.path = ""
    texture.save()


def outline(sprite, version, rect, size) -> Image.Image:
    """The sprite's mesh drawn where it sits in the atlas (bottom-up), a pixel wider."""
    mesh = MeshHandler(sprite.m_RD, version)
    mesh.process()
    points = mesh.m_Vertices
    min_x = min(x for x, _y, _z in points)
    min_y = min(y for _x, y, _z in points)
    ppu = sprite.m_PixelsToUnits
    xy = [(rect.x + (x - min_x) * ppu, rect.y + (y - min_y) * ppu) for x, y, _z in points]
    mask = Image.new("L", size, 0)
    draw = ImageDraw.Draw(mask)
    for sub in mesh.get_triangles():
        for a, b, c in sub:
            draw.polygon([xy[a], xy[b], xy[c]], fill=255)
    return mask.filter(ImageFilter.MaxFilter(3))  # its soft edge too


def find_atlas_regions(env):
    """{(file, path id): (texture object, [(sprite name, sprite, its object version, its atlas rect)])}"""
    regions = {}
    sprites = {}
    for obj in env.objects:
        if obj.type.name == "Sprite" and obj.peek_name() in ATLAS_SPRITES:
            sprites[obj.peek_name()] = obj
    for obj in env.objects:
        if obj.type.name != "SpriteAtlas":
            continue
        atlas = obj.read()
        for name in atlas.m_PackedSpriteNamesToIndex:
            if name not in ATLAS_SPRITES or name not in sprites:
                continue
            sprite_obj = sprites[name]
            sprite = sprite_obj.read()
            for key, rd in atlas.m_RenderDataMap:
                if key != sprite.m_RenderDataKey:
                    continue
                texture_obj = rd.texture.deref()
                k = (texture_obj.assets_file.name, texture_obj.path_id)
                regions.setdefault(k, (texture_obj, []))[1].append((name, sprite, sprite_obj.version, rd.textureRect))
    return regions


def main():
    apk_path = Path(sys.argv[1])
    check = Path(sys.argv[sys.argv.index("--check") + 1]) if "--check" in sys.argv else None
    log = print
    stat = apk_path.stat()
    with tempfile.TemporaryDirectory(dir=apk_path.parent, ignore_cleanup_errors=True) as work:
        work = Path(work)
        with zipfile.ZipFile(apk_path) as apk:
            data = load_data(apk, work)
        env = UnityPy.load(str(data))
        changed = {}
        for texture_obj, regions in find_atlas_regions(env).values():
            texture = texture_obj.read()
            before = decode(texture)
            image = before.copy()
            for name, sprite, version, rect in regions:
                mask = outline(sprite, version, rect, image.size)
                image.paste((0, 0, 0, 0), (0, 0, *image.size), mask)
                log(f"  {texture.m_Name}: {name} cleared ({int(rect.width)}x{int(rect.height)} at {int(rect.x)},{int(rect.y)})")
                if check:
                    check.mkdir(parents=True, exist_ok=True)
                    box = (int(rect.x) - 24, int(rect.y) - 24, int(rect.x + rect.width) + 24, int(rect.y + rect.height) + 24)
                    pair = Image.new("RGBA", ((box[2] - box[0]) * 2 + 8, box[3] - box[1]), (255, 0, 255, 255))
                    for i, img in enumerate((before, image)):
                        crop = img.crop(box).transpose(Image.FLIP_TOP_BOTTOM)
                        bg = Image.new("RGBA", crop.size, (70, 120, 70, 255))
                        bg.alpha_composite(crop)
                        pair.paste(bg, (i * (crop.width + 8), 0))
                    pair.save(check / f"{name}.png")
            store(texture, image)
            changed[texture_obj.assets_file.name] = texture_obj.assets_file
        for obj in env.objects:
            if obj.type.name == "Texture2D" and obj.peek_name() in WHOLE_TEXTURES:
                texture = obj.read()
                store(texture, Image.new("RGBA", (texture.m_Width, texture.m_Height), (0, 0, 0, 0)))
                changed[obj.assets_file.name] = obj.assets_file
                log(f"  {texture.m_Name}: all transparent")
        if not changed:
            raise SystemExit("Nothing found to clear: another game version?")
        replaced = {DATA + Path(name).name: f.save() for name, f in changed.items()}
        for name, blob in replaced.items():
            log(f"  {name}: {len(blob)} bytes")

        backup = apk_path.with_name(apk_path.name + ".slidepad.orig")
        if not backup.exists():
            shutil.copy2(apk_path, backup)
        out_path = work / "out.apk"
        with zipfile.ZipFile(apk_path) as apk, open(apk_path, "rb") as raw, open(out_path, "wb") as out:
            names = set(apk.namelist())
            missing = set(replaced) - names
            if missing:
                raise SystemExit(f"Not in the APK: {missing}")
            writer = assemble_apk.ApkWriter(out)
            for info in apk.infolist():
                if info.filename in replaced:
                    writer.add(info.filename, replaced[info.filename], compress=info.compress_type != 0)
                else:
                    writer.copy(raw, info)
            writer.close()
        with zipfile.ZipFile(out_path) as test:
            bad = test.testzip()
            if bad:
                raise RuntimeError(f"Corrupt entry in the output: {bad}")
        # In place: the same file (and its hard links), the same modification time.
        with open(out_path, "rb") as src, open(apk_path, "r+b") as dst:
            shutil.copyfileobj(src, dst, 1 << 22)
            dst.truncate()
        os.utime(apk_path, ns=(stat.st_atime_ns, stat.st_mtime_ns))
    log(f"Wrote {apk_path} ({apk_path.stat().st_size / 1e6:.1f} MB), its time kept")


if __name__ == "__main__":
    main()
