#!/usr/bin/env python3
"""render-brand.py: Scry's L3-S3 brand mark ([S113-3], restored in [S125-1]).

DESIGN.md, "Brand mark and wordmark", holds the mark as two canonical SVG
blocks (light and dark) on a 32 x 32 grid: three solid portrait cards
(16 x 19, rx 2.5), the back and middle cards upright and the cobalt top card
rotated 10 degrees clockwise around (19, 14) with its prompt bar. Those
blocks are binding and verbatim, so this script never draws the mark: it
copies the two blocks byte for byte out of DESIGN.md into
internal/web/brand/mark-light.svg and mark-dark.svg, and derives every
surface mechanically from those copies:

  favicon.svg          both canonical drawings, unchanged, in one 32 x 32
                       SVG, switched by prefers-color-scheme
  favicon-16/32.png    hinted rasters: the upright cards' x, y, width and
                       height are snapped to whole target pixels (the
                       rotated top card and its bar stay exactly as written)
  app icons            the canonical drawing, scaled whole by 1.25 (the mark
                       at 62.5%: 40 px on 64) and centred in a 64 px
                       continuous-corner squircle (light: #FFFFFF with a 1 px
                       line edge, light fills; dark: #12101A, dark fills),
                       then rasterised at 64, 180, 192 and 512 px
  icon-maskable.svg    the dark drawing at the same scale on a full-bleed
                       #12101A ground (180 px apple-touch, maskable 192/512)

The inline 24 px header mark is internal/web/brand.go, which embeds the
same two copies. Run from the repository root (needs rsvg-convert and
ImageMagick convert):

  python3 scripts/render-brand.py            # rewrite the committed assets
  python3 scripts/render-brand.py --evidence ~/chief-evidence/media

TestDesign_S125_1_* pins the results.
"""

import argparse
import hashlib
import math
import os
import re
import subprocess
import sys
import tempfile

STATIC = "internal/web/static"
SOURCES = "internal/web/brand"
LINE = "#E2E2E3"  # the light app icon's 1 px `line` edge (ink 12% over page)
PAGE = {"light": "#FFFFFF", "dark": "#12101A"}


def canonical():
    """The two ```svg blocks of DESIGN.md's brand section, verbatim."""
    with open("DESIGN.md", encoding="utf-8") as f:
        design = f.read()
    start = design.index("## Brand mark and wordmark")
    end = design.index("\n## ", start + 1)
    blocks = re.findall(r"```svg\n(.*?)\n```", design[start:end], re.S)
    if len(blocks) != 2:
        sys.exit(f"DESIGN.md brand section holds {len(blocks)} svg blocks, want 2")
    return {"light": blocks[0] + "\n", "dark": blocks[1] + "\n"}


def inner(svg):
    """The drawing inside the canonical <svg>: its lines, indentation
    dropped, joined (whitespace between tags only)."""
    body = svg[svg.index(">") + 1:svg.rindex("</svg>")]
    return "".join(line.strip() for line in body.splitlines())


def num(v):
    s = ("%.4f" % v).rstrip("0").rstrip(".")
    return "0" if s == "-0" else s


UPRIGHT = re.compile(r'^(<rect x=")([\d.]+)(" y=")([\d.]+)(" width=")([\d.]+)(" height=")([\d.]+)(".*)$')


def hinted(svg, size):
    """The canonical drawing with its upright cards snapped to whole pixels
    at `size` px: position and size each rounded on the target grid, so the
    two upright cards stay identical portrait slabs. The rotated group (top
    card and bar) is untouched."""
    unit = 32 / size
    snap = lambda v: num(math.floor(float(v) / unit + 0.5) * unit)
    out, rotated = [], False
    for line in svg.splitlines():
        s = line.strip()
        if s.startswith("<g "):
            rotated = True
        elif s == "</g>":
            rotated = False
        m = UPRIGHT.match(s)
        if m and not rotated:
            g = list(m.groups())
            g[1], g[3], g[5], g[7] = snap(g[1]), snap(g[3]), snap(g[5]), snap(g[7])
            line = line[:len(line) - len(s)] + "".join(g)
        out.append(line)
    return "\n".join(out) + "\n"


def favicon(marks):
    """One 32 x 32 SVG carrying both canonical drawings, unchanged."""
    return ('<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none">'
            '<style>.mark-dark{display:none}@media (prefers-color-scheme:dark)'
            '{.mark-light{display:none}.mark-dark{display:inline}}</style>'
            f'<g class="mark-light">{inner(marks["light"])}</g>'
            f'<g class="mark-dark">{inner(marks["dark"])}</g></svg>\n')


def squircle(a, cx=32.0, cy=32.0, n=5.0, steps=96):
    """A continuous-curvature superellipse |x|^n + |y|^n = a^n."""
    pts = []
    for i in range(steps):
        t = 2 * math.pi * i / steps
        c, s = math.cos(t), math.sin(t)
        x = cx + a * math.copysign(abs(c) ** (2 / n), c)
        y = cy + a * math.copysign(abs(s) ** (2 / n), s)
        pts.append(f"{num(round(x, 2))} {num(round(y, 2))}")
    return "M" + "L".join(pts) + "Z"


