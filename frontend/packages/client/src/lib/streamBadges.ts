import { formatChannelLayout, type V1MediaStream } from "@evey/api";

// The factual labels of a title's streams (resolution, HDR flavour, channels),
// read straight off the probe - no prediction of what playback will cost.
// Shared by the four clients: the web, the phone's library and title pages
// each kept a copy, and the phone's library card read a 576p file as "SD"
// where every other surface said "576p".
export function videoStream(streams?: V1MediaStream[]): V1MediaStream | undefined {
  return streams?.find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
}

// Mirrors internal/pkg/hdr DetectHDR, minus Dolby Vision: dovi_profile and
// dovi_bl_compat are not in the MediaStream proto, so a DoVi title reads as
// plain HDR10 here. The server still transcodes it (DoVi always tonemaps), and
// that shows up in transcode_reasons, so the badge stays right even though
// this label is coarse.
export function hdrLabel(stream?: V1MediaStream): string | null {
  if (!stream) return null;
  switch (stream.colorTransfer?.trim().toLowerCase()) {
    case "smpte2084":
      return "HDR10";
    case "arib-std-b67":
      return "HLG";
  }
  // Legacy rows probed before color_transfer was stored: BT.2020 + 10-bit is
  // HDR, but the two flavours can't be told apart without the transfer, so the
  // server reports hdr10 as the conservative default. Match it.
  const cs = stream.colorSpace?.trim().toLowerCase() ?? "";
  if (cs.startsWith("bt2020") && (stream.bitDepth ?? 0) >= 10) return "HDR10";
  return null;
}

export function isHdrStream(stream?: V1MediaStream): boolean {
  return hdrLabel(stream) !== null;
}

// The standard tier a frame belongs to (2160, 1440, 1080, 720), or 0 below
// 720p. The width counts as much as the height: a scope film cropped to
// 1920x818 is a 1080p file, and a label read off its height said "818p" on
// one screen and "720p" on another. Slack on the width absorbs encodes that
// shave a few columns (1916x798).
export function resolutionTier(width?: number, height?: number): number {
  const w = width ?? 0;
  const h = height ?? 0;
  const byHeight = h >= 2160 ? 2160 : h >= 1440 ? 1440 : h >= 1080 ? 1080 : h >= 720 ? 720 : 0;
  const byWidth = w >= 3800 ? 2160 : w >= 2500 ? 1440 : w >= 1880 ? 1080 : w >= 1240 ? 720 : 0;
  return Math.max(byHeight, byWidth);
}

// The ONE resolution label every surface prints: "4K", "1440p", "1080p",
// "720p", and below that the height ("576p").
export function resolutionLabel(stream?: Pick<V1MediaStream, "width" | "height">): string | null {
  const h = stream?.height ?? 0;
  if (h <= 0) return null;
  const tier = resolutionTier(stream?.width, h);
  if (tier === 2160) return "4K";
  if (tier > 0) return `${tier}p`;
  return `${h}p`;
}

// The same tiers as filter bands, with everything under 720p as "SD".
export const RESOLUTION_BANDS = ["4K", "1440p", "1080p", "720p", "SD"] as const;
export type ResolutionBand = (typeof RESOLUTION_BANDS)[number];

export function resolutionBand(width?: number, height?: number): ResolutionBand {
  const tier = resolutionTier(width, height);
  if (tier === 2160) return "4K";
  if (tier > 0) return `${tier}p` as ResolutionBand;
  return "SD";
}

// Null rather than "" so callers skip the chip entirely; the shared formatter
// returns the empty string for the same case.
export function channelLabel(channels?: number): string | null {
  return formatChannelLayout(channels) || null;
}
