"""Generate all Evey brand SVG assets with vectorized letterforms."""
import os

# Glyph paths from Archivo Black (unitsPerEm=1000, baseline at y=0, ascending up)
# We'll flip Y axis when rendering (SVG Y goes down)
GLYPHS = {
    'E': {'path': 'M74 688H669V523H295V428H615V270H295V165H676V0H74Z', 'advance': 722},
    'V': {'path': 'M261 0 17 688H253L391 228H395L534 688H760L517 0Z', 'advance': 778},
    'Y': {'path': 'M501 0H280V267L7 688H261L393 459H397L529 688H770L501 267Z', 'advance': 778},
}

# Visual params
LETTER_HEIGHT = 688  # cap height in font units
LETTER_SPACING = 100  # additional spacing between glyphs (font units)
CUT_HEIGHT_LARGE = 80   # bar thickness for wordmark (font units) ~= 8px @100px size
CUT_HEIGHT_SMALL = 40   # bar thickness for favicon ~= 4px @100px

RED = '#DC2626'
BLACK = '#0A0A0A'
WHITE = '#FFFFFF'

def letter_group(char, x_offset, color, scale_y=-1, baseline_y=688):
    """Render a glyph at given x offset. Flip Y because SVG is top-down."""
    glyph = GLYPHS[char]
    # Translate to position, flip Y so glyph appears upright
    return f'<path d="{glyph["path"]}" fill="{color}" transform="translate({x_offset}, {baseline_y}) scale(1, -1)"/>'

def build_wordmark(red_v=True, fg=BLACK, bg_for_cut=WHITE, cut_color=None):
    """Build EVEY wordmark in font-unit space.
    Returns: (svg_inner_content, total_width, total_height)
    cut_color: if None, the bar uses bg_for_cut (negative space). Otherwise specify color.
    """
    letters = ['E', 'V', 'E', 'Y']
    x = 0
    positions = []
    for ch in letters:
        positions.append((ch, x))
        x += GLYPHS[ch]['advance'] + LETTER_SPACING
    total_width = x - LETTER_SPACING  # remove trailing spacing

    parts = []
    for ch, xpos in positions:
        color = RED if (ch == 'V' and red_v) else fg
        parts.append(letter_group(ch, xpos, color))

    # Horizontal bar across all letters at mid-height
    bar_y = 344 - CUT_HEIGHT_LARGE / 2  # middle of 688
    bar_color = cut_color if cut_color else bg_for_cut
    bar = f'<rect x="0" y="{688 - bar_y - CUT_HEIGHT_LARGE}" width="{total_width}" height="{CUT_HEIGHT_LARGE}" fill="{bar_color}"/>'
    parts.append(bar)

    return '\n  '.join(parts), total_width, 688

def write_svg(filepath, inner, view_w, view_h, padding=40, bg=None):
    """Wrap inner content in SVG with optional background."""
    bg_rect = f'<rect width="100%" height="100%" fill="{bg}"/>' if bg else ''
    svg = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="-{padding} -{padding} {view_w + 2*padding} {view_h + 2*padding}" width="{view_w + 2*padding}" height="{view_h + 2*padding}">
  {bg_rect}
  {inner}
