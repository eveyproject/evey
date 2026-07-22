import Hls from "hls.js";
import type { SubtitleStream } from "../SubtitleSelector";
import { getBrowserCapabilities, type BrowserCapabilities } from "@/utils/codec";
import type { V1StreamInfo } from "@evey/api";

// computeNativeHLSPreference is the pure decision behind prefersNativeHLS,
// factored out so every browser can be exercised in a unit test without a real
// engine. Native HLS (the <video> element's own player) is preferred on Safari
// (macOS laptops + iOS) - lighter on CPU/battery than hls.js+MSE - and is the
// only option where MSE is missing (old iOS). Chrome/Firefox/Edge/Android stay
// on hls.js even when the OS can decode HLS, because that path is the mature one
// and carries our token refresh / CMCD / ABR control.
export function computeNativeHLSPreference(
  canPlayHLS: boolean,
  userAgent: string,
  platform: string,
  maxTouchPoints: number,
  mseSupported: boolean,
): boolean {
  if (!canPlayHLS) return false;
  // Safari, and only Safari, among the engines on Apple hardware: Chrome/Edge/
  // Firefox on macOS carry chrome/crios/fxios/edg tokens and must be excluded.
  const safari = /^((?!chrome|chromium|android|crios|fxios|edg).)*safari/i.test(userAgent);
  const ios =
    /iP(hone|ad|od)/.test(userAgent) ||
    // iPadOS 13+ masquerades as desktop Safari (MacIntel) but is touch-capable.
    (platform === "MacIntel" && maxTouchPoints > 1);
  return safari || ios || !mseSupported;
}

// prefersNativeHLS reports whether this browser should play HLS through the
// <video> element's native engine instead of hls.js/MSE. Memoised: it reads a
// static capability (canPlayType) + UA.
let _prefersNativeHLS: boolean | undefined;
export function prefersNativeHLS(): boolean {
  if (_prefersNativeHLS !== undefined) return _prefersNativeHLS;
  if (typeof document === "undefined" || typeof navigator === "undefined") return false;
  const canPlayHLS =
    document.createElement("video").canPlayType("application/vnd.apple.mpegurl") !== "";
  _prefersNativeHLS = computeNativeHLSPreference(
    canPlayHLS,
    navigator.userAgent,
    navigator.platform,
    navigator.maxTouchPoints,
    Hls.isSupported(),
  );
  return _prefersNativeHLS;
}

export function getSelectedBurnInTrack(
  currentSubtitle: number | null,
  subtitleStreams: SubtitleStream[],
): number | null {
  if (currentSubtitle === null) return null;
  const sub = subtitleStreams.find((s) => s.trackIndex === currentSubtitle);
  return sub?.burnIn ? currentSubtitle : null;
}

export function appendCodecParams(
  params: URLSearchParams,
  caps: BrowserCapabilities = getBrowserCapabilities(),
): void {
  if (caps.videoCodecs.length > 0) params.set("videoCodecs", caps.videoCodecs.join(","));
  if (caps.audioCodecs.length > 0) params.set("audioCodecs", caps.audioCodecs.join(","));
  if (caps.containers.length > 0) params.set("containers", caps.containers.join(","));
  // HDR display capability: without it the server decision is HDR-blind and
  // forces a tonemap transcode on every HDR title - even for clients that
  // decode HEVC+HDR10 natively (the copy rung). Only sent when true: the
  // server default is SDR (conservative, mirrors the gRPC supportsHdr param).
  if (caps.hdr) params.set("hdr", "1");
}

