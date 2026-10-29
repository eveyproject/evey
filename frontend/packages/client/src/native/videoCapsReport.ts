import type { VideoDecoderLimits } from "@evey/media-caps";

/** What the device's video decoders declared at launch, for the video_caps beacon. */
export interface VideoCapsReport {
  maxH264Level: number | undefined;
  limits: VideoDecoderLimits | undefined;
}

/**
 * One line for the server log: the H.264 level ceiling, the largest frame,
 * the 10-bit codecs, then per codec its largest frame and each size-rate
 * probe ("2160p60=no"). Diagnostic only: this is what a decoder error is read
 * against, never what the server decides from.
 */
export function formatVideoCaps(r: VideoCapsReport): string {
  const parts = [`h264_level=${r.maxH264Level ?? "unknown"}`];
  const l = r.limits;
  if (!l) return [...parts, "limits=unknown"].join(" ");
  parts.push(`max=${l.maxWidth}x${l.maxHeight}`, `10bit=${l.tenBitCodecs.join(",") || "none"}`);
  for (const [codec, c] of Object.entries(l.codecs)) {
    const rates = Object.entries(c.sizeRate ?? {}).map(([probe, ok]) => `${probe}=${ok ? "yes" : "no"}`);
    parts.push(`| ${codec} ${c.maxWidth}x${c.maxHeight}${rates.length ? " " + rates.join(" ") : ""}`);
  }
  return parts.join(" ");
}