# The mark at 62.5% of the icon: the 32 x 32 grid scaled whole to 40 px on
# 64, centred (12 px each side).
SCALED = '<g class="mark" transform="translate(12 12) scale(1.25)">'


def app_icon(marks, mode):
    if mode == "light":  # white ground, a 1 px `line` edge drawn just inside
        ground = f'<path class="ground" d="{squircle(31.5)}" fill="{PAGE[mode]}" stroke="{LINE}" stroke-width="1"/>'
    else:
        ground = f'<path class="ground" d="{squircle(32)}" fill="{PAGE[mode]}"/>'
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64" fill="none">'
            f'{ground}{SCALED}{inner(marks[mode])}</g></svg>\n')


def maskable(marks):
    """Full-bleed dark ground: the platform supplies the mask; the mark sits
    inside the central 80% safe circle."""
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64" fill="none">'
            f'<rect class="ground" width="64" height="64" fill="{PAGE["dark"]}"/>{SCALED}{inner(marks["dark"])}</g></svg>\n')


def write(path, text):
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write(text)


def raster(svg, png, size):
    subprocess.run(["rsvg-convert", "-w", str(size), "-h", str(size), svg, "-o", png], check=True)


def strip(png):
    # Drop timestamps and other ancillary chunks so a re-run is byte-stable.
    subprocess.run(["convert", png, "-strip", "-define", "png:exclude-chunks=date,time", png], check=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--evidence", help="also write nearest-neighbour proofs here")
    args = ap.parse_args()
    if not os.path.isdir(STATIC):
        sys.exit("run from the repository root")
    os.makedirs(SOURCES, exist_ok=True)
    for old in os.listdir(SOURCES):  # the retired S3-A sources
        os.remove(os.path.join(SOURCES, old))

    marks = canonical()
    for mode, svg in marks.items():
        write(f"{SOURCES}/mark-{mode}.svg", svg)
        print(f"mark-{mode}.svg sha256 {hashlib.sha256(svg.encode()).hexdigest()}")
    for size in (16, 32):
        for mode in ("light", "dark"):
            write(f"{SOURCES}/favicon-{size}-{mode}.svg", hinted(marks[mode], size))
    write(f"{SOURCES}/icon-maskable.svg", maskable(marks))
    write(f"{STATIC}/favicon.svg", favicon(marks))
    write(f"{STATIC}/app-icon.svg", app_icon(marks, "light"))
    write(f"{STATIC}/app-icon-dark.svg", app_icon(marks, "dark"))

    pngs = [
        (f"{SOURCES}/favicon-16-light.svg", f"{STATIC}/favicon-16.png", 16),
        (f"{SOURCES}/favicon-32-light.svg", f"{STATIC}/favicon-32.png", 32),
        (f"{STATIC}/app-icon.svg", f"{STATIC}/app-icon-64.png", 64),
        (f"{STATIC}/app-icon-dark.svg", f"{STATIC}/app-icon-64-dark.png", 64),
        (f"{STATIC}/app-icon-dark.svg", f"{STATIC}/icon-192.png", 192),
        (f"{STATIC}/app-icon-dark.svg", f"{STATIC}/icon-512.png", 512),
        (f"{SOURCES}/icon-maskable.svg", f"{STATIC}/icon-180.png", 180),
        (f"{SOURCES}/icon-maskable.svg", f"{STATIC}/icon-maskable-192.png", 192),
        (f"{SOURCES}/icon-maskable.svg", f"{STATIC}/icon-maskable-512.png", 512),
    ]
    for svg, png, size in pngs:
        raster(svg, png, size)
        strip(png)
    # /favicon.ico: the two hinted light PNGs, 16 and 32.
    subprocess.run(["convert", f"{STATIC}/favicon-16.png", f"{STATIC}/favicon-32.png", f"{STATIC}/favicon.ico"], check=True)

    if args.evidence:
        os.makedirs(args.evidence, exist_ok=True)
        with tempfile.TemporaryDirectory() as tmp:
            proofs = [
                (f"{SOURCES}/favicon-16-light.svg", 16, "brand-favicon16-light-8x.png"),
                (f"{SOURCES}/favicon-16-dark.svg", 16, "brand-favicon16-dark-8x.png"),
                (f"{SOURCES}/favicon-32-light.svg", 32, "brand-favicon32-light-8x.png"),
                (f"{SOURCES}/favicon-32-dark.svg", 32, "brand-favicon32-dark-8x.png"),
                (f"{STATIC}/app-icon.svg", 64, "brand-appicon64-light-4x.png"),
                (f"{STATIC}/app-icon-dark.svg", 64, "brand-appicon64-dark-4x.png"),
            ]
            for svg, size, name in proofs:
                small = os.path.join(tmp, name)
                raster(svg, small, size)
                scale = "800%" if size <= 32 else "400%"
                bg = "#8A8A90" if "appicon" in name else PAGE["dark"] if "dark" in name else PAGE["light"]
                subprocess.run(["convert", small, "-background", bg, "-alpha", "remove", "-alpha", "off",
                                "-filter", "point", "-resize", scale, "-strip",
                                os.path.join(args.evidence, name)], check=True)


if __name__ == "__main__":
    main()