export function buildTranscodeHlsUrl(
  mediaId: string,
  token: string,
  audioTrack: number,
  selectedBurnInTrack: number | null,
  startSec: number,
  audioOffsetMs = 0,
  gapless = false,
  wasmAudio = false,
  caps?: BrowserCapabilities,
): string {
  const params = new URLSearchParams();
  if (token) params.set("token", token);
  params.set("audioTrack", String(audioTrack));
  if (selectedBurnInTrack !== null) params.set("subtitleTrack", String(selectedBurnInTrack));
  if (startSec > 0) params.set("startSec", String(startSec));
  // wasm-audio path: ask the server for a VIDEO-ONLY master whose AC-3/E-AC-3
  // audio the client decodes itself (ac3go wasm). Named in the master's
  // #EVEY-AUDIO-URI tag, fetched out of band by WasmAudioEngine - never muxed
  // into the stream hls.js appends, which MSE couldn't decode.
  if (wasmAudio) params.set("wasmAudio", "1");
  // Audio resync: a decision input (forces the audio to re-encode with a
  // shifted input) - the server re-bakes it on every child URL like the
  // codec caps, so the whole session stays coherent.
  if (audioOffsetMs !== 0) params.set("audioOffsetMs", String(Math.trunc(audioOffsetMs)));
  // Gapless multi-episode session ("lecture enchaînée"): opt-in per request,
  // server-side fallback to today's per-episode master when the feature is
  // off / the next episode isn't eligible - never a hard failure client-side.
  if (gapless) params.set("gapless", "1");
  appendCodecParams(params, caps);
  // VOD pipeline: server precomputes the manifest from the known media
  // duration so hls.js sees a complete VOD stream from t=0 and never
  // live-edge-snaps. startSec is harmless here - server ignores it and the
  // client seeks via video.currentTime.
  return `/api/v1/media/${mediaId}/hls/master.m3u8?${params.toString()}`;
}

// castReceiverCapabilities is the capability profile advertised on every URL
// handed to a cast device. A cast stream is decoded by the RECEIVER, not by the
// browser that builds the URL, and neither the Cast SDK nor the Remote Playback
// fling tells us which Chromecast generation took the stream. The default is
// therefore the decode floor every generation shares: H.264 8-bit SDR in fMP4,
// AAC/MP3 audio. HEVC/HDR are Ultra/Google-TV-only - advertising them because
// the SENDER decodes them turns the server's copy rung into a black screen on
// a gen 1-3 receiver.
//
// `uhd` upgrades the profile to HEVC (incl. Main 10) + HDR so 4K HDR titles
// keep the copy rung instead of paying a tonemap transcode. Only callers that
// KNOW the receiver may set it: the mobile app derives it from the device
// model its native Cast SDK reports (receiverSupportsUhd in the mobile twin);
// the web sender has no such signal and always stays on the floor - the user
// is never asked what generation their dongle is. AC-3/E-AC-3 stay out even
// in UHD: receiver decode is TV-passthrough-dependent, and audio transcode
// is cheap.
export function castReceiverCapabilities(uhd: boolean): BrowserCapabilities {
  return {
    videoCodecs: uhd ? ["h264", "hevc"] : ["h264"],
    audioCodecs: ["aac", "mp3"],
    containers: ["mp4"],
    hdr: uhd,
  };
}

type CastStreamInfo = Pick<V1StreamInfo, "playMethod" | "container" | "mediaStreams" | "directPlayUrl">;

// castSafeDirectPlay reports whether the raw file behind a DIRECT_PLAY session
// can be flung to a cast receiver as-is: mp4 container, a video stream the
// receiver profile decodes (8-bit SDR H.264 on the floor; +HEVC/10-bit/HDR
// when the user declared a UHD receiver), and a default audio track it decodes
// itself (AAC/MP3). Everything else must go through the server decision with
// the receiver caps instead, where it becomes a copy rung or a transcode.
export function castSafeDirectPlay(
  info: CastStreamInfo,
  caps: BrowserCapabilities = castReceiverCapabilities(false),
): boolean {
  if (info.playMethod !== "PLAY_METHOD_DIRECT_PLAY") return false;
  if ((info.container ?? "").toLowerCase() !== "mp4") return false;
  const streams = info.mediaStreams ?? [];
  const video = streams.find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
  const videoCodec = (video?.codec ?? "").toLowerCase();
  if (!video || !caps.videoCodecs.includes(videoCodec)) return false;
  // 10-bit is an HEVC Main 10 story; 10-bit H.264 has no receiver decode.
  if ((video.bitDepth ?? 8) > 8 && videoCodec !== "hevc") return false;
  const transfer = (video.colorTransfer ?? "").toLowerCase();
  if (!caps.hdr && (transfer === "smpte2084" || transfer === "arib-std-b67")) return false;
  // The receiver plays the container's DEFAULT audio track; secondary tracks
  // it can't decode are irrelevant to the fling.
  const audios = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
  const audio = audios.find((s) => s.isDefault) ?? audios[0];
  if (audio && !caps.audioCodecs.includes((audio.codec ?? "").toLowerCase())) return false;
  return true;
}

