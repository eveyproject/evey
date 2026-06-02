import type { SubtitleStream } from "../SubtitleSelector";
import { getBrowserCapabilities } from "@/utils/codec";

export function getSelectedBurnInTrack(
  currentSubtitle: number | null,
  subtitleStreams: SubtitleStream[],
): number | null {
  if (currentSubtitle === null) return null;
  const sub = subtitleStreams.find((s) => s.trackIndex === currentSubtitle);
  return sub?.burnIn ? currentSubtitle : null;
}

export function appendCodecParams(params: URLSearchParams): void {
  const caps = getBrowserCapabilities();
  if (caps.videoCodecs.length > 0) params.set("videoCodecs", caps.videoCodecs.join(","));
  if (caps.audioCodecs.length > 0) params.set("audioCodecs", caps.audioCodecs.join(","));
  if (caps.containers.length > 0) params.set("containers", caps.containers.join(","));
}

export function buildTranscodeHlsUrl(
  mediaId: string,
  token: string,
  audioTrack: number,
  selectedBurnInTrack: number | null,
  startSec: number,
): string {
  const params = new URLSearchParams();
  if (token) params.set("token", token);
  params.set("audioTrack", String(audioTrack));
  if (selectedBurnInTrack !== null) params.set("subtitleTrack", String(selectedBurnInTrack));
  if (startSec > 0) params.set("startSec", String(startSec));
  appendCodecParams(params);
  // VOD pipeline: server precomputes the manifest from the known media
  // duration so hls.js sees a complete VOD stream from t=0 and never
  // live-edge-snaps. startSec is harmless here — server ignores it and the
  // client seeks via video.currentTime.
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
// For steps 2–3 we iterate the ordered list and return the first stream that
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

  // 3. Profile preferred langs — ordered list, skip codes absent in the file.
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
//   1. Per-series override (localStorage) — null means "disabled"
//   2. Profile preferred subtitle langs (ordered list from profile.preferences)
//   3. Profile locale fallback (from profileLangFallbacks)
//   4. No subtitle (null)
//
// Forced subtitles: when a forced subtitle track exists whose language matches
// the resolved audio language, that track is preferred. This mirrors standard
// streaming behaviour (forced tracks shown for foreign-language segments).
// NOTE: SubtitleStream does not currently carry an isForced flag — it is not
// propagated from V1MediaStream through $mediaId.lazy.tsx. The forced-subtitle
// logic is implemented structurally but will only activate once that field is
// added to SubtitleStream (outside this whitelist). See report for details.
export function resolveDefaultSubtitle(
  subtitleStreams: SubtitleStream[],
  seriesId: string | undefined,
  getSeriesTrackPrefs: (id: string) => { subtitleLang?: string | null } | null,
  getPreferredSubtitleLang: () => string | null,
  profileSubtitleLangs?: string[],
  resolvedAudioLang?: string,
): number | null {
  // 1. Per-series override — null means user explicitly disabled subs.
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
  // SubtitleStream type — currently absent; returns undefined until propagated.
  const forcedInAudioLang = resolvedAudioLang
    ? subtitleStreams.find(
        (s) =>
          (s as SubtitleStream & { isForced?: boolean }).isForced === true &&
          matchLang(s.language, s.title, resolvedAudioLang),
      )
    : undefined;

  // 2. Profile preferred subtitle langs — ordered, skip absent codes.
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
