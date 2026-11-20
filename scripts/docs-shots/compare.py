"""Turns the screenshots into the WebP images of docs/images and says which changed.

Usage: python3 compare.py <png dir> <webp out dir> [docs/images]

Each <name>.png becomes <out>/<name>.webp: a uniform band at the bottom of a
sparse page is trimmed down to a margin, then WebP quality 82. The new image
is compared with docs/images/<name>.webp: a clock, a CPU tile or a fresh
weather picture always moves a few pixels, so an image counts as changed only
when its size differs or when more than CHANGED_SHARE of its pixels moved
visibly. The list goes to <out>/changed.txt and to the job summary when run
in GitHub Actions.
"""

import os
import sys
from pathlib import Path

from PIL import Image, ImageChops

# Share of pixels that must differ by more than PIXEL_DELTA for an image to count as changed.
CHANGED_SHARE = 0.02
PIXEL_DELTA = 24


def trimmed(im):
    w, h = im.size
    bg = im.getpixel((w // 2, h - 2))
    px = im.load()

    def uniform(row):
        for x in range(12, w - 12, 7):
            c = px[x, row]
            if max(abs(c[i] - bg[i]) for i in range(3)) > 6:
                return False
        return True

    y = h - 1
    while y > 0 and uniform(y):
        y -= 1
    bottom = min(h, y + 1 + 64)
    return im.crop((0, 0, w, bottom)) if bottom < h else im


def moved_share(a, b):
    diff = ImageChops.difference(a, b).convert("L").point(lambda v: 255 if v > PIXEL_DELTA else 0)
    return diff.histogram()[255] / (a.size[0] * a.size[1])


def main():
    src, dst = Path(sys.argv[1]), Path(sys.argv[2])
    docs = Path(sys.argv[3]) if len(sys.argv) > 3 else Path("docs/images")
    dst.mkdir(parents=True, exist_ok=True)
    changed, lines = [], []
    for png in sorted(src.glob("*.png")):
        im = trimmed(Image.open(png).convert("RGB"))
        out = dst / (png.stem + ".webp")
        im.save(out, "WEBP", quality=82, method=6)
        old_path = docs / out.name
        if not old_path.exists():
            verdict = "new"
        else:
            old = Image.open(old_path).convert("RGB")
            if old.size != im.size:
                verdict = f"size {old.size[0]}x{old.size[1]} -> {im.size[0]}x{im.size[1]}"
            else:
                share = moved_share(old, Image.open(out).convert("RGB"))
                verdict = f"{share:.1%} of pixels moved" if share > CHANGED_SHARE else ""
        if verdict:
            changed.append(out.name)
        lines.append(f"| `{out.name}` | {verdict or 'unchanged'} |")
        print(f"{out.name}: {verdict or 'unchanged'}")
    (dst / "changed.txt").write_text("".join(n + "\n" for n in changed))
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as f:
            f.write(f"### Documentation screenshots: {len(changed)} changed\n\n| Image | Verdict |\n|---|---|\n")
            f.write("\n".join(lines) + "\n")


if __name__ == "__main__":
    main()
