#!/usr/bin/env python3
"""Generate web-friendly wallpaper copies for the frontend.

The source files in ``wallpaper/`` are extremely large (some > 40 MB, up to
9622x7463). Serving those directly would tank first paint. This script emits
two derivatives per source into ``web/public/wallpaper``:

  * ``<slug>.jpg``       – full hero (max width 2400, progressive JPEG)
  * ``<slug>-thumb.jpg`` – picker thumbnail (max width 360)

Source files are never modified. Re-run any time new art lands in wallpaper/.
"""
from __future__ import annotations

import os
import re
import sys

from PIL import Image, ImageOps

SRC = "wallpaper"
DST = "web/public/wallpaper"
MAX_W = 2400
THUMB_W = 360
QUALITY = 82
THUMB_QUALITY = 72

# Friendly slug mapping (source filename -> slug). Keep these stable; the
# frontend wallpaper registry references these slugs.
MAPPING = {
    "foggy forest_1501395260_.png": "foggy-forest",
    "golden hour.png": "golden-hour",
    "city_in_the_clouds_by_tatasz_d8yebbu.png": "city-in-the-clouds",
    "Summer sea.png": "summer-sea",
    "Petals Of The Moon [4K].png": "petals-of-the-moon",
    "sunset-ocean-beautiful-scenery-4k-wallpaper-3840x.jpg": "sunset-ocean",
    "kevin-laminto-HxCl2w7pKy0-unsplash (1)_1501396221.jpg": "mountain-glow",
    "春天的花园.jpg": "spring-garden",
    "日出印象-高清版.jpg": "impression-sunrise",
    "塞纳河畔的春天.jpg": "seine-spring",
}

# Human-readable labels shown in the wallpaper picker.
LABELS = {
    "foggy-forest": "Foggy Forest",
    "golden-hour": "Golden Hour",
    "city-in-the-clouds": "City in the Clouds",
    "summer-sea": "Summer Sea",
    "petals-of-the-moon": "Petals of the Moon",
    "sunset-ocean": "Sunset Ocean",
    "mountain-glow": "Mountain Glow",
    "spring-garden": "Spring Garden",
    "impression-sunrise": "Impression, Sunrise",
    "seine-spring": "Spring by the Seine",
}


def slugify(name: str) -> str:
    s = re.sub(r"\s+", "-", name.strip().lower())
    s = re.sub(r"[^a-z0-9\-]", "", s)
    return re.sub(r"-+", "-", s).strip("-")


def save(im: Image.Image, path: str, max_w: int, quality: int) -> tuple[int, int]:
    im = ImageOps.exif_transpose(im)
    if im.mode not in ("RGB", "L"):
        im = im.convert("RGB")
    elif im.mode == "P":
        im = im.convert("RGB")
    w, h = im.size
    if w > max_w:
        nh = max(1, round(h * max_w / w))
        im = im.resize((max_w, nh), Image.LANCZOS)
    im.save(path, "JPEG", quality=quality, optimize=True, progressive=True)
    return im.size


def main() -> int:
    os.makedirs(DST, exist_ok=True)
    if not os.path.isdir(SRC):
        print(f"error: source dir {SRC!r} not found", file=sys.stderr)
        return 1

    found = sorted(os.listdir(SRC))
    produced = []
    for fname in found:
        slug = MAPPING.get(fname)
        if slug is None:
            base = os.path.splitext(fname)[0]
            slug = slugify(base)
            print(f"  (unmapped) {fname} -> {slug}")
        src_path = os.path.join(SRC, fname)
        if not os.path.isfile(src_path):
            continue
        try:
            im = Image.open(src_path)
        except Exception as exc:  # noqa: BLE001
            print(f"  skip {fname}: {exc}", file=sys.stderr)
            continue
        full = os.path.join(DST, f"{slug}.jpg")
        thumb = os.path.join(DST, f"{slug}-thumb.jpg")
        size = save(im, full, MAX_W, QUALITY)
        save(im, thumb, THUMB_W, THUMB_QUALITY)
        kb = os.path.getsize(full) / 1024
        print(f"  {fname} -> {slug}.jpg {size[0]}x{size[1]} {kb:.0f}KB")
        produced.append(slug)

    print(f"\nGenerated {len(produced)} wallpapers into {DST}/")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
