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

// nominalQualityTier maps an ENCODED height to the nearest standard tier. A
// widescreen master (2.40:1, say) is scaled to a fixed rung WIDTH with the
// height left to preserve aspect, so the "1080p" rung of a 1920x800 film is
// really 1920x798 - and 798p reads as a bug to a viewer. Round it for display
// only; playback and rung selection still key off the exact height.
export function nominalQualityTier(height: number): number {
  if (!height || height <= 0) return 0;
  return QUALITY_TIERS.reduce((best, tier) =>
    Math.abs(tier - height) < Math.abs(best - height) ? tier : best,
  );
}

// parseHlsMasterHeights lists the heights declared by a master playlist, in the
// order they appear. A variant without RESOLUTION contributes nothing: it is
// usually the audio-only rendition, which is not a quality choice.
//
// Returns an empty list for anything that is not a master playlist - a media
// playlist, an error page, an empty body - so a caller that fetched the wrong
// thing falls back to its static ladder instead of showing an empty menu.
export function parseHlsMasterHeights(manifest: string): number[] {
  if (!manifest) return [];
  const out: number[] = [];
  for (const line of manifest.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed.startsWith("#EXT-X-STREAM-INF")) continue;
    // RESOLUTION=<width>x<height>, unquoted per RFC 8216, and the height is
    // what a rung is named after.
    const m = /RESOLUTION=(\d+)x(\d+)/i.exec(trimmed);
    if (!m) continue;
    const height = Number(m[2]);
    if (Number.isFinite(height) && height > 0) out.push(height);
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
export function qualityRungsFromHeights(heights: readonly number[]): QualityRung[] {
  const seen = new Set<number>();
  return [...heights]
    .filter((h) => Number.isFinite(h) && h > 0)
    .sort((a, b) => b - a)
    .map((h) => ({ tier: nominalQualityTier(h), h }))
    .filter(({ tier }) => (seen.has(tier) ? false : (seen.add(tier), true)))
    .map(({ tier, h }) => ({ value: h, label: `${tier}p` }));
}
