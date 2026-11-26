#!/usr/bin/env python3
"""render-brand.py: draw Scry's S3-A brand mark ([S113-3]) at every size.

DESIGN.md, "Brand mark and wordmark": three identical 14 x 10 cards with
1.5 px corners on a 24 px grid, each 2.5 px up and right of the one in front.
The top card is a solid accent fill with a page-coloured prompt bar knocked
out of it; the two receding cards are 1.5 px outlines (nearer ink-muted,
farther ink at 35%) filled with page so each hides the one behind.

The 24 px drawing is internal/web/brand.go's inline header mark. The small
sizes are hand-hinted redrawings on their own pixel grid (every edge on a
whole pixel, parallel edges at least 1 px apart, the prompt bar at least
1 px tall and clear of the card's edges, no anti-aliased pixel at all):

  16 px  cards 10 x 7, step 2, 1 px outline, corners notched 1 px, bar 5 x 1
  24 px  cards 14 x 10, step 3, 2 px outline, corners notched 1 px, bar 7 x 2
         (the header mark on a 1x screen; brand.go's markHinted)
  32 px  cards 18 x 13, step 3, 2 px outline, corners notched 1 px, bar 9 x 3
  64 px  cards 24 x 17, step 4, 2 px outline, rx 2.5, bar 11 x 3 rx 1.5
         (the app icon's 40 px mark, centred in a 64 px superellipse)

Run from the repository root (needs rsvg-convert and ImageMagick convert):

  python3 scripts/render-brand.py            # rewrite the committed assets
  python3 scripts/render-brand.py --evidence ~/chief-evidence/media

It rewrites the served files in internal/web/static/ and the raster-only
sources in internal/web/brand/. TestStory_S113_3_OneMarkEverywhere pins the
results.
"""

import argparse
import math
import os
import subprocess
import sys
import tempfile

STATIC = "internal/web/static"
SOURCES = "internal/web/brand"


def mix(fg, bg, alpha):
    """fg over bg at alpha, as an opaque #RRGGBB (ink steps over page)."""
    f = [int(fg[i:i + 2], 16) for i in (1, 3, 5)]
    b = [int(bg[i:i + 2], 16) for i in (1, 3, 5)]
    return "#" + "".join("%02X" % round(bb + alpha * (ff - bb)) for ff, bb in zip(f, b))


# DESIGN.md, Color. Receding cards use the ink steps composited over page, so
# they stay opaque on a transparent favicon ground and hide the card behind.
LIGHT = {"page": "#FFFFFF", "ink": "#0B1015", "accent": "#0562EF"}
DARK = {"page": "#12101A", "ink": "#FFFFFF", "accent": "#4D8DFF"}
for p in (LIGHT, DARK):
    p["near"] = mix(p["ink"], p["page"], 0.60)  # ink-muted
    p["far"] = mix(p["ink"], p["page"], 0.35)
LIGHT["line"] = mix(LIGHT["ink"], LIGHT["page"], 0.12)  # the light icon's 1 px edge


def num(v):
    s = ("%.2f" % v).rstrip("0").rstrip(".")
    return "0" if s == "-0" else s


def notched(x, y, w, h):
    """A w x h pixel block with its four corner pixels left out: the
    crisp, hinted stand-in for a small rounded corner."""
    return (f"M{x + 1} {y}H{x + w - 1}V{y + 1}H{x + w}V{y + h - 1}H{x + w - 1}V{y + h}"
            f"H{x + 1}V{y + h - 1}H{x}V{y + 1}H{x + 1}Z")


def style(pal, dark=None):
    css = (f".mark-far{{fill:{pal['far']}}}.mark-near{{fill:{pal['near']}}}"
           f".mark-face,.mark-bar{{fill:{pal['page']}}}.mark-top{{fill:{pal['accent']}}}")
    if dark:
        css += ("@media (prefers-color-scheme:dark){"
                f".mark-far{{fill:{dark['far']}}}.mark-near{{fill:{dark['near']}}}"
                f".mark-face,.mark-bar{{fill:{dark['page']}}}.mark-top{{fill:{dark['accent']}}}}}")
    return f"<style>{css}</style>"


def hinted(cards, w, h, stroke, bar, bar_notch):
    """The hinted 16 and 32 px drawing: back to front, farther, nearer, top."""
    out = []
    for cls, (x, y) in zip(("mark-far", "mark-near"), cards[:2]):
        out.append(f'<path class="{cls}" d="{notched(x, y, w, h)}"/>')
        out.append(f'<rect class="mark-face" x="{x + stroke}" y="{y + stroke}" width="{w - 2 * stroke}" height="{h - 2 * stroke}"/>')
    x, y = cards[2]
    out.append(f'<path class="mark-top" d="{notched(x, y, w, h)}"/>')
    bx, by, bw, bh = bar
    if bar_notch:
        out.append(f'<path class="mark-bar" d="{notched(bx, by, bw, bh)}"/>')
    else:
        out.append(f'<rect class="mark-bar" x="{bx}" y="{by}" width="{bw}" height="{bh}"/>')
    return "".join(out)


FAV16 = dict(cards=[(5, 2), (3, 4), (1, 6)], w=10, h=7, stroke=1, bar=(3, 9, 5, 1), bar_notch=False)
HINT24 = dict(cards=[(8, 4), (5, 7), (2, 10)], w=14, h=10, stroke=2, bar=(5, 14, 7, 2), bar_notch=False)
FAV32 = dict(cards=[(10, 6), (7, 9), (4, 12)], w=18, h=13, stroke=2, bar=(8, 17, 9, 3), bar_notch=True)