// planCastUrl picks the URL a cast receiver is handed: the raw file when it is
// receiver-safe, otherwise the HLS master re-negotiated with the receiver caps.
// Deliberately NOT streamInfo.directPlayUrl in the general case: usePlaySession
// poses that field for every play method, and for a transcode session it holds
// the LOCAL player's master URL - negotiated with the browser caps, so a
// hevc/hdr-capable sender would make the server copy a stream the receiver
// cannot decode. wasmAudio stays off: the receiver can't run the wasm decoder
// (and its absence is what tells a receiver fetch from a browser fetch in the
// server logs). Token handling (session-token append, cast-token swap) stays
// with the caller.
export function planCastUrl(
  info: CastStreamInfo,
  mediaId: string,
  token: string,
  audioTrack: number,
  gapless: boolean,
  caps: BrowserCapabilities = castReceiverCapabilities(false),
): { url: string; direct: boolean } {
  if (info.directPlayUrl && castSafeDirectPlay(info, caps)) {
    return { url: info.directPlayUrl, direct: true };
  }
  return {
    url: buildTranscodeHlsUrl(mediaId, token, audioTrack, null, 0, 0, gapless, false, caps),
    direct: false,
  };
}

// shouldRestartTranscodeSeek decides whether a seek in transcode mode can be
// served by moving the <video> playhead in place, or needs the transcode to
// restart at the new offset. The server transcodes sequentially from the
// current offset and only exposes a sliding window of segments (live-style),
// so hls.js can seek in place ONLY within the range it has actually buffered.
// A target before the buffered window's start (the window scrolled past it, as
// after a long playback then a backward scrub) or beyond its end is no longer
// in the playlist and must restart. An empty buffer also forces a restart.
//
// The +10s forward tolerance lets a small seek just past the live edge ride the
// segments about to arrive instead of paying a full restart.
export function shouldRestartTranscodeSeek(
  absoluteTimeSec: number,
  offsetSec: number,
  buffered: TimeRanges,
): boolean {
  if (buffered.length === 0) return true;
  const localTime = absoluteTimeSec - offsetSec;
  const start = buffered.start(0);
  const end = buffered.end(buffered.length - 1);
  return localTime < start || localTime > end + 10;
}

// matchLang returns true when a stream's language or title matches a candidate
// language code (case-insensitive, trimmed).
function matchLang(
  streamLang: string | undefined,
  streamTitle: string | undefined,
  candidate: string,
): boolean {
  const norm = candidate.toLowerCase().trim();
  if (!norm) return false;
  const lang = (streamLang ?? "").toLowerCase().trim();
  const title = (streamTitle ?? "").toLowerCase().trim();
  return lang === norm || title === norm;
}

// resolveDefaultAudio picks the best matching audio stream from an ordered
// preference list. Resolution order:
//   1. Per-series override (localStorage)
//   2. Device-local preferred lang (localStorage)
//   3. Profile preferred langs (ordered list from profile.preferences)
//   4. isDefault flag
//   5. First available stream
//
// For steps 2-3 we iterate the ordered list and return the first stream that
// is actually present in the file, so missing languages are silently skipped.
export function resolveDefaultAudio(
  audioStreams: { index: number; trackIndex: number; language?: string; title?: string; isDefault?: boolean }[],
  seriesId: string | undefined,
  getSeriesTrackPrefs: (id: string) => { audioLang?: string } | null,
  getPreferredAudioLang: () => string | null,
  profileAudioLangs?: string[],
) {
  // 1. Per-series override wins unconditionally.
  const seriesPrefs = seriesId ? getSeriesTrackPrefs(seriesId) : null;
  if (seriesPrefs?.audioLang) {
    const match = audioStreams.find((s) => matchLang(s.language, s.title, seriesPrefs.audioLang!));
    if (match) return match;
  }

  // 2. Device-local preferred lang (single value, set when user manually
  //    picks a track during playback).
  const deviceLang = getPreferredAudioLang();
  if (deviceLang) {
    const match = audioStreams.find((s) => matchLang(s.language, s.title, deviceLang));
    if (match) return match;
  }

  // 3. Profile preferred langs - ordered list, skip codes absent in the file.
  if (profileAudioLangs) {
    for (const lang of profileAudioLangs) {
      if (!lang) continue;
      const match = audioStreams.find((s) => matchLang(s.language, s.title, lang));
      if (match) return match;
    }
  }

  // 4. isDefault flag, then first stream.
  return audioStreams.find((s) => s.isDefault) ?? audioStreams[0];
}