</svg>'''
    with open(filepath, 'w') as f:
        f.write(svg)
    print(f"Wrote {filepath}")

def write_svg_raw(filepath, content):
    with open(filepath, 'w') as f:
        f.write(content)
    print(f"Wrote {filepath}")

OUT = '/home/claude/evey-brand'
os.makedirs(f'{OUT}/svg', exist_ok=True)
os.makedirs(f'{OUT}/png', exist_ok=True)
os.makedirs(f'{OUT}/favicon', exist_ok=True)
os.makedirs(f'{OUT}/social', exist_ok=True)

# === 1. Wordmark color (red V on white bg) ===
inner, w, h = build_wordmark(red_v=True, fg=BLACK, bg_for_cut=WHITE)
write_svg(f'{OUT}/svg/evey-wordmark.svg', inner, w, h, padding=60)

# === 2. Wordmark mono (no red) ===
inner, w, h = build_wordmark(red_v=False, fg=BLACK, bg_for_cut=WHITE)
write_svg(f'{OUT}/svg/evey-wordmark-mono.svg', inner, w, h, padding=60)

# === 3. Wordmark inverted (white on black, red V) ===
inner, w, h = build_wordmark(red_v=True, fg=WHITE, bg_for_cut=BLACK)
write_svg(f'{OUT}/svg/evey-wordmark-inverted.svg', inner, w, h, padding=60, bg=BLACK)

# === 4. Wordmark inverted mono ===
inner, w, h = build_wordmark(red_v=False, fg=WHITE, bg_for_cut=BLACK)
write_svg(f'{OUT}/svg/evey-wordmark-inverted-mono.svg', inner, w, h, padding=60, bg=BLACK)

# === 5. Wordmark on transparent (red V, bar uses transparent / uses a "cut" approach via mask) ===
# For transparent background, we use a mask trick
def build_wordmark_transparent(red_v=True, fg=BLACK):
    letters = ['E', 'V', 'E', 'Y']
    x = 0
    positions = []
    for ch in letters:
        positions.append((ch, x))
        x += GLYPHS[ch]['advance'] + LETTER_SPACING
    total_width = x - LETTER_SPACING

    # Build mask: white = visible, black = cut
    mask_parts = ['<rect width="100%" height="100%" fill="white"/>']
    bar_y_top = 344 - CUT_HEIGHT_LARGE / 2
    mask_parts.append(f'<rect x="0" y="{688 - bar_y_top - CUT_HEIGHT_LARGE}" width="{total_width}" height="{CUT_HEIGHT_LARGE}" fill="black"/>')

    letter_parts = []
    for ch, xpos in positions:
        color = RED if (ch == 'V' and red_v) else fg
        letter_parts.append(letter_group(ch, xpos, color))

    svg = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="-60 -60 {total_width + 120} {688 + 120}" width="{total_width + 120}" height="{688 + 120}">
  <defs>
    <mask id="cutmask">
      {chr(10).join(mask_parts)}
    </mask>
  </defs>
  <g mask="url(#cutmask)">
    {chr(10).join(letter_parts)}
  </g>
</svg>'''
    return svg

svg_content = build_wordmark_transparent(red_v=True, fg=BLACK)
write_svg_raw(f'{OUT}/svg/evey-wordmark-transparent.svg', svg_content)

svg_content = build_wordmark_transparent(red_v=True, fg=WHITE)
write_svg_raw(f'{OUT}/svg/evey-wordmark-transparent-white.svg', svg_content)

# === 6. Favicon (V seul dans carré noir, V rouge, barre fine) ===
def build_favicon(size=512, bg=BLACK, v_color=RED, bar_color=BLACK, corner_radius_ratio=0.08):
    """Square favicon with V centered, horizontal cut bar."""
    radius = size * corner_radius_ratio
    # V glyph: width 778 in font units, height 688
    # Scale to occupy ~64% of size
    v_target_height = size * 0.64
    scale = v_target_height / 688
    v_scaled_width = 778 * scale
    v_x = (size - v_scaled_width) / 2
    v_y_top = (size - v_target_height) / 2

    # Bar across: at mid-V height, thickness = ~4% of size
    bar_h = max(2, size * 0.04)
    bar_y = size / 2 - bar_h / 2
    bar_padding_x = size * 0.14
    bar_x = bar_padding_x
    bar_w = size - 2 * bar_padding_x

    svg = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {size} {size}" width="{size}" height="{size}">
  <rect width="{size}" height="{size}" rx="{radius}" fill="{bg}"/>
  <g transform="translate({v_x}, {v_y_top + v_target_height}) scale({scale}, -{scale})">
    <path d="{GLYPHS['V']['path']}" fill="{v_color}"/>
  </g>
  <rect x="{bar_x}" y="{bar_y}" width="{bar_w}" height="{bar_h}" fill="{bar_color}"/>
