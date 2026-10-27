import type { VideoDecoderLimits } from "@evey/media-caps";

// What the device's video decoders can open, turned into the declaration the
// server judges direct play on (?pixelFormats=, ?maxHeight=, ?maxWidth= on
// the HLS master, the same fields on GetStreamInfo). Declared by codec name
// alone, a set whose decoder takes neither 4K nor 10 bits was handed such a
// film and could not open it.

// Below this the probe is nonsense (a decoder that opens nothing); the
// server ignores it too.
const MIN_PLAUSIBLE_EDGE = 240;

/**
 * The pixel formats to declare, one entry per codec the app offers: 8-bit
 * 4:2:0 for each, plus 10-bit for the codecs the decoders take at 10 bits.
 * Undefined when the limits are unknown, which keeps the server's built-in
 * rule (8-bit H.264, up to 10-bit HEVC/VP9/AV1).
 */
export function devicePixelFormats(limits: VideoDecoderLimits | undefined, videoCodecs: readonly string[]): string[] | undefined {
  if (!limits) return undefined;
  const out: string[] = [];
  for (const codec of videoCodecs) {
    out.push(`${codec}:yuv420p`);
    if (limits.tenBitCodecs.includes(codec)) out.push(`${codec}:yuv420p10`);
  }
  return out;
}

/** The decoders' height ceiling, when the probe gave a plausible one. */
export function decoderMaxHeight(limits: VideoDecoderLimits | undefined): number | undefined {
  return limits && limits.maxHeight >= MIN_PLAUSIBLE_EDGE ? limits.maxHeight : undefined;
}

/** The decoders' width ceiling, when the probe gave a plausible one. */
export function decoderMaxWidth(limits: VideoDecoderLimits | undefined): number | undefined {
  return limits && limits.maxWidth >= MIN_PLAUSIBLE_EDGE ? limits.maxWidth : undefined;
}

/** The tighter of two ceilings; an undefined one does not count. */
export function tighterOf(a: number | undefined, b: number | undefined): number | undefined {
  if (!a || a <= 0) return b && b > 0 ? b : undefined;
  if (!b || b <= 0) return a;
  return Math.min(a, b);
}
