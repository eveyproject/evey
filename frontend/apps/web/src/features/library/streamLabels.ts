import { formatChannelLayout, type V1MediaStream } from "@evey/api";

// Factual labels read straight off the probed streams - no prediction, no
// judgement about what playback will cost. The playback-difficulty badge that
// used to live here was removed: it reported the DECISION, not the PLAN, so a
// title with no keyframe index (or a VFR source, or a tight disk, or no free
// GPU slot) was announced as light and then transcoded anyway.

export function videoStream(streams?: V1MediaStream[]): V1MediaStream | undefined {
  return streams?.find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
}

// Mirrors internal/pkg/hdr DetectHDR, minus Dolby Vision: dovi_profile and
// dovi_bl_compat are not in the MediaStream proto, so a DoVi title reads as
// plain HDR10 here. The server still transcodes it (DoVi always tonemaps), and
// that shows up in transcode_reasons, so the BADGE stays right even though this
// label is coarse.
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

export function resolutionLabel(stream?: V1MediaStream): string | null {
  const h = stream?.height ?? 0;
  if (h <= 0) return null;
  if (h >= 2160) return "4K";
  if (h >= 1440) return "1440p";
  if (h >= 1080) return "1080p";
  if (h >= 720) return "720p";
  return `${h}p`;
}

// Null rather than "" because callers here skip the chip entirely; the shared
// formatter returns the empty string for the same case.
export function channelLabel(channels?: number): string | null {
  return formatChannelLayout(channels) || null;
}
