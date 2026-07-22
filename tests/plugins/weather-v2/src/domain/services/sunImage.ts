// Sanity gate for the LIVE sun image.
//
// SDO's "latest" endpoint sometimes serves an essentially black frame (Earth
// eclipse season, lunar transit, calibration): a perfectly valid JPEG whose
// only bright pixels are the white caption text. The backend can only check
// the JPEG magic (no decoder there), so a dark frame reaches the frontend -
// which then textured the sun disc with black (bug seen 2026-07-22). The
// frontend decodes the image anyway, so brightness is judged here and a dark
// frame is dropped in favour of the bundled/previous skin.
//
// File size cannot discriminate: the black 1024px frame (~24 KB) is BIGGER
// than the tight-cropped bundled skin (~20 KB).
//
// Pure domain: operates on raw RGBA bytes, no DOM/canvas.

// Below this mean luminance a frame cannot be a visible sun disc. A normal
// full-disc AIA frame averages ~0.25 (bright disc on black background); a
// 90%-occluded disc still reaches ~0.04 only. Dark frames sit near 0.004.
export const SUN_FRAME_MIN_LUMA = 0.05;

// Mean Rec. 709 luminance of an RGBA pixel buffer, 0..1. Alpha is ignored:
// the JPEG source is always opaque, and the check must not be spoofable by
// transparency.
export function meanLuma(rgba: Uint8ClampedArray | Uint8Array): number {
  const pixels = rgba.length >> 2;
  if (pixels === 0) return 0;
  let sum = 0;
  for (let i = 0; i < pixels; i++) {
    const o = i * 4;
    sum += 0.2126 * rgba[o] + 0.7152 * rgba[o + 1] + 0.0722 * rgba[o + 2];
  }
  return sum / (pixels * 255);
}

export function isUsableSunFrame(luma: number): boolean {
  return luma >= SUN_FRAME_MIN_LUMA;
}
