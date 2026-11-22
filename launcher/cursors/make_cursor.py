"""Draws the port's mouse cursor, cursors/nier.cur (an original design in the
launcher's colours, no game art), after the game's back button: a thick
chevron with a small arrow in its notch, turned to point up and left. Ivory
on a dark outline, with a faint light rim outside the outline so it reads on
dark and light screens alike.

    python make_cursor.py <out.cur> [preview.png]

The .cur holds 32 to 128 pixel images (32-bit BGRA with an AND mask), the
hotspot at the chevron's tip. Needs Pillow.
"""
import math
import struct
import sys

from PIL import Image, ImageDraw, ImageFilter

INK = (232, 228, 216, 255)      # --ink  #e8e4d8
DARK = (43, 40, 38, 255)        # --dialog #2b2826
RIM = (232, 228, 216, 90)       # the faint outer rim
SS = 8                          # supersampling
TIP = 2.6                       # the tip's place (both axes) in the 32-unit box


def shapes():
    """The chevron and the small arrow, pointing left (-x) with the tip at 0,0."""
    A, H, T = 15.0, 8.6, 5.0                # arm reach along x, half height, thickness along x
    chevron = [(0, 0), (A, -H), (A + T, -H), (T * 1.2, 0), (A + T, H), (A, H)]
    hx = T * 1.2 + 4.4                      # the small arrow's head tip, clear of the notch
    head = [(hx, 0), (hx + 4.2, -3.0), (hx + 4.2, 3.0)]
    shaft = [(hx + 3.8, -1.15), (A + T + 1.5, -1.15), (A + T + 1.5, 1.15), (hx + 3.8, 1.15)]
    return [chevron, head, shaft]


def place(pts, k):
    """Turned so -x points up and left, the tip at (TIP, TIP), scaled by k."""
    c = 1 / math.sqrt(2)
    return [((TIP + (x - y) * c * 1.05) * k, (TIP + (x + y) * c * 1.05) * k) for x, y in pts]


def render(size):
    k = size / 32.0 * SS
    n = size * SS
    mask = Image.new("L", (n, n), 0)
    d = ImageDraw.Draw(mask)
    for s in shapes():
        d.polygon(place(s, k), fill=255)
    odd = lambda px: max(3, int(px) | 1)
    outline = mask.filter(ImageFilter.MaxFilter(odd(1.5 * 2 * k)))
    rim = outline.filter(ImageFilter.MaxFilter(odd(0.9 * 2 * k)))
    img = Image.new("RGBA", (n, n), (0, 0, 0, 0))
    img.paste(Image.new("RGBA", (n, n), RIM), (0, 0), rim)
    img.paste(Image.new("RGBA", (n, n), DARK), (0, 0), outline)
    img.paste(Image.new("RGBA", (n, n), INK), (0, 0), mask)
    return img.resize((size, size), Image.LANCZOS)


def dib(img):
    """A cursor image: BITMAPINFOHEADER (double height), BGRA rows bottom-up, AND mask."""
    w, h = img.size
    px = img.load()
    header = struct.pack("<IiiHHIIiiII", 40, w, h * 2, 1, 32, 0, 0, 0, 0, 0, 0)
    xor = bytearray()
    for y in range(h - 1, -1, -1):
        for x in range(w):
            r, g, b, a = px[x, y]
            xor += bytes((b, g, r, a))
    row = ((w + 31) // 32) * 4
    mask = bytearray()
    for y in range(h - 1, -1, -1):
        bits = bytearray(row)
        for x in range(w):
            if px[x, y][3] == 0:
                bits[x // 8] |= 0x80 >> (x % 8)
        mask += bits
    return header + bytes(xor) + bytes(mask)


def main():
    out = sys.argv[1]
    sizes = (32, 48, 64, 96, 128)
    images = [render(s) for s in sizes]
    blobs = [dib(i) for i in images]
    data = struct.pack("<HHH", 0, 2, len(sizes))
    offset = 6 + 16 * len(sizes)
    for s, blob in zip(sizes, blobs):
        hot = round(TIP * s / 32)
        data += struct.pack("<BBBBHHII", s % 256, s % 256, 0, 0, hot, hot, len(blob), offset)
        offset += len(blob)
    data += b"".join(blobs)
    with open(out, "wb") as f:
        f.write(data)
    if len(sys.argv) > 2:  # a preview: each size on the launcher's dialog and on white, and 4x
        prev = Image.new("RGBA", (560, 280), (43, 40, 38, 255))
        x = 10
        for img in images:
            prev.alpha_composite(img, (x, 10))
            white = Image.new("RGBA", img.size, (240, 240, 240, 255))
            white.alpha_composite(img)
            prev.alpha_composite(white, (x, 145))
            x += img.size[0] + 12
        
        prev.save(sys.argv[2])


if __name__ == "__main__":
    main()
