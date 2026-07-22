// Backend-side brightness gate for the live SDO sun image.
//
// SDO's "latest" endpoint sometimes serves an essentially black frame (Earth
// eclipse season, lunar transit, calibration): a perfectly valid JPEG whose
// only bright pixels are the caption text. This is THE authoritative gate,
// running before the frame is persisted: the frontend has a mirror check on
// decoded pixels (src/domain/services/sunImage.ts), but Firefox with
// privacy.resistFingerprinting poisons canvas readbacks with noise and blinds
// it - reproduced 2026-07-22, black disc in a golden halo. Server-side decode
// (jpeg-js, vendored pure-JS) cannot be defeated by browser privacy settings.
//
// Keep SUN_FRAME_MIN_LUMA in sync with src/domain/services/sunImage.ts
// (asserted by tests/sunFrameGuard.test.ts).

const jpeg = require("jpeg-js");

// Below this mean Rec. 709 luminance a frame cannot be a visible sun disc. A
// normal full-disc AIA frame averages ~0.25; dark frames sit near 0.004.
const SUN_FRAME_MIN_LUMA = 0.05;

// Mean Rec. 709 luminance (0..1) of an RGBA buffer.
function meanLumaRGBA(data) {
  const pixels = data ? data.length >> 2 : 0;
  if (pixels === 0) return null;
  let sum = 0;
  for (let i = 0; i < pixels; i++) {
    const o = i * 4;
    sum += 0.2126 * data[o] + 0.7152 * data[o + 1] + 0.0722 * data[o + 2];
  }
  return sum / (pixels * 255);
}

// Mean luminance (0..1) of a JPEG buffer, or null when it cannot be decoded.
// The 64 MB decode cap covers any plausible full-disc frame (1024² RGBA is
// 4 MB) while bounding a hostile/corrupt stream.
function sunFrameLuma(buf) {
  try {
    const raw = jpeg.decode(buf, { useTArray: true, maxMemoryUsageInMB: 64 });
    return meanLumaRGBA(raw && raw.data);
  } catch {
    return null;
  }
}

function isUsableSunFrame(luma) {
  return luma !== null && luma >= SUN_FRAME_MIN_LUMA;
}

// ---- geometry normalization ------------------------------------------------
//
// SDO full-disc frames keep the solar limb at ~0.78 of the half-width (plus a
// caption strip), while the bundled skin is cropped tight with the limb at
// ~0.92 - and the frontend shader maps the texture assuming THAT geometry
// (limb ≈ 0.94). Serving the raw full-disc frame put the limb inside the
// shader's edge-fade band: dark limb patches punched visible bites into the
// disc ("nibbled border", reported 2026-07-22). The frame is therefore cropped
// here so its limb lands where the bundled skin's does, then resampled to the
// bundled skin's 384px (same GPU memory footprint on every client).

const TARGET_LIMB_FRACTION = 0.93; // where the bundled skin keeps its limb
const OUTPUT_SIZE = 384;           // bundled skin dimensions
const FALLBACK_LIMB_FRACTION = 0.78; // canonical SDO latest_1024 geometry

function lumaAt(raw, x, y) {
  const o = (y * raw.width + x) * 4;
  return 0.2126 * raw.data[o] + 0.7152 * raw.data[o + 1] + 0.0722 * raw.data[o + 2];
}

// Mean luminance over a ring at radius fraction f of the half-width.
function ringLuma(raw, f) {
  const cx = raw.width / 2;
  const cy = raw.height / 2;
  let sum = 0;
  const N = 128;
  for (let a = 0; a < N; a++) {
    const th = (a / N) * 2 * Math.PI;
    const x = Math.min(raw.width - 1, Math.max(0, Math.round(cx + Math.cos(th) * f * cx)));
    const y = Math.min(raw.height - 1, Math.max(0, Math.round(cy + Math.sin(th) * f * cy)));
    sum += lumaAt(raw, x, y);
  }
  return sum / N;
}

