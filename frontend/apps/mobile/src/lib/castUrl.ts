import type { V1StreamInfo, V1MediaStream } from "@evey/api";

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
// playback locally. A single "uhd or not" floor (a prior version of this
// file) was itself too conservative: it forced a Chromecast with Google TV
// or a Google TV Streamer down to the Ultra's codec set and, worse, had no
// notion that H.264 decode plateaus below the profile's HEVC/VP9 ceiling on
// every model except the two with an L5.x H.264 decoder. This version keyed
// the capability table on the actual receiver family instead.
export interface CastProfile {
  videoCodecs: string[];
  audioCodecs: string[];
  hdr: boolean;
  maxHeight: number;
  // h264MaxHeight: the H.264 decode ceiling, independent of maxHeight (which
  // bounds HEVC/VP9/AV1 when the profile has them). Only Chromecast with
  // Google TV (H.264 High L5.1) and Google TV Streamer (L5.2) decode 4K in
  // H.264; every other family - Chromecast Ultra included, whose H.264
  // decoder is L4.2 same as Chromecast 3 - plateaus at 1080p60 in H.264 even
  // though it decodes 4K fine via HEVC/VP9.
  h264MaxHeight: number;
}

// Source: developers.google.com/cast/docs/media (default media receiver
// codec/level support). Model-name substrings below are best-effort: the
// Cast SDK's CastDevice.modelName strings for the newer Google TV family
// (Chromecast HD, Google TV Streamer, launched 2024) were not verified
// against a physical unit - only the pattern documented by Google is
// encoded here. Unverified assumption, flagged for whoever validates on
// real hardware.
const PROFILES: Array<{ match: (m: string) => boolean; profile: CastProfile }> = [
  {
    // Google TV Streamer: H.264 High L5.2 (4K60), HEVC Main/Main10 L5.1
    // (4K60), VP9 profile 2 (4K60), AV1 Main L5.1. HDR10/HDR10+/DV.
    match: (m) => m.includes("google tv streamer"),
    profile: { videoCodecs: ["h264", "hevc", "vp9", "av1"], audioCodecs: ["aac", "mp3", "opus", "flac", "eac3", "ac3"], hdr: true, maxHeight: 2160, h264MaxHeight: 2160 },
  },
  {
    // Chromecast HD (with Google TV, HD tier): same modern decoders as the
    // rest of the Google TV family but a 1080p SDR-safe output stage - no
    // point negotiating HDR or > 1080p the panel/output never uses.
    match: (m) => m.includes("chromecast hd"),
    profile: { videoCodecs: ["h264", "hevc", "vp9", "av1"], audioCodecs: ["aac", "mp3", "opus", "flac", "eac3", "ac3"], hdr: false, maxHeight: 1080, h264MaxHeight: 1080 },
  },
  {
    // Chromecast with Google TV (4K): H.264 L5.1 (4K30), HEVC Main/Main10
    // L5.1 (4K60), VP9 profile 2 (4K60). HDR10/HDR10+/DV.
    match: (m) => m.includes("google tv"),
    profile: { videoCodecs: ["h264", "hevc", "vp9"], audioCodecs: ["aac", "mp3", "opus", "flac", "eac3", "ac3"], hdr: true, maxHeight: 2160, h264MaxHeight: 2160 },
  },
  {
    // Chromecast Ultra: H.264 L4.2 (plateaus at 1080p60, NOT 4K), HEVC
    // Main/Main10 L5.1 (4K60), VP9 profiles 0/2 L5.1 (4K60). HDR10 + Dolby
    // Vision. No AC-3/E-AC-3: the Ultra has no Dolby license of its own and
    // passthrough depends entirely on the TV behind it - never guaranteed,
    // and transcoding audio is cheap enough not to gamble on it.
    match: (m) => m.includes("ultra"),
    profile: { videoCodecs: ["h264", "hevc", "vp9"], audioCodecs: ["aac", "mp3", "opus", "flac"], hdr: true, maxHeight: 2160, h264MaxHeight: 1080 },
  },
  {
    // Nest Hub (H.264 L4.1 720p60, VP9 L4.0 720p60) and Hub Max (same
    // decoders at 30fps) both output to a 720p screen - no HDR panel either.
    match: (m) => m.includes("nest hub") || m.includes("hub max"),
    profile: { videoCodecs: ["h264", "vp9"], audioCodecs: ["aac", "mp3", "opus", "flac"], hdr: false, maxHeight: 720, h264MaxHeight: 720 },
  },
  {
    // MiTV-AFMU0 (built-in cast, Android TV, Amlogic): first model
    // string ever observed through the cast_receiver_model beacon in prod
    // (2026-07-25). HEVC 4K and HDR10 panel; h264 held at 1080p - Amlogic
    // h264 decode above L4.2 is not a given.
    // NO Dolby here: the "Dolby Audio" certification covers the TV's Android
    // apps, not its cast runtime. Declaring eac3/ac3 got E-AC-3 served as
    // passthrough to the Default Media Receiver and 3 out of 3 casts ended
    // up stuck on the backdrop (prod 2026-07-25), when the SAME files played
    // fine the day before via the floor profile's AAC transcode. Same rule
    // as the Ultra: never bet on a receiver's Dolby passthrough.
    // Jellyfin reached the same conclusion: their custom Chromecast receiver
    // hardcodes hasEAC3Support()/hasAC3Support() to FALSE ("problems
    // getting it to work with HLS") even though it COULD probe
    // canDisplayType - HLS cast audio there is aac/mp3/opus, period.
    // (jellyfin-chromecast src/components/codecSupportHelper.ts)
    match: (m) => m.includes("mitv"),
    profile: { videoCodecs: ["h264", "hevc", "vp9"], audioCodecs: ["aac", "mp3", "opus", "flac"], hdr: true, maxHeight: 2160, h264MaxHeight: 1080 },
  },
  {
    // Chromecast gen 1/2 ("Eureka Dongle") and gen 3: H.264 High L4.1/L4.2,
    // VP8. SDR only, no Dolby decoder at all.
    match: (m) => m.includes("chromecast") || m.includes("eureka"),
    profile: { videoCodecs: ["h264"], audioCodecs: ["aac", "mp3", "opus", "flac"], hdr: false, maxHeight: 1080, h264MaxHeight: 1080 },
  },
];

