"""Convert all SVG to PNG at multiple sizes."""
import cairosvg
import os

OUT = '/home/claude/evey-brand'

# Wordmark PNG at various widths
WORDMARK_SIZES = [400, 800, 1200, 2400, 4000]
WORDMARK_SVGS = [
    ('evey-wordmark', 'svg/evey-wordmark.svg', 'png'),
    ('evey-wordmark-mono', 'svg/evey-wordmark-mono.svg', 'png'),
    ('evey-wordmark-inverted', 'svg/evey-wordmark-inverted.svg', 'png'),
    ('evey-wordmark-inverted-mono', 'svg/evey-wordmark-inverted-mono.svg', 'png'),
    ('evey-wordmark-transparent', 'svg/evey-wordmark-transparent.svg', 'png'),
    ('evey-wordmark-transparent-white', 'svg/evey-wordmark-transparent-white.svg', 'png'),
]

for name, svg_path, out_dir in WORDMARK_SVGS:
    for w in WORDMARK_SIZES:
        out = f'{OUT}/{out_dir}/{name}-{w}.png'
        cairosvg.svg2png(url=f'{OUT}/{svg_path}', write_to=out, output_width=w)
        print(f'Wrote {out}')

# Favicons at standard sizes
FAVICON_SIZES = [16, 32, 48, 64, 96, 128, 192, 256, 512]
for size in FAVICON_SIZES:
    out = f'{OUT}/favicon/favicon-{size}.png'
    cairosvg.svg2png(url=f'{OUT}/favicon/favicon.svg', write_to=out, output_width=size, output_height=size)
    print(f'Wrote {out}')

# Apple touch icon (180px)
cairosvg.svg2png(url=f'{OUT}/favicon/apple-touch-icon.svg', write_to=f'{OUT}/favicon/apple-touch-icon-180.png', output_width=180, output_height=180)
# Android chrome icons
cairosvg.svg2png(url=f'{OUT}/favicon/favicon.svg', write_to=f'{OUT}/favicon/android-chrome-192.png', output_width=192, output_height=192)
cairosvg.svg2png(url=f'{OUT}/favicon/favicon.svg', write_to=f'{OUT}/favicon/android-chrome-512.png', output_width=512, output_height=512)

# Social images
SOCIAL = [
    ('github-social', 1280, 640),
    ('github-social-light', 1280, 640),
    ('twitter-header', 1500, 500),
    ('readme-hero', 1600, 400),
    ('og-image', 1200, 630),
]
for name, w, h in SOCIAL:
    out = f'{OUT}/social/{name}.png'
    cairosvg.svg2png(url=f'{OUT}/social/{name}.svg', write_to=out, output_width=w, output_height=h)
    print(f'Wrote {out}')

# 2x retina versions for social
for name, w, h in SOCIAL:
    out = f'{OUT}/social/{name}@2x.png'
    cairosvg.svg2png(url=f'{OUT}/social/{name}.svg', write_to=out, output_width=w*2, output_height=h*2)
    print(f'Wrote {out}')

print('\n=== All PNG generated ===')
