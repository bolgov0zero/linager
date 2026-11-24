#!/usr/bin/env python3
"""Makes rsrc_windows_amd64.syso: the icon and the manifest of linager-app.exe (run it when the icon or the manifest change;
the result is committed, so a build needs no extra tool). The Go linker picks a .syso file of the package by its name.

The icon comes from ../appicon.png (sizes 16 to 256, small ones as bitmaps, 256 as PNG), the manifest is app.manifest:
per-monitor DPI awareness, common controls v6, long paths, UTF-8.

The file is a COFF object with one .rsrc section: the resource tree, the data and one relocation per data entry (the entries
hold offsets from the start of the section; the linker turns them into addresses).
"""
import io
import struct
from pathlib import Path

from PIL import Image

here = Path(__file__).resolve().parent
SIZES = [16, 24, 32, 48, 64, 128, 256]
RT_ICON, RT_GROUP_ICON, RT_MANIFEST = 3, 14, 24
LANG = 0x0409


def dib(im: Image.Image) -> bytes:
    """An icon image as a DIB: 32 bit BGRA, bottom up, with an (empty) AND mask."""
    w, h = im.size
    px = im.convert("RGBA").tobytes("raw", "BGRA")
    rows = [px[y * w * 4:(y + 1) * w * 4] for y in range(h)][::-1]
    mask_row = ((w + 31) // 32) * 4
    head = struct.pack("<IiiHHIIiiII", 40, w, h * 2, 1, 32, 0, w * h * 4 + mask_row * h, 0, 0, 0, 0)
    return head + b"".join(rows) + bytes(mask_row * h)


def icons():
    src = Image.open(here.parent / "appicon.png").convert("RGBA")
    out = []
    for n in SIZES:
        im = src.resize((n, n), Image.LANCZOS)
        if n == 256:
            buf = io.BytesIO()
            im.save(buf, "PNG")
            out.append((n, buf.getvalue()))
        else:
            out.append((n, dib(im)))
    return out


def group(icons_):
    d = struct.pack("<HHH", 0, 1, len(icons_))
    for i, (n, data) in enumerate(icons_, 1):
        d += struct.pack("<BBBBHHIH", 0 if n == 256 else n, 0 if n == 256 else n, 0, 0, 1, 32, len(data), i)
    return d


class Tree:
    """Type -> name -> language -> data, built the way a PE resource section is laid out."""

    def __init__(self):
        self.res = {}  # type -> {name -> data}

    def add(self, typ, name, data):
        self.res.setdefault(typ, {})[name] = data

    def build(self):
        types = sorted(self.res)
        # sizes of the directories: 16 bytes of header and 8 per entry
        dirs = []  # (offset, entries) in the order they are written
        off = 16 + 8 * len(types)
        type_off, name_off = {}, {}
        for t in types:
            type_off[t] = off
            off += 16 + 8 * len(self.res[t])
        for t in types:
            for n in sorted(self.res[t]):
                name_off[(t, n)] = off
                off += 16 + 8
        entry_off = {}
        for t in types:
            for n in sorted(self.res[t]):
                entry_off[(t, n)] = off
                off += 16
        data_off = off
        buf = bytearray()
        relocs = []

        def directory(entries):
            return struct.pack("<IIHHHH", 0, 0, 0, 0, 0, len(entries)) + b"".join(struct.pack("<II", i, o) for i, o in entries)

        buf += directory([(t, 0x80000000 | type_off[t]) for t in types])
        for t in types:
            buf += directory([(n, 0x80000000 | name_off[(t, n)]) for n in sorted(self.res[t])])
        for t in types:
            for n in sorted(self.res[t]):
                buf += directory([(LANG, entry_off[(t, n)])])
        # data entries, then the data (4 byte aligned)
        pos = data_off
        blobs = []
        for t in types:
            for n in sorted(self.res[t]):
                d = self.res[t][n]
                relocs.append(len(buf))  # the field OffsetToData of this entry
                buf += struct.pack("<IIII", pos, len(d), 0, 0)
                blobs.append(d)
                pos += (len(d) + 3) & ~3
        assert len(buf) == data_off, (len(buf), data_off)
        for d in blobs:
            buf += d + bytes((-len(d)) % 4)
        return bytes(buf), relocs


def main():
    t = Tree()
    ic = icons()
    for i, (_, data) in enumerate(ic, 1):
        t.add(RT_ICON, i, data)
    t.add(RT_GROUP_ICON, 1, group(ic))
    t.add(RT_MANIFEST, 1, (here / "app.manifest").read_bytes())
    raw, relocs = t.build()

    hdr_size, sec_size = 20, 40
    raw_off = hdr_size + sec_size
    reloc_off = raw_off + len(raw)
    sym_off = reloc_off + 10 * len(relocs)
    obj = struct.pack("<HHIIIHH", 0x8664, 1, 0, sym_off, 2, 0, 0)
    obj += struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(raw), raw_off, reloc_off, 0, len(relocs), 0, 0x40000040)
    obj += raw
    for r in relocs:
        obj += struct.pack("<IIH", r, 0, 3)  # IMAGE_REL_AMD64_ADDR32NB against the section symbol
    obj += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 1)  # the symbol of the section
    obj += struct.pack("<IHHIHBBBB", len(raw), len(relocs), 0, 0, 0, 0, 0, 0, 0)[:18]  # its auxiliary record
    obj += struct.pack("<I", 4)  # an empty string table
    (here.parent.parent / "rsrc_windows_amd64.syso").write_bytes(obj)
    print("rsrc_windows_amd64.syso", len(obj), "bytes,", len(ic), "icons")


main()
