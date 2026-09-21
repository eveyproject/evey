import { buildCastStreamUrl, type CastProfile, type ReceiverStreamInfo } from "@evey/media-caps/cast";
import type { SubtitleStream } from "../SubtitleSelector";
// The codec/HLS negotiation helpers are shared with the TV web client:
// packages/client/src/web/hlsCodecParams.ts. Re-exported so the call sites
// and tests in this folder keep importing from where they always did.
import { computeNativeHLSPreference, prefersNativeHLS, getFmp4AudioPassthrough, appendCodecParams } from "@evey/client/web/hlsCodecParams";
export { computeNativeHLSPreference, prefersNativeHLS, getFmp4AudioPassthrough, appendCodecParams };

export function getSelectedBurnInTrack(
  currentSubtitle: number | null,
  subtitleStreams: SubtitleStream[],
): number | null {
  if (currentSubtitle === null) return null;
  const sub = subtitleStreams.find((s) => s.trackIndex === currentSubtitle);
  return sub?.burnIn ? currentSubtitle : null;
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
  forceTranscode = false,
): string {
  const params = new URLSearchParams();
  if (token) params.set("token", token);
  // Explicit handoff off a stalling single-variant master: pin the video
  // decision to transcode (vd=transcode) so the server drops the copy rung and
  // serves the full transcode ABR ladder - the one with lower rungs to step
  // down onto. The pin re-bakes onto every child URL, so the whole session
  // stays on the ladder. See shouldHandoffToTranscode / networkCap.
  if (forceTranscode) params.set("vd", "transcode");
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
  appendCodecParams(params);
  // VOD pipeline: server precomputes the manifest from the known media
  // duration so hls.js sees a complete VOD stream from t=0 and never
  // live-edge-snaps. startSec does not shape the manifest - the client seeks
  // via video.currentTime - but the server takes it as a pre-warm hint, so
  // the variant playlist opens its encoder at that position rather than at 0.
  return `/api/v1/media/${mediaId}/hls/master.m3u8?${params.toString()}`;
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

// CastLoadPlan is what a new cast session loads: the URL the receiver fetches
// on its own (the long-lived cast token) and the absolute position handed to
// the load request.
export interface CastLoadPlan {
  url: string;
  resumeSec: number;
}

// castResumeSec: the receiver's timeline IS the absolute media timeline
// (direct play streams the complete file, transcode the complete precomputed
// VOD manifest), so resuming mid-film rides the load request's currentTime,
// the same mechanism an in-session seek uses. Never baked into the URL: no
// server handler parses a startSec there, and a baked resume silently
// started the receiver at 0 while the UI added the offset back and believed
// the resume had landed.
export function castResumeSec(resumeAtSec: number): number {
  return resumeAtSec > 0 ? Math.floor(resumeAtSec) : 0;
}

// planCastLoad builds the media a Cast session hands the receiver, from the
// SERVER's verdict for THAT receiver (GetStreamInfo asked with the receiver's
// profile - the raw file only when the server said direct play, the HLS
// master carrying the same profile otherwise, where the server decides copy
// vs transcode per track) - never from the browser's own negotiation: the
// browser decodes hevc/mkv/HDR the TV may not, and handing its direct-play
// URL to a receiver was a black screen on the TV. No verdict in hand (the
// call failed) is the master too: the conservative shape.
export function planCastLoad(
  origin: string,
  mediaId: string,
  castToken: string,
  profile: CastProfile,
  verdict: ReceiverStreamInfo | null,
  audioTrack: number,
  resumeAtSec: number,
  gapless: boolean,
): CastLoadPlan {
  return {
    url: buildCastStreamUrl(origin, mediaId, verdict, castToken, profile, { audioTrack, gapless }),
    resumeSec: castResumeSec(resumeAtSec),
  };
}

// planRendererLoad is the DLNA renderer's load: a renderer plays the file the
// BROWSER negotiated (a raw mp4 it can fetch; the transcode master when the
// browser itself needed one) - it is not a Cast receiver and has no profile
// to negotiate with. Unchanged from the pre-negotiation cast plan.
export function planRendererLoad(
  directPlayUrl: string | null | undefined,
  sessionToken: string,
  mediaId: string,
  audioTrack: number,
  resumeAtSec: number,
  gapless: boolean,
): CastLoadPlan {
  const resumeSec = castResumeSec(resumeAtSec);
  if (directPlayUrl) {
    const url = directPlayUrl.includes("token=")
      ? directPlayUrl
      : `${directPlayUrl}${directPlayUrl.includes("?") ? "&" : "?"}token=${encodeURIComponent(sessionToken)}`;
    return { url, resumeSec };
  }
  return { url: buildTranscodeHlsUrl(mediaId, sessionToken, audioTrack, null, 0, 0, gapless), resumeSec };
}

// --- Cast button contract -------------------------------------------------
// The cast button broke silently twice (2026-07-22 and 2026-07-24 in prod).
// These two functions ARE the button's behavior, extracted pure so the tests
// in hlsPlayerUtils.test.ts lock it. Do not reroute the tap or change the
// preload without a red test first and a device validation.

// One tap = one path, decided BEFORE the tap runs anything: chaining a second
// cast path after a failed one runs outside the tap's user activation and the
// browser refuses to open its picker (InvalidAccessError, measured in prod).
// A live session always wins the tap and stops itself.
export type CastTapPath = "stop-sdk" | "stop-fling" | "fling" | "sdk";

export function pickCastTapPath(s: {
  castConnected: boolean;
  remoteConnected: boolean;
  preferFling: boolean;
  castSdkReady: boolean;
}): CastTapPath {
  if (s.castConnected) return "stop-sdk";
  if (s.remoteConnected) return "stop-fling";
  if (s.preferFling) return "fling";
  return s.castSdkReady ? "sdk" : "fling";
}

// When the LAN cast paths find nothing, the user's own Evey clients (TV app,
// another tab) reachable over their persistent WebSocket are the NAT-proof
// fallback - the one that works for users who don't live on the server's LAN.
// Exactly one device: send straight to it, that's what the tap meant. Several:
// don't guess which TV - point at the Play On picker. None: nothing to offer.
export type PlayOnFallback<D> =
  | { action: "send"; device: D }
  | { action: "picker" }
  | { action: "none" };

export function pickPlayOnFallback<D>(devices: readonly D[] | undefined | null): PlayOnFallback<D> {
  if (!devices || devices.length === 0) return { action: "none" };
  if (devices.length === 1) return { action: "send", device: devices[0] };
  return { action: "picker" };
}

// Index of the rung to PIN as soon as the manifest is parsed - -1 = nothing to
// pin (no levels yet).
//
// Pinning `hls.currentLevel` disables hls.js ABR, which is what this path
// wants: every rung is a separate ffmpeg server-side, so one ABR probe is a
// second concurrent 4K decode. It only works if the pin lands BEFORE hls.js
// loads its first fragment: left to a React effect (so one re-render late),
// hls.js starts on free ABR and fetches level 0. Seen in prod on 2026-07-25: a
// 480p and a 1080p spawned 43 ms apart on a host calibrated for ONE encode,
// segment 0 still missing 9.5 s later.
//
// desiredHeight <= 0 means "Auto" -> the TOP rung (the one calibration says is
// sustainable), one decode, no switching. A height that was chosen but is gone
// from the ladder (trimmed by admission since) falls back to the top rather
// than pinning nothing: pinning nothing hands control back to free ABR, which
// is exactly what this avoids.
export function pickPinnedLevel(s: { levelHeights: readonly number[]; desiredHeight: number }): number {
  if (s.levelHeights.length === 0) return -1;
  const top = s.levelHeights.length - 1;
  if (s.desiredHeight <= 0) return top;
  const idx = s.levelHeights.indexOf(s.desiredHeight);
  return idx >= 0 ? idx : top;
}

// Recovers from a mute the BROWSER imposed, not the viewer: lift it on the next
// real gesture, then disarm.
//
// startPlayback runs on every MANIFEST_PARSED, so on every hls.js instance
// rebuild (audio track, subtitles, offset, token refresh). Its last-resort
// branch deliberately leaves the element muted - the right trade on a cold
// start, a silent regression when the viewer was already listening. And since
// the video element survives the rebuild, that mute stuck until they touched
// the volume control themselves.
//
// A real gesture lifts the autoplay policy, so the next click or keystroke is
// exactly when the sound can come back.
//
// Two guards, because this recovery must never fight an explicit intent:
//   - the `m` key toggles the mute itself, so lifting just before would invert
//     the gesture (pressing `m` to get the sound back would cut it again);
//   - the volume button is handled before the click reaches the document, so if
//     the sound is already back we touch nothing.
export function liftForcedMuteOnNextGesture(video: HTMLVideoElement): () => void {
  const disarm = () => {
    document.removeEventListener("click", onGesture);
    document.removeEventListener("keydown", onGesture);
  };
  const onGesture = (e: Event) => {
    if (e.type === "keydown" && (e as KeyboardEvent).key === "m") {
      disarm();
      return;
    }
    disarm();
    if (!video.muted) return;
    // A browser can answer the unmute of an autoplayed media by pausing it
    // (startPlayback documents Firefox doing exactly that): the viewer would
    // hear the sound come back for an instant and stop. We are inside the
    // gesture here, so play() is allowed - put the picture back in motion.
    // Only when it was playing before the lift: a pause that landed just
    // before (the pause button runs ahead of this document listener) is the
    // viewer's own and must stand.
    const wasPlaying = !video.paused;
    video.muted = false;
    if (!wasPlaying) return;
    const replay = () => {
      if (video.paused) video.play().catch(() => { /* the viewer's play button remains */ });
    };
    replay();
    setTimeout(replay, 0);
  };
  document.addEventListener("click", onGesture);
  document.addEventListener("keydown", onGesture);
  return disarm;
}

export type TranscodeAudioSwitchPath = "hls-rendition" | "wasm-engine" | "restart";

// Switching audio track on a transcode is a CLIENT-side selection of a
// rendition the master already exposes (hls.audioTrack): instant, and decoupled
// from the DEFAULT flag - restarting the session only ever moved DEFAULT=YES
// around and cut playback. A restart stays legitimate only where the current
// instance cannot play the target rendition:
//   - native HLS (Safari): no hls.js instance to drive;
//   - wasm audio involved (current OR target): the AC-3 engine is wired per
//     session (video-only master + dedicated rendition);
//   - rendition absent from the parsed manifest.
export function pickTranscodeAudioSwitchPath(s: {
  hasHls: boolean;
  wasmActiveNow: boolean;
  wasmActiveForTarget: boolean;
  audioRenditionCount: number;
  targetTrackIndex: number;
}): TranscodeAudioSwitchPath {
  if (!s.hasHls) return "restart";
  // wasm to wasm: the video is out of band and keeps playing; only the audio
  // engine is rebuilt on the new track (a restart cost seconds of silence).
  if (s.wasmActiveNow && s.wasmActiveForTarget) return "wasm-engine";
  if (s.wasmActiveNow || s.wasmActiveForTarget) return "restart";
  if (s.targetTrackIndex < 0 || s.targetTrackIndex >= s.audioRenditionCount) return "restart";
  return "hls-rendition";
}

// Chromium only ever opens the Remote Playback picker for an element whose
// duration is KNOWN: remote_playback.cc drops every availability URL while
// duration is NaN, and prompt() then rejects instantly with NotAllowedError
// ("The prompt was dismissed") - no picker, 18ms, measured in prod 2026-07-24.
// preload="none" guaranteed duration stayed NaN forever, so the fling was
// structurally dead. The fling element must load metadata where the fling is
// the tap's path (Android); elsewhere it stays lazy - the SDK path doesn't
// read the element, and a weak NAS shouldn't pay a metadata fetch for nothing.
export function castVideoPreload(preferFling: boolean): "metadata" | "none" {
  return preferFling ? "metadata" : "none";
}

// ISO 639-2 codes for the languages MEDIA_LANG_CODES (LanguageSettings.tsx)
// offers as a preference, mapped to their ISO 639-1 equivalent. mkvgo/ffprobe
// tag source files with 639-2 (often the "bibliographic" B form, sometimes the
// "terminological" T form for the ~20 languages that have both), while user
// preferences and locale tags are ISO 639-1 - so the two conventions must be
// reconciled before comparing.
//
// A plain slice(0,2) of the 639-2 code is NOT a safe shortcut: it happens to
// work for some ("fre"/"fra" -> "fr"), but produces the WRONG 639-1 code for
// others ("ger" -> "ge" instead of "de", "por" -> "po" instead of "pt"), and
// for a few it silently COLLIDES with an unrelated language's real code
// ("slo" (Slovak) -> "sl", which is Slovenian's actual 639-1 code). Every
// language below is therefore listed explicitly.
const ISO_639_2_TO_1: Record<string, string> = {
  afr: "af", ara: "ar", bul: "bg", ben: "bn", cat: "ca",
  cze: "cs", ces: "cs", wel: "cy", cym: "cy", dan: "da",
  ger: "de", deu: "de", gre: "el", ell: "el", eng: "en",
  spa: "es", est: "et", baq: "eu", eus: "eu", per: "fa", fas: "fa",
  fin: "fi", fil: "fil", fre: "fr", fra: "fr", gle: "ga", glg: "gl",
  guj: "gu", heb: "he", hin: "hi", hrv: "hr", hun: "hu",
  arm: "hy", hye: "hy", ind: "id", ice: "is", isl: "is", ita: "it",
  jpn: "ja", geo: "ka", kat: "ka", kan: "kn", kor: "ko",
  lit: "lt", lav: "lv", mac: "mk", mkd: "mk", mal: "ml", mar: "mr",
  may: "ms", msa: "ms", mlt: "mt", dut: "nl", nld: "nl", nor: "no",
  pan: "pa", pol: "pl", por: "pt", rum: "ro", ron: "ro", rus: "ru",
  slo: "sk", slk: "sk", slv: "sl", alb: "sq", sqi: "sq", srp: "sr",
  swe: "sv", swa: "sw", tam: "ta", tel: "te", tha: "th", tur: "tr",
  ukr: "uk", urd: "ur", vie: "vi", chi: "zh", zho: "zh",
};

// normalizeLangCode reduces a language tag to a comparable ISO 639-1 code:
// strips a BCP-47 region/script subtag ("fr-FR" -> "fr"), then maps a known
// ISO 639-2 code to 639-1. Codes outside the table above (not offered as a
// preference) fall back to a 2-letter slice - the best guess available.
//
// Exported so the library language filters (LibraryFilters.tsx) can dedupe
// the 639-2/B and 639-2/T forms of the same language ("fre"/"fra") into one
// entry instead of showing both raw codes side by side - this is the same
// table the player already uses to resolve a profile's language preference,
// kept as the single source of truth rather than a second copy.
export function normalizeLangCode(code: string): string {
  const primary = code.toLowerCase().trim().split("-")[0];
  if (!primary) return "";
  return ISO_639_2_TO_1[primary] ?? (primary.length > 2 ? primary.slice(0, 2) : primary);
}

// matchLang returns true when a stream's language or title matches a candidate
// language code. Language comparison is ISO 639-1/639-2/BCP-47 aware (see
// normalizeLangCode); title comparison stays a literal, case-insensitive
// string match since titles are free text ("French", "VFF"), not codes.
function matchLang(
  streamLang: string | undefined,
  streamTitle: string | undefined,
  candidate: string,
): boolean {
  const raw = candidate.toLowerCase().trim();
  if (!raw) return false;
  const title = (streamTitle ?? "").toLowerCase().trim();
  if (title === raw) return true;
  const streamLangNorm = normalizeLangCode(streamLang ?? "");
  if (!streamLangNorm) return false;
  return streamLangNorm === normalizeLangCode(raw);
}

// resolveDefaultAudio picks the best matching audio stream from an ordered
// preference list. Resolution order:
//   1. Per-series override (localStorage)
//   2. Profile preferred langs (ordered list from profile.preferences, locale last)
//   3. Device-local preferred lang (localStorage)
//   4. isDefault flag
//   5. First available stream
//
// For steps 2-3 we iterate the ordered list and return the first stream that
// is actually present in the file, so missing languages are silently skipped.
//
// The device-local pick sits BELOW the profile on purpose: it is written on
// every manual track change, so with it above the profile a one-off VO
// viewing turned into a permanent device override - an account set to French
// then started every next film on the English track (2026-09-03). Per-series
// memory keeps its priority: that one is scoped to what the user picked it on.
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

  // 2. Profile preferred langs - ordered list, skip codes absent in the file.
  if (profileAudioLangs) {
    for (const lang of profileAudioLangs) {
      if (!lang) continue;
      const match = audioStreams.find((s) => matchLang(s.language, s.title, lang));
      if (match) return match;
    }
  }

  // 3. Device-local preferred lang (single value, set when user manually
  //    picks a track during playback). Only reached when the profile has no
  //    preference the file can satisfy.
  const deviceLang = getPreferredAudioLang();
  if (deviceLang) {
    const match = audioStreams.find((s) => matchLang(s.language, s.title, deviceLang));
    if (match) return match;
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

  // 2bis. The track the FILE marks as default. A container names the subtitle
  // it wants played, exactly as it names the default audio track, and whoever
  // mastered the release knew which one belongs with which audio. Missing here,
  // a film whose forced French track is flagged default opened with its
  // foreign-language lines untranslated whenever the audio was not French
  // (prod, 2026-09-05).
  const fileDefault = subtitleStreams.find(
    (s) => (s as SubtitleStream & { isDefault?: boolean }).isDefault === true,
  );
  if (fileDefault) return fileDefault.trackIndex;

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
export async function startPlayback(video: HTMLVideoElement, silentElement = false): Promise<boolean> {
  try {
    await video.play();
    return true; // sounded autoplay allowed - nothing to work around
  } catch {
    // fall through to the muted start
  }

  // An element that carries no audio of its own must NEVER be muted here. On
  // the wasm-audio path the master is video-only and the sound comes from
  // WasmAudioEngine, whose gain MIRRORS video.muted - so this "muted playback
  // beats none" ladder, which can end with muted left true, silences the
  // decoder instead of helping it. And muting cannot buy anything anyway: an
  // audio-less element is not what the autoplay policy gates on.
  if (silentElement) {
    return !video.paused;
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

/** Bytes of forward buffer a browser's MSE quota comfortably holds for one
 *  video SourceBuffer. Firefox threw bufferFullError at ~150 MB (prod 2026-09-10). */
const FORWARD_BUFFER_BYTES = 48_000_000;
export const MIN_FORWARD_BUFFER_SEC = 6;
export const MAX_FORWARD_BUFFER_SEC = 30;

// Seconds of forward buffer that fit the byte budget at this rung's bitrate: a
// 70 Mb/s rung gets 6 s (what a 4K remux needs to play in a browser), a
// 5 Mb/s rung keeps the full 30 s.
export function forwardBufferSeconds(bitrateBps: number): number {
  if (!Number.isFinite(bitrateBps) || bitrateBps <= 0) return MAX_FORWARD_BUFFER_SEC;
  const fit = Math.floor((FORWARD_BUFFER_BYTES * 8) / bitrateBps);
  return Math.max(MIN_FORWARD_BUFFER_SEC, Math.min(MAX_FORWARD_BUFFER_SEC, fit));
}

// pickTranscodeResumeSec chooses where a transcode player rebuild (a new hls.js
// instance, or the native-HLS loadedmetadata seek) resumes.
//
// It must be the LIVE playhead, never the transcodeStartTime state. That state
// is refreshed only by the deliberate switches that call setTranscodeStartTime
// (audio track, burn-in subtitle, audio offset); a plain timeline seek moves the
// live position (positionRef + video.currentTime) but leaves transcodeStartTime
// stale at the last anchor - usually the resume point. A rebuild NOT routed
// through a switch (the wasm-audio fallback flipping wasmAudioActive, a gapless
// toggle) then resumed at that stale anchor and yanked the picture back: the
// "click 12:25, playback restarts at 12:05" report, where the ~20 s gap is
// exactly how far the viewer had seeked past the anchor.
//
// livePosSec (positionRef.current) is initialised to the resume point and kept
// current by timeupdate and by the synchronous write in handleSeekRequest, so it
// is always the right target. startTimeSec (transcodeStartTime) is only the
// fallback for the degenerate livePos<=0 (pre-first-tick cold mount).
export function pickTranscodeResumeSec(livePosSec: number, startTimeSec: number): number {
  if (livePosSec > 0) return livePosSec;
  if (startTimeSec > 0) return startTimeSec;
  return 0;
}
