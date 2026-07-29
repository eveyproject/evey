// Pure navigation/timing logic for the TV fullscreen photo slideshow
// (app/(auth)/photos/slideshow.tsx). Kept separate from the screen so the
// index bounds can be unit-tested without rendering the Image/TVEventHandler
// wiring.

// A slow-enough cadence to actually look at a photo from across the room,
// fast enough that a party doesn't stall on one picture. Matches the kind of
// pace consumer photo-frame apps default to.
export const SLIDESHOW_AUTOPLAY_INTERVAL_MS = 6000;

// Manual D-pad navigation clamps at both ends (mirrors the mobile lightbox's
// prevLightboxIndex/nextLightboxIndex): stepping past an end would otherwise
// silently jump to the opposite photo, which reads as a random jump rather
// than "there is nothing more this way".
export function prevSlideshowIndex(index: number, total: number): number {
  if (total <= 0) return index;
  return index > 0 ? index - 1 : index;
}

export function nextSlideshowIndex(index: number, total: number): number {
  if (total <= 0) return index;
  return index < total - 1 ? index + 1 : index;
}

// Bounds an arbitrary starting index (e.g. one carried across a route param)
// into [0, total-1]. total<=0 (empty album) always resolves to 0 - the
// screen is responsible for showing its own empty state rather than reading
// out of bounds.
export function clampSlideshowIndex(index: number, total: number): number {
  if (total <= 0) return 0;
  if (index < 0) return 0;
  if (index > total - 1) return total - 1;
  return index;
}

// Autoplay advances differently from a manual key press: reaching the last
// photo loops back to the first rather than stopping, so a slideshow left
// running at a gathering keeps cycling instead of freezing on the last
// frame. Empty albums stay at 0 (the screen won't be running a timer for one
// anyway, this just keeps the function total).
export function nextAutoplayIndex(index: number, total: number): number {
  if (total <= 0) return 0;
  return (index + 1) % total;
}