</svg>'''
    return svg

# Multiple favicon variants
write_svg_raw(f'{OUT}/favicon/favicon.svg', build_favicon(512))
write_svg_raw(f'{OUT}/favicon/favicon-light.svg', build_favicon(512, bg=WHITE, v_color=RED, bar_color=WHITE))

# Apple touch icon (180px source, 1024 actual for retina)
write_svg_raw(f'{OUT}/favicon/apple-touch-icon.svg', build_favicon(1024, corner_radius_ratio=0.22))

# === 7. Social header (GitHub repo header, Twitter/X banner) ===
def build_social_header(width=1280, height=640, bg=BLACK, with_tagline=True):
    """GitHub social preview / Twitter banner."""
    # Wordmark in center
    letters = ['E', 'V', 'E', 'Y']
    glyph_unit_width = sum(GLYPHS[c]['advance'] for c in letters) + LETTER_SPACING * (len(letters) - 1)

    target_wordmark_width = width * 0.55
    scale = target_wordmark_width / glyph_unit_width

    wordmark_w = glyph_unit_width * scale
    wordmark_h = 688 * scale

    wm_x = (width - wordmark_w) / 2
    wm_y = (height - wordmark_h) / 2 - height * 0.04  # slight upward shift for tagline

    # Build wordmark inner
    x = 0
    parts = []
    for ch in letters:
        color = RED if ch == 'V' else WHITE
        parts.append(f'<path d="{GLYPHS[ch]["path"]}" fill="{color}" transform="translate({x}, 688) scale(1, -1)"/>')
        x += GLYPHS[ch]['advance'] + LETTER_SPACING

    bar_y_top_fontunit = 344 - CUT_HEIGHT_LARGE / 2
    bar_y = 688 - bar_y_top_fontunit - CUT_HEIGHT_LARGE
    parts.append(f'<rect x="0" y="{bar_y}" width="{glyph_unit_width}" height="{CUT_HEIGHT_LARGE}" fill="{bg}"/>')

    tagline_svg = ''
    if with_tagline:
        ty = wm_y + wordmark_h + height * 0.08
        tagline_svg = f'''<text x="{width/2}" y="{ty}" font-family="ui-monospace, 'JetBrains Mono', Menlo, monospace" font-size="{height * 0.032}" fill="white" opacity="0.55" text-anchor="middle" letter-spacing="3">SELF-HOSTED MEDIA SERVER</text>'''

    svg = f'''<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" width="{width}" height="{height}">
  <rect width="{width}" height="{height}" fill="{bg}"/>
  <g transform="translate({wm_x}, {wm_y}) scale({scale})">
    {chr(10).join(parts)}
  </g>
  {tagline_svg}
</svg>'''
    return svg

# GitHub social preview (1280x640 recommended)
write_svg_raw(f'{OUT}/social/github-social.svg', build_social_header(1280, 640))
write_svg_raw(f'{OUT}/social/github-social-light.svg', build_social_header(1280, 640, bg=WHITE).replace('fill="white" opacity="0.55"', 'fill="black" opacity="0.55"').replace('fill="' + WHITE + '"', f'fill="{BLACK}"'))

# Twitter/X header (1500x500)
write_svg_raw(f'{OUT}/social/twitter-header.svg', build_social_header(1500, 500))

# README hero banner (wide, slim)
write_svg_raw(f'{OUT}/social/readme-hero.svg', build_social_header(1600, 400))

# Open Graph / og:image (1200x630)
write_svg_raw(f'{OUT}/social/og-image.svg', build_social_header(1200, 630))

print("\n=== All SVG generated ===")
