#!/usr/bin/env python3
"""render-brand.py: draw Scry's L3-S3 brand mark ([S113-3], [S125-1]) at every size.

DESIGN.md, "Brand mark and wordmark": the mark is L3-S3, the Fanned Scry
Reveal · Cobalt Tilt, exactly as approved in Visual Lab 1. Its canonical SVG
is binding and verbatim: three solid portrait cards (16 x 19, rx 2.5) on a
32 x 32 grid, back to front, the top card rotated 10 degrees clockwise
around (19, 14) with its prompt bar riding the tilt. Only the fills change
between light and dark, and they are the mark's own fixed brand colours, not
theme tokens.

Every surface draws that one geometry, scaled whole:

  header       internal/web/brand.go renders it inline at 24 px (the same
               numbers; TestDesign_S125_1_CanonicalSVGIsDesignVerbatim)
  favicon.svg  the 32 x 32 grid with both fill sets, switched by
               prefers-color-scheme
  16/32 px     PNG favicons rasterised from the canonical SVG, hinted: the
               two upright cards snapped so their edges land on whole pixels
               (16 x 19 stays portrait), the tilted top card and its prompt
               bar untouched
  app icons    the mark at 62.5% (translate 12, scale 1.25: 40 px on 64)
               centred in a continuous-corner superellipse, light (#FFFFFF,
               1 px line edge, light fills) and dark (#12101A, dark fills);
               the dark one is the 192/512 home-screen icon, and its
               full-bleed form the maskable, apple-touch (180) and social
               preview icon

Run from the repository root (needs rsvg-convert and ImageMagick convert):

  python3 scripts/render-brand.py            # rewrite the committed assets
  python3 scripts/render-brand.py --evidence ~/chief-evidence/media

It rewrites the served files in internal/web/static/ and the sources in
internal/web/brand/. The Go tests in internal/web pin the results.
"""

import argparse
import math
import os
import subprocess
import sys
import tempfile

STATIC = "internal/web/static"
SOURCES = "internal/web/brand"

# DESIGN.md, Brand mark and wordmark: the layer table, back to front.
LIGHT = {"back": "#CBD5E1", "middle": "#1E293B", "top": "#0562EF", "bar": "#FFFFFF", "page": "#FFFFFF"}
DARK = {"back": "#334155", "middle": "#64748B", "top": "#3B82F6", "bar": "#DBEAFE", "page": "#12101A"}
# The light app icon's 1 px `line` edge: ink #0B1015 at 12% over #FFFFFF.
LINE = "#E2E2E3"

BACK = (5, 10)      # x, y of the upright back card
MIDDLE = (7, 7.5)   # x, y of the upright middle card


def num(v):
    s = ("%.2f" % v).rstrip("0").rstrip(".")
    return "0" if s == "-0" else s


def canonical(pal):
    """DESIGN.md's canonical SVG, verbatim, in one palette."""
    return (
        '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" fill="none">\n'
        f'  <rect x="5" y="10" width="16" height="19" rx="2.5" fill="{pal["back"]}"/>\n'
        f'  <rect x="7" y="7.5" width="16" height="19" rx="2.5" fill="{pal["middle"]}"/>\n'
        '  <g transform="rotate(10 19 14)">\n'
        f'    <rect x="11" y="3.5" width="16" height="19" rx="2.5" fill="{pal["top"]}"/>\n'
        f'    <rect x="14.5" y="7" width="9" height="2" rx="1" fill="{pal["bar"]}" opacity="0.85"/>\n'
        '  </g>\n'
        '</svg>\n'
    )


def body(pal, back=BACK, middle=MIDDLE, classes=False, h=19):
    """The four layers, with optional classes (so a style sheet can switch
    the fill pair) and optionally hinted upright card positions."""
    def c(name):
        return f' class="mark-{name}"' if classes else ""
    return (
        f'<rect{c("back")} x="{num(back[0])}" y="{num(back[1])}" width="16" height="{h}" rx="2.5" fill="{pal["back"]}"/>'
        f'<rect{c("middle")} x="{num(middle[0])}" y="{num(middle[1])}" width="16" height="{h}" rx="2.5" fill="{pal["middle"]}"/>'
        '<g transform="rotate(10 19 14)">'
        f'<rect{c("top")} x="11" y="3.5" width="16" height="19" rx="2.5" fill="{pal["top"]}"/>'
        f'<rect{c("bar")} x="14.5" y="7" width="9" height="2" rx="1" fill="{pal["bar"]}" opacity="0.85"/>'
        '</g>'
    )


def favicon_svg():
    """The served SVG favicon: the canonical grid, light fills, switched to
    the dark pair by prefers-color-scheme."""
    css = ("@media (prefers-color-scheme:dark){"
           f".mark-back{{fill:{DARK['back']}}}.mark-middle{{fill:{DARK['middle']}}}"
           f".mark-top{{fill:{DARK['top']}}}.mark-bar{{fill:{DARK['bar']}}}}}")
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="32" height="32" fill="none">'
            f'<style>{css}</style>{body(LIGHT, classes=True)}</svg>\n')