def favicon(size, geo, pal, dark=None):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {size} {size}" width="{size}" height="{size}" '
            f'shape-rendering="crispEdges">{style(pal, dark)}{hinted(**geo)}</svg>\n')


def mark24(pal):
    """The exact 24 px grid drawing, as brand.go renders it inline."""
    body = (f'<rect class="mark-far" x="7.5" y="4.5" width="14" height="10" rx="1.5"/>'
            f'<rect class="mark-face" x="9" y="6" width="11" height="7"/>'
            f'<rect class="mark-near" x="5" y="7" width="14" height="10" rx="1.5"/>'
            f'<rect class="mark-face" x="6.5" y="8.5" width="11" height="7"/>'
            f'<rect class="mark-top" x="2.5" y="9.5" width="14" height="10" rx="1.5"/>'
            f'<rect class="mark-bar" x="5.5" y="13.5" width="6.5" height="2" rx="1"/>')
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24" height="24">'
            f'{style(pal)}{body}</svg>\n')


def mark64():
    """The app icon's mark: 40 px, centred on the 64 px canvas, hinted to
    whole pixels at 64."""
    W, H, S = 24, 17, 2
    out = []
    for cls, (x, y) in (("mark-far", (24, 19)), ("mark-near", (20, 23))):
        out.append(f'<rect class="{cls}" x="{x}" y="{y}" width="{W}" height="{H}" rx="2.5"/>')
        out.append(f'<rect class="mark-face" x="{x + S}" y="{y + S}" width="{W - 2 * S}" height="{H - 2 * S}" rx="0.5"/>')
    out.append(f'<rect class="mark-top" x="16" y="27" width="{W}" height="{H}" rx="2.5"/>')
    out.append('<rect class="mark-bar" x="21" y="34" width="11" height="3" rx="1.5"/>')
    return "".join(out)


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


def app_icon(pal, edge=None):
    if edge:  # light: white ground, a 1 px `line` edge drawn just inside
        ground = f'<path class="ground" d="{squircle(31.5)}" fill="{pal["page"]}" stroke="{edge}" stroke-width="1"/>'
    else:
        ground = f'<path class="ground" d="{squircle(32)}" fill="{pal["page"]}"/>'
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64">'
            f'{style(pal)}{ground}{mark64()}</svg>\n')


def maskable(pal):
    """Full-bleed ground: the platform supplies the mask; the mark sits well
    inside the central 80% safe circle."""
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" width="64" height="64">'
            f'{style(pal)}<rect class="ground" width="64" height="64" fill="{pal["page"]}"/>{mark64()}</svg>\n')


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
    ap.add_argument("--evidence", help="also write 8x nearest-neighbour proofs here")
    args = ap.parse_args()
    if not os.path.isdir(STATIC):
        sys.exit("run from the repository root")
    os.makedirs(SOURCES, exist_ok=True)

    # Served vectors.
    write(f"{STATIC}/favicon.svg", favicon(16, FAV16, LIGHT, DARK))
    write(f"{STATIC}/app-icon.svg", app_icon(LIGHT, edge=LIGHT["line"]))
    write(f"{STATIC}/app-icon-dark.svg", app_icon(DARK))
    # Raster-only sources (one per mode; rsvg has no prefers-color-scheme).
    src = {
        "mark-24-light.svg": mark24(LIGHT), "mark-24-dark.svg": mark24(DARK),
        "mark-24-hinted-light.svg": favicon(24, HINT24, LIGHT), "mark-24-hinted-dark.svg": favicon(24, HINT24, DARK),
        "favicon-16-light.svg": favicon(16, FAV16, LIGHT), "favicon-16-dark.svg": favicon(16, FAV16, DARK),
        "favicon-32-light.svg": favicon(32, FAV32, LIGHT), "favicon-32-dark.svg": favicon(32, FAV32, DARK),
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
                (f"{SOURCES}/mark-24-hinted-light.svg", 24, "brand-header24-light-8x.png"),
                (f"{SOURCES}/mark-24-hinted-dark.svg", 24, "brand-header24-dark-8x.png"),
                (f"{SOURCES}/mark-24-light.svg", 24, "brand-header24-exact-1x-light-8x.png"),
                (f"{SOURCES}/mark-24-light.svg", 48, "brand-header24-light-2xdpr-4x.png"),
                (f"{SOURCES}/mark-24-dark.svg", 48, "brand-header24-dark-2xdpr-4x.png"),
                (f"{STATIC}/app-icon.svg", 64, "brand-appicon64-light.png"),
                (f"{STATIC}/app-icon-dark.svg", 64, "brand-appicon64-dark.png"),
                (f"{SOURCES}/icon-maskable.svg", 64, "brand-maskable64-dark.png"),
            ]
            for svg, size, name in proofs:
                small = os.path.join(tmp, name)
                raster(svg, small, size)
                scale = "400%" if size == 48 else "800%"
                # Favicons and the header mark on their mode's page; app
                # icons on a neutral grey so the squircle's outline shows.
                bg = "#8A8A90" if "appicon" in name else "#12101A" if "dark" in name else "#FFFFFF"
                subprocess.run(["convert", small, "-background", bg, "-alpha", "remove", "-alpha", "off",
                                "-filter", "point", "-resize", scale, "-strip",
                                os.path.join(args.evidence, name)], check=True)


if __name__ == "__main__":
    main()