// Locate the limb as the OUTERMOST radius still within 95% of the peak ring
// brightness. AIA's limb brightening makes the peak sit right on the limb;
// for a flat disc the plateau's outer edge is the limb. Averaging 128 angles
// keeps single coronal loops or the caption text from skewing the estimate.
function findLimbFraction(raw) {
  let max = 0;
  const samples = [];
  for (let f = 0.55; f <= 1.001; f += 0.01) {
    const v = ringLuma(raw, f);
    samples.push([f, v]);
    if (v > max) max = v;
  }
  let limb = 0;
  for (const [f, v] of samples) {
    if (v >= max * 0.95) limb = f;
  }
  // Outside the plausible band means the probe latched onto something that is
  // not a centered disc (odd framing, instrument anomaly): fall back to the
  // canonical SDO geometry rather than producing a wild crop.
  if (limb < 0.6 || limb > 0.99) return FALLBACK_LIMB_FRACTION;
  return limb;
}

// Bilinear sample of the source at fractional coordinates.
function bilinear(raw, fx, fy, c) {
  const x0 = Math.max(0, Math.min(raw.width - 1, Math.floor(fx)));
  const y0 = Math.max(0, Math.min(raw.height - 1, Math.floor(fy)));
  const x1 = Math.min(raw.width - 1, x0 + 1);
  const y1 = Math.min(raw.height - 1, y0 + 1);
  const dx = fx - x0;
  const dy = fy - y0;
  const at = (x, y) => raw.data[(y * raw.width + x) * 4 + c];
  return (
    at(x0, y0) * (1 - dx) * (1 - dy) +
    at(x1, y0) * dx * (1 - dy) +
    at(x0, y1) * (1 - dx) * dy +
    at(x1, y1) * dx * dy
  );
}

// Full pipeline: decode, dark-gate, crop the limb onto the bundled-skin
// geometry, resample to OUTPUT_SIZE, re-encode. Returns {ok:true, jpeg} or
// {ok:false, luma} (luma null when the buffer does not even decode).
function prepareSunFrame(buf) {
  let raw;
  try {
    raw = jpeg.decode(buf, { useTArray: true, maxMemoryUsageInMB: 64 });
  } catch {
    return { ok: false, luma: null };
  }
  if (!raw || !raw.data || raw.data.length < 4) return { ok: false, luma: null };
  const luma = meanLumaRGBA(raw.data);
  if (!isUsableSunFrame(luma)) return { ok: false, luma };

  const limb = findLimbFraction(raw);
  // Crop half-size that moves the limb to TARGET_LIMB_FRACTION, never
  // zooming OUT (scale capped at 1: an already-tight frame stays as is).
  const scale = Math.min(1, limb / TARGET_LIMB_FRACTION);
  const cx = raw.width / 2;
  const cy = raw.height / 2;
  const half = Math.min(cx, cy) * scale;

  const out = Buffer.alloc(OUTPUT_SIZE * OUTPUT_SIZE * 4);
  for (let y = 0; y < OUTPUT_SIZE; y++) {
    for (let x = 0; x < OUTPUT_SIZE; x++) {
      const fx = cx - half + ((x + 0.5) / OUTPUT_SIZE) * 2 * half;
      const fy = cy - half + ((y + 0.5) / OUTPUT_SIZE) * 2 * half;
      const o = (y * OUTPUT_SIZE + x) * 4;
      out[o] = bilinear(raw, fx, fy, 0);
      out[o + 1] = bilinear(raw, fx, fy, 1);
      out[o + 2] = bilinear(raw, fx, fy, 2);
      out[o + 3] = 255;
    }
  }
  const jpegOut = jpeg.encode({ data: out, width: OUTPUT_SIZE, height: OUTPUT_SIZE }, 88);
  return { ok: true, jpeg: Buffer.from(jpegOut.data) };
}

module.exports = { sunFrameLuma, isUsableSunFrame, SUN_FRAME_MIN_LUMA, prepareSunFrame };
