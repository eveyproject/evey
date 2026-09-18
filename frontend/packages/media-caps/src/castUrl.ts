// Cast negotiation shared by the mobile and web senders: how a RECEIVER is
// described (CastProfile), where that description comes from (the model
// table for a sender that only knows the model name, the receiver's own
// observed capabilities when it reports them), and the request/URL built
// from it. No SDK in here: each sender gets the model name or the caps its
// own SDK gives it, hands them to these functions, and the SERVER decides
// copy / remux / transcode from what they declare - this module keeps no
// such rule.
//
// A cast stream is decoded by the RECEIVER, not by the sender: a stream URL
// negotiated with the sender's own MediaCaps (hevc/hdr on any modern phone,
// mkv on ExoPlayer or a desktop browser) made the server serve a copy the
// Chromecast could not decode - black screen on the TV, perfect playback
// locally. A single "uhd or not" floor (a prior version) was itself too
// conservative: it forced a Chromecast with Google TV or a Google TV Streamer
// down to the Ultra's codec set and, worse, had no notion that H.264 decode
// plateaus below the profile's HEVC/VP9 ceiling on every model except the
// two with an L5.x H.264 decoder. The capability table is keyed on the
// receiver family instead.
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
  // maxAudioChannels: the channel ceiling of the receiver's OUTPUT chain, when
  // it was observed (profileFromReceiverCaps). Absent on the model table: the
  // table knows decoders, not the sink behind them.
  maxAudioChannels?: number;
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
export const FLOOR_PROFILE: CastProfile = PROFILES[PROFILES.length - 1].profile;

// ObservedReceiverCaps is what a receiver reports about ITSELF (our custom
// receiver probes CAF's canDisplayType at start - apps/web cast-receiver,
// probeReceiverCaps): the authority over any model table when it is in hand.
// h264MaxHeight is optional: a receiver built before it was probed reports
// the level only (see profileFromReceiverCaps).
export interface ObservedReceiverCaps {
  videoCodecs: string[];
  maxHeight: number;
  hdr: boolean;
  nativeDolby: boolean;
  maxAudioChannels: number;
  maxH264Level: number;
  h264MaxHeight?: number;
}

const BASE_AUDIO = ["aac", "mp3", "opus", "flac"];

// h264HeightFromLevel is the CONSERVATIVE fallback for a receiver that
// reported no direct h264MaxHeight probe: the largest frame a decoder of
// that level is required to take (level 5.0+ = 4K, 4.x = 1080p, below =
// 720p). A level is a decoder bound, not a measured resolution - the direct
// probe (canDisplayType at 3840x2160) is what a current receiver reports,
// and it wins whenever present.
export function h264HeightFromLevel(levelIdc: number): number {
  if (levelIdc >= 50) return 2160;
  if (levelIdc >= 40) return 1080;
  return 720;
}

// profileFromReceiverCaps turns the receiver's observed capabilities into the
// profile the server is asked with. Observed beats assumed: the codec list,
// the 4K ceiling, HDR and the H.264 ceiling are what the device answered, the
// audio list adds Dolby only when the device itself vouches for passthrough.
export function profileFromReceiverCaps(caps: ObservedReceiverCaps): CastProfile {
  const videoCodecs = caps.videoCodecs.length > 0 ? [...caps.videoCodecs] : ["h264"];
  return {
    videoCodecs,
    audioCodecs: caps.nativeDolby ? [...BASE_AUDIO, "eac3", "ac3"] : [...BASE_AUDIO],
    hdr: caps.hdr,
    maxHeight: caps.maxHeight,
    h264MaxHeight: caps.h264MaxHeight ?? h264HeightFromLevel(caps.maxH264Level),
    maxAudioChannels: caps.maxAudioChannels,
  };
}

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

// castProfileFor picks the profile from what the sender learned about the
// receiver, most trustworthy first: the capabilities the receiver OBSERVED on
// itself (our custom receiver answering the caps channel), else the model
// name its SDK reported (the table), else - unknown receiver, no answer in
// time, a Default Media Receiver, a device found by a subnet probe - the
// conservative floor. Guessing "capable" wrongly is a black screen; guessing
// "floor" wrongly costs a transcode.
export function castProfileFor(observed: ObservedReceiverCaps | null | undefined, modelName: string | null | undefined): CastProfile {
  if (observed) return profileFromReceiverCaps(observed);
  return pickCastProfile(modelName);
}

// ReceiverStreamInfo is the server's verdict for the RECEIVER: GetStreamInfo
// called with the receiver's profile (castStreamInfoParams), not the sender's.
// The server judges the direct play - codecs, resolution (its own H.264
// ceiling included), bit depth, HDR, the default audio track, the container -
// with the same rules every other client gets; this file keeps none of them.
// Structurally V1StreamInfo's playMethod (the API package is not a dependency
// of this one).
export interface ReceiverStreamInfo {
  playMethod?: string;
}

// CastUrlExtras are the sender's request-time choices that ride the master:
// the audio track the viewer picked and the gapless opt-in (web). They are
// inputs the server bakes, never a decision.
export interface CastUrlExtras {
  audioTrack?: number;
  gapless?: boolean;
}

// castStreamInfoParams declares the receiver to GetStreamInfo. Receivers list
// no MKV support, so mp4 is the only container: an mkv source negotiates to
// the HLS copy rung (direct_stream), never to a raw file the TV cannot demux.
export function castStreamInfoParams(profile: CastProfile) {
  return {
    supportedCodecs: profile.videoCodecs,
    supportedAudioCodecs: profile.audioCodecs,
    supportedContainers: ["mp4"],
    supportsHdr: profile.hdr,
    maxHeight: profile.maxHeight,
    maxH264Height: profile.h264MaxHeight,
  };
}

// buildCastStreamUrl is the cast-side sibling of the players' buildStreamUrl:
// the raw file when the SERVER, asked with the receiver's profile, answers
// direct play; otherwise the HLS master carrying that same profile (the
// server then decides copy vs transcode per track). No receiver verdict in
// hand (the stream-info call failed) plays it safe: the master. The token is
// the purpose=cast one the session already minted - the receiver fetches
// segments itself, and the server's cast progress derivation only tracks that
// token kind.
export function buildCastStreamUrl(
  serverUrl: string,
  mediaId: string,
  receiverInfo: ReceiverStreamInfo | null | undefined,
  token: string,
  profile: CastProfile,
  extras: CastUrlExtras = {},
): string {
  if (receiverInfo?.playMethod === "PLAY_METHOD_DIRECT_PLAY") {
    return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/stream?token=${token}`;
  }
  const params = new URLSearchParams({ token });
  params.set("videoCodecs", profile.videoCodecs.join(","));
  params.set("audioCodecs", profile.audioCodecs.join(","));
  params.set("containers", "mp4"); // receivers list no MKV support - never direct-play/remux mkv to a receiver
  params.set("maxHeight", String(profile.maxHeight));
  params.set("maxH264Height", String(profile.h264MaxHeight));
  if (profile.hdr) params.set("hdr", "1");
  // The output chain's channel ceiling, when observed: without it the server
  // applies its 5.1 default and a stereo-only sink refuses the AAC track.
  if (profile.maxAudioChannels) params.set("maxAudioChannels", String(profile.maxAudioChannels));
  if (extras.audioTrack !== undefined) params.set("audioTrack", String(extras.audioTrack));
  if (extras.gapless) params.set("gapless", "1");
  return `${serverUrl}/api/v1/media/${encodeURIComponent(mediaId)}/hls/master.m3u8?${params.toString()}`;
}