// FLOOR_PROFILE covers unknown receivers - a TV's built-in cast receiver, a
// third-party Cast-certified box, or any model string this table hasn't
// seen. Guessing "capable" wrongly is a black screen on the TV; guessing
// "floor" wrongly just costs a transcode. Always guess floor.
const FLOOR_PROFILE: CastProfile = PROFILES[PROFILES.length - 1].profile;

// pickCastProfile derives the receiver capability profile from the model
// name the Cast SDK reports once a session is up - the only capability
// signal any SDK exposes for the connected receiver (users cannot be asked
// what generation their dongle is). Matching is case-insensitive and tries
// the most specific pattern first (e.g. "chromecast hd" before the bare
// "chromecast" gen 1-3 fallback).
export function pickCastProfile(modelName: string | null | undefined): CastProfile {
  const m = (modelName ?? "").toLowerCase();
  for (const { match, profile } of PROFILES) {
    if (match(m)) return profile;
  }
  return FLOOR_PROFILE;
}

type CastStreamInfo = Pick<V1StreamInfo, "playMethod" | "container" | "mediaStreams">;

// castSafeDirectPlay reports whether the raw file behind a DIRECT_PLAY session
// can be handed to the receiver as-is: mp4 container, a video stream the
// profile decodes at its actual resolution (10-bit only as HEVC Main 10 -
// 10-bit H.264 has no receiver decode), HDR only where the profile supports
// it, and a default audio track the profile decodes.
export function castSafeDirectPlay(info: CastStreamInfo, profile: CastProfile): boolean {
  if (info.playMethod !== "PLAY_METHOD_DIRECT_PLAY") return false;
  if ((info.container ?? "").toLowerCase() !== "mp4") return false;
  const streams = info.mediaStreams ?? [];
  const video = streams.find((s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
  if (!video) return false;
  const videoCodec = (video.codec ?? "").toLowerCase();
  if (!profile.videoCodecs.includes(videoCodec)) return false;
  const height = video.height ?? 0;
  // H.264 has its own, generally lower, decode ceiling (see h264MaxHeight's
  // doc comment) - a 4K H.264 source clears the codec check on the Ultra but
  // still can't decode there.
  const heightCeiling = videoCodec === "h264" ? profile.h264MaxHeight : profile.maxHeight;
  if (height > heightCeiling) return false;
  if ((video.bitDepth ?? 8) > 8 && videoCodec !== "hevc") return false;
  const transfer = (video.colorTransfer ?? "").toLowerCase();
  if (!profile.hdr && (transfer === "smpte2084" || transfer === "arib-std-b67")) return false;
  // The receiver plays the container's DEFAULT audio track; secondary tracks
  // it can't decode are irrelevant.
  const audios = streams.filter((s: V1MediaStream) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
  const audio = audios.find((s: V1MediaStream) => s.isDefault) ?? audios[0];
  if (audio && !profile.audioCodecs.includes((audio.codec ?? "").toLowerCase())) return false;
  return true;
}

// buildCastStreamUrl is the cast-side sibling of usePlaySession's
// buildStreamUrl: the raw file when it is receiver-safe, otherwise the HLS
// master re-negotiated with the receiver's profile (the server then decides
// copy vs transcode per track). The token is the purpose=cast one the
// session already minted - the receiver fetches segments itself, and the
// server's cast progress derivation only tracks that token kind.
export function buildCastStreamUrl(
  serverUrl: string,
  mediaId: string,
  info: CastStreamInfo,
  token: string,
  profile: CastProfile,
): string {
  if (castSafeDirectPlay(info, profile)) {
    return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/stream?token=${token}`;
  }
  const params = new URLSearchParams({ token });
  params.set("videoCodecs", profile.videoCodecs.join(","));
  params.set("audioCodecs", profile.audioCodecs.join(","));
  params.set("containers", "mp4"); // receivers list no MKV support - never direct-play/remux mkv to a receiver
  params.set("maxHeight", String(profile.maxHeight));
  if (profile.hdr) params.set("hdr", "1");
  return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/hls/master.m3u8?${params.toString()}`;
}
