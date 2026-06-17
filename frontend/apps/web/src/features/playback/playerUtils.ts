export function formatTime(seconds: number): string {
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

const QUALITY_TIERS = [360, 480, 720, 1080, 1440, 2160];

// nominalQualityTier maps an ENCODED height to the nearest standard quality
// tier. A widescreen master (e.g. 2.40:1) is scaled to a fixed rung WIDTH with
// the height left to preserve aspect, so the "1080p" rung of a 1920×800 film is
// actually 1920×798 — and the raw height (798p) reads as a bug to users. Round
// it to the nearest familiar label (798 → 720p) for display only; playback and
// rung selection still key off the exact height.
export function nominalQualityTier(height: number): number {
  if (!height || height <= 0) return 0;
  return QUALITY_TIERS.reduce((best, tier) =>
    Math.abs(tier - height) < Math.abs(best - height) ? tier : best,
  );
}
