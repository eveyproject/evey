// The quality rungs a transcode actually offers, read from the master
// playlist, plus the rounding that turns an encoded height into a label a
// viewer recognises.
//
// The web learns its rungs from hls.js, which hands them over on
// MANIFEST_PARSED. The native players do not: expo-video drives HLS itself and
// exposes no level list, so the mobile quality menu shipped a FIXED ladder -
// 1080/720/480/360 - and offered 1080p on a 720p source, then quietly served
// whatever the server had. The manifest is the answer both platforms already
// agree on; on mobile it just has to be read directly.
//
// Parsing is deliberately narrow: one attribute of one tag. Anything else in
// the playlist is somebody else's business.

const QUALITY_TIERS = [360, 480, 720, 1080, 1440, 2160];

// The standard tier a frame belongs to (2160, 1440, 1080, 720), or 0 below
// 720p. The width counts as much as the height: a scope film cropped to
// 1920x818 is a 1080p file, and a label read off its height said "818p" on
// one screen and "720p" on another. Slack on the width absorbs encodes that
// shave a few columns (1916x798). The title page and the player's quality menu
// both name a frame through this, so they cannot disagree.
export function resolutionTier(width?: number, height?: number): number {
  const w = width ?? 0;
  const h = height ?? 0;
  const byHeight = h >= 2160 ? 2160 : h >= 1440 ? 1440 : h >= 1080 ? 1080 : h >= 720 ? 720 : 0;
  const byWidth = w >= 3800 ? 2160 : w >= 2500 ? 1440 : w >= 1880 ? 1080 : w >= 1240 ? 720 : 0;
  return Math.max(byHeight, byWidth);
}

// nominalQualityTier maps an ENCODED rung to the tier a viewer recognises.
// With its width, the rung is named like the title page names the file: the
// 1920x818 rung of a scope film is "1080p" there and must be here too - from
// the height alone it rounded to 720p (demo, 2026-10-01). Without a width the
// height rounds to the nearest tier. Display only; playback and rung selection
// still key off the exact height.
export function nominalQualityTier(height: number, width?: number): number {
  if (!height || height <= 0) return 0;
  const nearest = QUALITY_TIERS.reduce((best, tier) =>
    Math.abs(tier - height) < Math.abs(best - height) ? tier : best,
  );
  return width && width > 0 ? Math.max(nearest, resolutionTier(width, height)) : nearest;
}

// A rung as the master declares it. The height alone is accepted where the
// width is unknown.
export interface RungSize {
  width?: number;
  height: number;
}
export type QualityLevel = number | RungSize;

// parseHlsMasterHeights lists the heights declared by a master playlist, in the
// order they appear. A variant without RESOLUTION contributes nothing: it is
// usually the audio-only rendition, which is not a quality choice.
//
// Returns an empty list for anything that is not a master playlist - a media
// playlist, an error page, an empty body - so a caller that fetched the wrong
// thing falls back to its static ladder instead of showing an empty menu.
export function parseHlsMasterHeights(manifest: string): number[] {
  return parseHlsMasterResolutions(manifest).map((r) => r.height);
}

// parseHlsMasterResolutions is parseHlsMasterHeights with each rung's width,
// which its label needs.
export function parseHlsMasterResolutions(manifest: string): Required<RungSize>[] {
  if (!manifest) return [];
  const out: Required<RungSize>[] = [];
  for (const line of manifest.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed.startsWith("#EXT-X-STREAM-INF")) continue;
    // RESOLUTION=<width>x<height>, unquoted per RFC 8216.
    const m = /RESOLUTION=(\d+)x(\d+)/i.exec(trimmed);
    if (!m) continue;
    const width = Number(m[1]);
    const height = Number(m[2]);
    if (Number.isFinite(height) && height > 0) out.push({ width: Number.isFinite(width) ? width : 0, height });
  }
  return out;
}

export interface QualityRung {
  /** The exact encoded height to ask the server for. */
  value: number;
  /** What the viewer reads: the rounded tier. */
  label: string;
}

// qualityRungsFromHeights turns declared heights into the menu, tallest first,
// one entry per tier. Two rungs that round to the same label (1080p at two
// bitrates) would be indistinguishable on screen, so only the taller survives -
// a menu with two identical labels is a menu nobody can use.
//
// Returns an empty list when there is nothing to offer, which is the caller's
// signal to keep its static ladder rather than render an empty menu.
export function qualityRungsFromHeights(levels: readonly QualityLevel[]): QualityRung[] {
  const seen = new Set<number>();
  return levels
    .map((l) => (typeof l === "number" ? { width: 0, height: l } : { width: l.width ?? 0, height: l.height }))
    .filter(({ height }) => Number.isFinite(height) && height > 0)
    .sort((a, b) => b.height - a.height)
    .map(({ width, height }) => ({ tier: nominalQualityTier(height, width), h: height }))
    .filter(({ tier }) => (seen.has(tier) ? false : (seen.add(tier), true)))
    .map(({ tier, h }) => ({ value: h, label: `${tier}p` }));
}
