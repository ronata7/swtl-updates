#!/usr/bin/env python3
"""Generates rsrc_windows_amd64.syso (exe icon + version/copyright info) and assets/app.ico.
No external tools: writes the COFF resource object directly. Needs Pillow.
Run:  python3 genrsrc.py   then   ./build.sh
"""
import io, struct
from PIL import Image

VERSION = (2, 24, 0, 0)
INFO = {
    "CompanyName":      "ronata",
    "FileDescription":  "SoulWorker translation data patcher",
    "FileVersion":      "%d.%d.%d.%d" % VERSION,
    "InternalName":     "SWTLUpdater",
    "LegalCopyright":   "Copyright © 2026 ronata",
    "OriginalFilename": "SWTLUpdater.exe",
    "ProductName":      "SW TL Updater",
    "ProductVersion":   "%d.%d.%d.%d" % VERSION,
}
ICONS = ["assets/icon16.png", "assets/icon24.png", "assets/icon32.png", "assets/icon48.png", "assets/icon256.png"]

def align4(b): return b + b"\0" * ((-len(b)) % 4)

# ---------- icons ----------
def dib(im):                      # 32-bit BMP-in-ICO payload (no file header)
    w, h = im.size
    px = im.convert("RGBA").load()
    xor = bytearray()
    for y in range(h - 1, -1, -1):
        for x in range(w):
            r, g, b, a = px[x, y]
            xor += bytes((b, g, r, a))
    mask = b"\0" * (((w + 31) // 32) * 4 * h)
    return struct.pack("<IiiHHIIiiII", 40, w, h * 2, 1, 32, 0, len(xor) + len(mask), 0, 0, 0, 0) + bytes(xor) + mask

def png(im):
    b = io.BytesIO(); im.convert("RGBA").save(b, "PNG"); return b.getvalue()

imgs = []
for f in ICONS:
    im = Image.open(f).convert("RGBA")
    assert im.size[0] == im.size[1]
    imgs.append((im.size[0], png(im) if im.size[0] >= 256 else dib(im)))

# standalone .ico for reference
ico = struct.pack("<HHH", 0, 1, len(imgs)); off = 6 + 16 * len(imgs); body = b""
for sz, data in imgs:
    ico += struct.pack("<BBBBHHII", sz % 256, sz % 256, 0, 0, 1, 32, len(data), off + len(body)); body += data
open("assets/app.ico", "wb").write(ico + body)

# ---------- version info ----------
def blk(key, value=b"", vlen=0, typ=0, children=b""):
    b = b"\0\0" + struct.pack("<HH", vlen, typ) + (key + "\0").encode("utf-16le")
    b = align4(b) + value
    if children: b = align4(b) + children
    return struct.pack("<H", len(b)) + b[2:]

def vstr(k, v): 
    val = (v + "\0").encode("utf-16le")
    return align4(blk(k, val, len(v) + 1, 1))

v = VERSION
ms, ls = (v[0] << 16) | v[1], (v[2] << 16) | v[3]
fixed = struct.pack("<IIIIIIIIIIIII", 0xFEEF04BD, 0x00010000, ms, ls, ms, ls, 0x3F, 0, 0x00040004, 1, 0, 0, 0)
strings = b"".join(vstr(k, val) for k, val in sorted(INFO.items()))
stable = align4(blk("040904B0", b"", 0, 1, strings))
sfi = align4(blk("StringFileInfo", b"", 0, 1, stable))
var = align4(blk("Translation", struct.pack("<HH", 0x0409, 0x04B0), 4, 0))
vfi = align4(blk("VarFileInfo", b"", 0, 1, var))
version_res = blk("VS_VERSION_INFO", fixed, len(fixed), 0, sfi + vfi)

# ---------- group icon ----------
grp = struct.pack("<HHH", 0, 1, len(imgs))
for i, (sz, data) in enumerate(imgs, 1):
    grp += struct.pack("<BBBBHHIH", sz % 256, sz % 256, 0, 0, 1, 32, len(data), i)

# ---------- resource tree ----------
LANG = 0x0409
res = {3: {i: data for i, (sz, data) in enumerate(imgs, 1)}, 14: {1: grp}, 16: {1: version_res}}
types = sorted(res)
pos = 16 + 8 * len(types)
type_off, name_off, de_off, blob_off = {}, {}, {}, {}
for t in types:
    type_off[t] = pos; pos += 16 + 8 * len(res[t])
for t in types:
    for n in sorted(res[t]): name_off[(t, n)] = pos; pos += 16 + 8
for t in types:
    for n in sorted(res[t]): de_off[(t, n)] = pos; pos += 16
for t in types:
    for n in sorted(res[t]):
        pos = (pos + 3) & ~3; blob_off[(t, n)] = pos; pos += len(res[t][n])
sec = bytearray(pos)
def dirhdr(o, n): sec[o:o + 16] = struct.pack("<IIHHHH", 0, 0, 0, 0, 0, n)
dirhdr(0, len(types))
for i, t in enumerate(types):
    sec[16 + 8 * i:24 + 8 * i] = struct.pack("<II", t, 0x80000000 | type_off[t])
relocs = []
for t in types:
    names = sorted(res[t]); dirhdr(type_off[t], len(names))
    for i, n in enumerate(names):
        o = type_off[t] + 16 + 8 * i
        sec[o:o + 8] = struct.pack("<II", n, 0x80000000 | name_off[(t, n)])
        dirhdr(name_off[(t, n)], 1)
        sec[name_off[(t, n)] + 16:name_off[(t, n)] + 24] = struct.pack("<II", LANG, de_off[(t, n)])
        d = de_off[(t, n)]
        sec[d:d + 16] = struct.pack("<IIII", blob_off[(t, n)], len(res[t][n]), 0, 0)
        relocs.append(d)
        sec[blob_off[(t, n)]:blob_off[(t, n)] + len(res[t][n])] = res[t][n]

# ---------- COFF object ----------
raw_off = 20 + 40
reloc_off = raw_off + len(sec)
sym_off = reloc_off + 10 * len(relocs)
out = struct.pack("<HHIIIHH", 0x8664, 1, 0, sym_off, 1, 0, 0)
out += struct.pack("<8sIIIIIIHHI", b".rsrc", 0, 0, len(sec), raw_off, reloc_off, 0, len(relocs), 0, 0x40000040)
out += bytes(sec)
for r in relocs: out += struct.pack("<IIH", r, 0, 3)          # IMAGE_REL_AMD64_ADDR32NB -> section symbol
out += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)       # section symbol
out += struct.pack("<I", 4)                                    # empty string table
open("rsrc_windows_amd64.syso", "wb").write(out)
print("wrote rsrc_windows_amd64.syso (%d bytes), %d icons, %d resource blobs" % (len(out), len(imgs), len(relocs)))
