import type { V1MediaStream } from "@evey/api";

// The technical badges of a library card (resolution, HDR), shared by both
// televisions.
export function videoStream(streams?: V1MediaStream[]): V1MediaStream | undefined {
  return streams?.find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
}

export function isHdrStream(stream?: V1MediaStream): boolean {
  if (!stream) return false;
  const transfer = stream.colorTransfer?.trim().toLowerCase();
  if (transfer === "smpte2084" || transfer === "arib-std-b67") return true;
  const cs = stream.colorSpace?.trim().toLowerCase() ?? "";
  return cs.startsWith("bt2020") && (stream.bitDepth ?? 0) >= 10;
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
