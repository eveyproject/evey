import type { SubtitleStream } from "../SubtitleSelector";

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
): string {
  const params = new URLSearchParams();
  if (token) params.set("token", token);
  params.set("audioTrack", String(audioTrack));
  if (selectedBurnInTrack !== null) params.set("subtitleTrack", String(selectedBurnInTrack));
  if (startSec > 0) params.set("startSec", String(startSec));
  return `/api/v1/media/${mediaId}/hls/master.m3u8?${params.toString()}`;
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
