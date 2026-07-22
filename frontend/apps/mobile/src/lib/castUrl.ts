import type { V1StreamInfo } from "@evey/api";

// Twin of the web cast helpers (apps/web .../hooks/hlsPlayerUtils.ts:
// castReceiverCapabilities / castSafeDirectPlay / planCastUrl) - keep the
// SEMANTICS in sync when touching either side. Duplicated rather than shared
// because mobile URLs are absolute against serverUrl and carry no wasm/gapless
// machinery.
//
// A cast stream is decoded by the RECEIVER, not by this phone: the local
// streamUrl is negotiated with the device MediaCaps (hevc/hdr on any modern
// phone, mkv on ExoPlayer), and handing it to loadMedia made the server serve
// a copy rung the Chromecast cannot decode - black screen on the TV, perfect
// playback locally. The default profile is the decode floor every Chromecast
// generation shares; `uhd` comes from receiverSupportsUhd (the connected
// device's model name, the only capability signal the SDK exposes). AC-3/
// E-AC-3 stay out even then: receiver decode is TV-passthrough-dependent,
// and the audio transcode is cheap.
export interface CastReceiverCaps {
  videoCodecs: string[];
  audioCodecs: string[];
  containers: string[];
  hdr: boolean;
}

// receiverSupportsUhd derives the receiver profile from the model name the
// Cast SDK reports once a session is up - the only capability signal any SDK
// exposes for the default receiver (users cannot be asked what generation
// their dongle is). Known HEVC/HDR-capable models: Chromecast Ultra and the
// Google TV family (which downconvert HDR themselves when the TV is SDR).
// Everything else - gen 1-3 "Chromecast", "Eureka Dongle" (gen 1), Android TV
// boxes we have not verified - stays on the floor: a wrong "capable" guess is
// a black screen on the TV, a wrong "floor" guess is only a transcode.
export function receiverSupportsUhd(modelName: string | null | undefined): boolean {
  const m = (modelName ?? "").toLowerCase();
  return m.includes("ultra") || m.includes("google tv");
}

export function castReceiverCapabilities(uhd: boolean): CastReceiverCaps {
  return {
    videoCodecs: uhd ? ["h264", "hevc"] : ["h264"],
    audioCodecs: ["aac", "mp3"],
    containers: ["mp4"],
    hdr: uhd,
  };
}

type CastStreamInfo = Pick<V1StreamInfo, "playMethod" | "container" | "mediaStreams">;

// castSafeDirectPlay reports whether the raw file behind a DIRECT_PLAY session
// can be handed to the receiver as-is: mp4 container, a video stream the
// receiver profile decodes (10-bit only as HEVC Main 10 - 10-bit H.264 has no
// receiver decode), and a default audio track it decodes itself.
export function castSafeDirectPlay(info: CastStreamInfo, caps: CastReceiverCaps): boolean {
  if (info.playMethod !== "PLAY_METHOD_DIRECT_PLAY") return false;
  if ((info.container ?? "").toLowerCase() !== "mp4") return false;
  const streams = info.mediaStreams ?? [];
  const video = streams.find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
  const videoCodec = (video?.codec ?? "").toLowerCase();
  if (!video || !caps.videoCodecs.includes(videoCodec)) return false;
  if ((video.bitDepth ?? 8) > 8 && videoCodec !== "hevc") return false;
  const transfer = (video.colorTransfer ?? "").toLowerCase();
  if (!caps.hdr && (transfer === "smpte2084" || transfer === "arib-std-b67")) return false;
  // The receiver plays the container's DEFAULT audio track; secondary tracks
  // it can't decode are irrelevant.
  const audios = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
  const audio = audios.find((s) => s.isDefault) ?? audios[0];
  if (audio && !caps.audioCodecs.includes((audio.codec ?? "").toLowerCase())) return false;
  return true;
}

// buildCastStreamUrl is the cast-side sibling of usePlaySession's
// buildStreamUrl: the raw file when it is receiver-safe, otherwise the HLS
// master re-negotiated with the receiver caps (the server then decides copy
// vs transcode per track). The token is the long-lived purpose=native one the
// session already minted - the receiver fetches segments itself, exactly like
// the native player this token exists for.
export function buildCastStreamUrl(
  serverUrl: string,
  mediaId: string,
  info: CastStreamInfo,
  token: string,
  uhd: boolean,
): string {
  const caps = castReceiverCapabilities(uhd);
  if (castSafeDirectPlay(info, caps)) {
    return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/stream?token=${token}`;
  }
  const params = new URLSearchParams({ token });
  params.set("videoCodecs", caps.videoCodecs.join(","));
  params.set("audioCodecs", caps.audioCodecs.join(","));
  params.set("containers", caps.containers.join(","));
  if (caps.hdr) params.set("hdr", "1");
  return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/hls/master.m3u8?${params.toString()}`;
}