// resolveDefaultSubtitle picks the best matching subtitle track. Resolution order:
//   1. Per-series override (localStorage) - null means "disabled"
//   2. Profile preferred subtitle langs (ordered list from profile.preferences)
//   3. Profile locale fallback (from profileLangFallbacks)
//   4. No subtitle (null)
//
// Forced subtitles: when a forced subtitle track exists whose language matches
// the resolved audio language, that track is preferred. This mirrors standard
// streaming behaviour (forced tracks shown for foreign-language segments).
// SubtitleStream carries isForced, propagated from V1MediaStream in
// $mediaId.lazy.tsx, so this path is active.
export function resolveDefaultSubtitle(
  subtitleStreams: SubtitleStream[],
  seriesId: string | undefined,
  getSeriesTrackPrefs: (id: string) => { subtitleLang?: string | null } | null,
  getPreferredSubtitleLang: () => string | null,
  profileSubtitleLangs?: string[],
  resolvedAudioLang?: string,
): number | null {
  // 1. Per-series override - null means user explicitly disabled subs.
  const seriesPrefs = seriesId ? getSeriesTrackPrefs(seriesId) : null;
  if (seriesPrefs && "subtitleLang" in seriesPrefs) {
    if (seriesPrefs.subtitleLang === null || seriesPrefs.subtitleLang === undefined) return null;
    const match = subtitleStreams.find(
      (s) => matchLang(s.language, s.title, seriesPrefs.subtitleLang!),
    );
    if (match) return match.trackIndex;
  }

  // Forced subtitle helper: if a forced track exists in the audio language,
  // prefer it (hidden dialogue / foreign segments). Requires isForced on the
  // SubtitleStream type - currently absent; returns undefined until propagated.
  const forcedInAudioLang = resolvedAudioLang
    ? subtitleStreams.find(
        (s) =>
          (s as SubtitleStream & { isForced?: boolean }).isForced === true &&
          matchLang(s.language, s.title, resolvedAudioLang),
      )
    : undefined;

  // 2. Profile preferred subtitle langs - ordered, skip absent codes.
  if (profileSubtitleLangs) {
    for (const lang of profileSubtitleLangs) {
      if (!lang) continue;
      // Prefer a forced track in this language first.
      const forced = subtitleStreams.find(
        (s) =>
          (s as SubtitleStream & { isForced?: boolean }).isForced === true &&
          matchLang(s.language, s.title, lang),
      );
      if (forced) return forced.trackIndex;
      const match = subtitleStreams.find((s) => matchLang(s.language, s.title, lang));
      if (match) return match.trackIndex;
    }
  }

  // Forced track in the audio language wins over "no preference".
  if (forcedInAudioLang) return forcedInAudioLang.trackIndex;

  // 3. Legacy device-local pref (single value).
  const prefLang = getPreferredSubtitleLang();
  if (!prefLang) return null;
  const match = subtitleStreams.find((s) => matchLang(s.language, s.title, prefLang));
  return match?.trackIndex ?? null;
}

// startPlayback begins playback and gets the sound back on.
//
// Browsers only block SOUNDED autoplay: play() is refused until the viewer has
// interacted with the page, but a MUTED play() is always allowed. So when the
// sounded start is refused, start muted - then lift the mute straight away, which
// is the only part the policy does not gate. The viewer gets, at worst, a few
// silent frames.
//
// The player used to stop at the muted start and never lift it, which is how a
// title opened in a fresh browser played as a silent film with no clue why.
//
// A browser may still object to the unmute (Firefox can re-pause a media it
// considers autoplaying with sound). If it does, keep it muted and PLAYING rather
// than dropping the viewer into a dead player: the volume control shows the muted
// state, and one click brings the sound back.
export async function startPlayback(video: HTMLVideoElement): Promise<boolean> {
  try {
    await video.play();
    return true; // sounded autoplay allowed - nothing to work around
  } catch {
    // fall through to the muted start
  }

  video.muted = true;
  try {
    await video.play();
  } catch {
    video.muted = false; // hard block: needs a real gesture, don't leave it muted
    return false;
  }

  video.muted = false;
  if (video.paused) {
    // The browser answered the unmute by pausing. Muted playback beats none.
    video.muted = true;
    await video.play().catch(() => { /* nothing left to try */ });
  }
  return !video.paused;
}
