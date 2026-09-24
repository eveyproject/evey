import type { V1MediaStream, V1StreamInfo } from "@evey/api";

// The variant the player is decoding right now, read off its engine (hls.js
// level, expo-video track, the <video> element). The negotiated StreamInfo
// describes the plan at start; on an ABR ladder, in auto or on a picked
// quality, the rung playing is another one.
export interface PlayingVariant {
  width?: number | null;
  height?: number | null;
  // Advertised BANDWIDTH of the variant, bps.
  bitrate?: number | null;
  frameRate?: number | null;
  // The variant's URL as the master lists it.
  url?: string | null;
}

// The audio muxed in the variant, when the master names it on the variant URL
// (the copy plan's fallback rung: its audio is its own plan's, not the copy's).
export interface ServedAudio {
  decision: string;
  codec?: string;
  bitrate?: number;
}

export interface ServedVariant {
  copy: boolean;
  width?: number;
  height?: number;
  bitrate?: number;
  frameRate?: number;
  // Output codec of a transcoded rung.
  codec?: string;
  audio?: ServedAudio;
}

// Reads the servedAudio* parameters the server puts on a fallback rung's URL.
export function servedAudioFromUrl(url: string | null | undefined): ServedAudio | undefined {
  if (!url) return undefined;
  const q = url.indexOf("?");
  if (q < 0) return undefined;
  const params = new URLSearchParams(url.slice(q + 1));
  const decision = params.get("servedAudio");
  if (!decision) return undefined;
  const kbps = Number(params.get("servedAudioKbps"));
  return {
    decision,
    codec: params.get("servedAudioCodec") || undefined,
    bitrate: Number.isFinite(kbps) && kbps > 0 ? kbps * 1000 : undefined,
  };
}

function sourceVideo(info: V1StreamInfo): V1MediaStream | undefined {
  return info.mediaStreams?.find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
}

// servedVariant says whether the playing variant is the copied video or a
// transcoded rung. A copy plan's master carries the copy rung as its richest
// variant; every other rung (the ladder below it, the emergency fallback) is
// an encode. ladderBitrates are every variant's BANDWIDTH; without them (a
// native engine that lists none) the copy rung is the one at the source height.
export function servedVariant(info: V1StreamInfo | null | undefined, playing: PlayingVariant | null | undefined, ladderBitrates: number[] = []): ServedVariant | null {
  if (!info || !playing || !(playing.height && playing.height > 0)) return null;
  if (info.playMethod === "PLAY_METHOD_DIRECT_PLAY") return null;
  const src = sourceVideo(info);
  const bitrate = playing.bitrate && playing.bitrate > 0 ? playing.bitrate : undefined;
  const top = Math.max(0, ...ladderBitrates.filter((b) => b > 0));
  let copy = info.videoDecision === "copy";
  if (copy) {
    copy = bitrate !== undefined && top > 0
      ? bitrate >= top
      : !src?.height || playing.height >= src.height;
  }
  // A copy plan's fallback rung keeps the copy's codec family (vc= pin).
  const codec = copy ? undefined : (info.videoDecision === "copy" ? src?.codec : info.videoCodec) ?? src?.codec;
  return {
    copy,
    width: playing.width && playing.width > 0 ? playing.width : undefined,
    height: playing.height,
    bitrate,
    frameRate: playing.frameRate && playing.frameRate > 0 ? playing.frameRate : undefined,
    codec: codec || undefined,
    audio: servedAudioFromUrl(playing.url),
  };
}

// Engines, structurally: no hls.js or expo-video import here.
export function playingFromHlsLevel(level: { width?: number; height?: number; bitrate?: number; frameRate?: number; uri?: string } | undefined): PlayingVariant | null {
  return level ? { width: level.width, height: level.height, bitrate: level.bitrate, frameRate: level.frameRate, url: level.uri } : null;
}

export function playingFromVideoTrack(track: { size?: { width: number; height: number } | null; peakBitrate?: number | null; averageBitrate?: number | null; bitrate?: number | null; frameRate?: number | null; url?: string | null } | null | undefined): PlayingVariant | null {
  if (!track?.size) return null;
  return { width: track.size.width, height: track.size.height, bitrate: track.peakBitrate ?? track.bitrate ?? track.averageBitrate, frameRate: track.frameRate, url: track.url };
}