# Hinting: at 16 px one grid unit is half a pixel, so the upright cards'
# x 5 / 7, y 7.5 and 19-unit height (9.5 px) fall between pixels. Snap each
# edge to the nearest even grid unit (a whole pixel): the cards become 8 x 10
# px (still portrait), the back at (2, 5) px and the middle at (3, 4) px, one
# pixel fanned up and right as in the canonical drawing. At 32 px one unit is
# one pixel: only the middle card's y 7.5 snaps (to 8; 16 x 19 kept). The
# tilted top card and its prompt bar cannot sit on a pixel grid and are
# drawn exactly, tilt and all.
HINT = {16: {"back": (4, 10), "middle": (6, 8), "h": 20}, 32: {"back": (5, 10), "middle": (7, 8), "h": 19}}


def favicon_hinted(size, pal):
    h = HINT[size]
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32" width="{size}" height="{size}" fill="none">'
            f'{body(pal, h["back"], h["middle"], h=h["h"])}</svg>\n')


def squircle(a, cx=32.0, cy=32.0, n=5.0, steps=96):
    """A continuous-curvature superellipse |x|^n + |y|^n = a^n."""
    pts = []
    for i in range(steps):
        t = 2 * math.pi * i / steps
        c, s = math.cos(t), math.sin(t)
        x = cx + a * math.copysign(abs(c) ** (2 / n), c)
        y = cy + a * math.copysign(abs(s) ** (2 / n), s)
        pts.append(f"{num(x)} {num(y)}")
    return "M" + "L".join(pts) + "Z"


# The mark at 62.5% of the icon: the 32 grid scaled 1.25 to 40 px, centred.
ICON_MARK = '<g class="mark" transform="translate(12 12) scale(1.25)">{}</g>'


def app_icon(pal, edge=None):
    if edge:  # light: white ground, a 1 px `line` edge drawn just inside
        ground = f'<path class="ground" d="{squircle(31.5)}" fill="{pal["page"]}" stroke="{edge}" stroke-width="1"/>'
    else:
        ground = f'<path class="ground" d="{squircle(32)}" fill="{pal["page"]}"/>'
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64">'
            f'{ground}{ICON_MARK.format(body(pal))}</svg>\n')


def maskable(pal):
    """Full-bleed ground: the platform supplies the mask; the mark sits well
    inside the central 80% safe circle."""
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64">'
            f'<rect class="ground" width="64" height="64" fill="{pal["page"]}"/>{ICON_MARK.format(body(pal))}</svg>\n')


def write(path, text):
    with open(path, "w") as f:
        f.write(text)


def raster(svg, png, size):
    subprocess.run(["rsvg-convert", "-w", str(size), "-h", str(size), svg, "-o", png], check=True)


def strip(png):
    # Drop timestamps and other ancillary chunks so a re-run is byte-stable.
    subprocess.run(["convert", png, "-strip", "-define", "png:exclude-chunks=date,time", png], check=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--evidence", help="also write magnified proofs here")
    args = ap.parse_args()
    if not os.path.isdir(STATIC):
        sys.exit("run from the repository root")
    os.makedirs(SOURCES, exist_ok=True)
    for old in os.listdir(SOURCES):  # retire the interim deck's sources
        if old.endswith(".svg"):
            os.remove(os.path.join(SOURCES, old))

    # Served vectors.
    write(f"{STATIC}/favicon.svg", favicon_svg())
    write(f"{STATIC}/app-icon.svg", app_icon(LIGHT, edge=LINE))
    write(f"{STATIC}/app-icon-dark.svg", app_icon(DARK))
    # Sources: the canonical mark itself (byte for byte DESIGN.md's blocks),
    # the hinted favicon drawings (one per mode; rsvg has no
    # prefers-color-scheme) and the full-bleed maskable icon.
    src = {
        "mark-light.svg": canonical(LIGHT), "mark-dark.svg": canonical(DARK),
        "favicon-16-light.svg": favicon_hinted(16, LIGHT), "favicon-16-dark.svg": favicon_hinted(16, DARK),
        "favicon-32-light.svg": favicon_hinted(32, LIGHT), "favicon-32-dark.svg": favicon_hinted(32, DARK),
        "icon-maskable.svg": maskable(DARK),
    }
    for name, text in src.items():
        write(f"{SOURCES}/{name}", text)

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
                (f"{SOURCES}/mark-light.svg", 24, "brand-header24-light-8x.png"),
                (f"{SOURCES}/mark-dark.svg", 24, "brand-header24-dark-8x.png"),
                (f"{SOURCES}/mark-light.svg", 256, "brand-canonical-light-256.png"),
                (f"{SOURCES}/mark-dark.svg", 256, "brand-canonical-dark-256.png"),
                (f"{STATIC}/app-icon.svg", 64, "brand-appicon64-light-4x.png"),
                (f"{STATIC}/app-icon-dark.svg", 64, "brand-appicon64-dark-4x.png"),
                (f"{SOURCES}/icon-maskable.svg", 64, "brand-maskable64-dark-4x.png"),
            ]
            for svg, size, name in proofs:
                small = os.path.join(tmp, name)
                raster(svg, small, size)
                scale = "100%" if size == 256 else "400%" if size == 64 else "800%"
                # Favicons and the header mark on their mode's page; app
                # icons on a neutral grey so the squircle's outline shows.
                bg = "#8A8A90" if "appicon" in name or "maskable" in name else "#12101A" if "dark" in name else "#FFFFFF"
                subprocess.run(["convert", small, "-background", bg, "-alpha", "remove", "-alpha", "off",
                                "-filter", "point", "-resize", scale, "-strip",
                                os.path.join(args.evidence, name)], check=True)


if __name__ == "__main__":
    main()
