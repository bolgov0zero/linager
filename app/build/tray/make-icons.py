#!/usr/bin/env python3
"""Makes the PNG files of the tray (run it when the logo or the icons change; the results are committed).

tray-template.png  the logo without the background, black: macOS draws a template image in the colour of the menu bar
tray-white.png     the logo, white: the tray of Windows and Linux on a dark taskbar
tray-black.png     the logo, dark: the same on a light taskbar
item-*.png         the icons of the connections in the menu (terminal, remote desktop, Assistant), grey: visible on both themes
"""
import re, subprocess, sys
from pathlib import Path
from PIL import Image

root = Path(__file__).resolve().parent
icons_ts = (root.parent.parent / "frontend/src/lib/icons.ts").read_text()

def logo(color: str, px: int = 44) -> bytes:
    # the 14x14 pixel logo of the application (web/public/icon.svg) without the background; the underscore is not blue here
    r = lambda x, y, w, h: f'<rect x="{x}" y="{y}" width="{w}" height="{h}" fill="{color}"/>'
    body = r(2, 3, 2, 2) + r(4, 5, 2, 2) + r(6, 7, 2, 2) + r(4, 9, 2, 2) + r(8, 10, 4, 2)
    svg = f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="1 1 12 12" shape-rendering="crispEdges" width="{px}" height="{px}">{body}</svg>'
    return subprocess.run(["rsvg-convert", "-w", str(px), "-h", str(px)], input=svg.encode(), capture_output=True, check=True).stdout

def lucide(name: str, color: str, px: int) -> bytes:
    m = re.search(r'"%s": "((?:[^"\\]|\\.)*)"' % re.escape(name), icons_ts)
    inner = m.group(1).replace('\\"', '"')
    svg = f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="{px}" height="{px}" fill="none" stroke="{color}" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">{inner}</svg>'
    return subprocess.run(["rsvg-convert", "-w", str(px), "-h", str(px)], input=svg.encode(), capture_output=True, check=True).stdout

(root / "tray-template.png").write_bytes(logo("#000000"))
(root / "tray-white.png").write_bytes(logo("#ffffff"))
(root / "tray-black.png").write_bytes(logo("#1a1c20"))
for name, icon in (("terminal", "square-terminal"), ("rdp", "monitor"), ("asst", "app-window")):
    (root / f"item-{name}.png").write_bytes(lucide(icon, "#8e8e93", 36))
for p in sorted(root.glob("*.png")):
    im = Image.open(p)
    print(p.name, im.size, im.mode)
