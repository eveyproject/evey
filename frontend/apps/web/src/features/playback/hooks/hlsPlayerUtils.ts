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
  // The VOD pipeline (Phase 1 refacto, /hls/v2/) computes the manifest from
  // the known media duration so hls.js sees a complete VOD stream from t=0
  // and never live-edge-snaps. startSec is harmless here — the server
  // ignores it and the client seeks via video.currentTime.
  return `/api/v1/media/${mediaId}/hls/v2/master.m3u8?${params.toString()}`;
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

export function resolveDefaultAudio(
  audioStreams: { index: number; trackIndex: number; language?: string; title?: string; isDefault?: boolean }[],
  seriesId: string | undefined,
  getSeriesTrackPrefs: (id: string) => { audioLang?: string } | null,
  getPreferredAudioLang: () => string | null,
) {
  const seriesPrefs = seriesId ? getSeriesTrackPrefs(seriesId) : null;
  const prefLang = seriesPrefs?.audioLang ?? getPreferredAudioLang();
  if (prefLang) {
    const match = audioStreams.find(
      (s) => (s.language ?? s.title ?? "").toLowerCase() === prefLang.toLowerCase(),
    );
    if (match) return match;
  }
  return audioStreams.find((s) => s.isDefault) ?? audioStreams[0];
}

export function resolveDefaultSubtitle(
  subtitleStreams: SubtitleStream[],
  seriesId: string | undefined,
  getSeriesTrackPrefs: (id: string) => { subtitleLang?: string | null } | null,
  getPreferredSubtitleLang: () => string | null,
): number | null {
  const seriesPrefs = seriesId ? getSeriesTrackPrefs(seriesId) : null;
  if (seriesPrefs && "subtitleLang" in seriesPrefs) {
    if (seriesPrefs.subtitleLang === null || seriesPrefs.subtitleLang === undefined) return null;
    const match = subtitleStreams.find(
      (s) => (s.language ?? s.title ?? "").toLowerCase() === seriesPrefs.subtitleLang!.toLowerCase(),
    );
    if (match) return match.trackIndex;
  }
  const prefLang = getPreferredSubtitleLang();
  if (!prefLang) return null;
  const match = subtitleStreams.find(
    (s) => (s.language ?? s.title ?? "").toLowerCase() === prefLang.toLowerCase(),
  );
  return match?.trackIndex ?? null;
}
