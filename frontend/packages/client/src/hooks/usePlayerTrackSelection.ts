import { useEffect, useMemo, useRef, useState } from "react";
import {
  burnInSubtitleIndex as pickBurnInSubtitle,
  clientSubtitleTrack,
  pickDefaultSubtitle,
  pickPreferredStreamIndex,
  type PlayerAudioStream,
  type PlayerSubtitleStream,
} from "@evey/api";

/**
 * The audio and subtitle choice of a native player screen (phone, Android TV)
 * that is REUSED from one title to the next.
 *
 * - A track picked on the title page (`pick`), or by hand in the player, is
 *   never overridden by the profile's languages when they land late.
 * - Otherwise audio follows the profile's languages, then the file's default,
 *   then the first track; subtitles follow the profile's languages, then the
 *   track the FILE flags default ("no preference means no subtitle" left a
 *   film's forced track off everywhere, prod 2026-09-05).
 * - A bitmap subtitle picked on the title page is the server's to burn in:
 *   one restart once the streams are known, never again.
 * - A new title starts from scratch: a hand-picked track of the previous
 *   episode must not follow the viewer (its absolute index usually names
 *   nothing in the next file, which silently turned subtitles off).
 *
 * `burnInSubtitleIndex` is what the SERVER may be told (never the raw
 * selection: a burn-in denies the copy rung), `clientSubtitleIndex` the track
 * the player draws itself, subtitle-relative.
 */
export function usePlayerTrackSelection({
  mediaId,
  pick,
  audioStreams,
  subtitleStreams,
  audioLangs,
  subtitleLangs,
  restartWithTracks,
}: {
  mediaId: string;
  pick: { audio?: number; subtitle?: number | "off" };
  audioStreams: PlayerAudioStream[];
  subtitleStreams: PlayerSubtitleStream[];
  audioLangs: string[];
  subtitleLangs: string[];
  restartWithTracks: (audio?: number, subtitle?: number, quality?: number) => void;
}) {
  const [selectedAudioIndex, setSelectedAudioIndex] = useState<number>(pick.audio ?? 0);
  const [selectedSubtitleIndex, setSelectedSubtitleIndex] = useState<number | null>(
    typeof pick.subtitle === "number" ? pick.subtitle : null,
  );
  const userPickedAudioRef = useRef(pick.audio !== undefined);
  const userPickedSubtitleRef = useRef(pick.subtitle !== undefined);
  const pickedBurnInAppliedRef = useRef(false);

  // A new title: forget the previous one's choices. Only on a CHANGE - on
  // mount the title page's pick is exactly what must be kept.
  const titleRef = useRef(mediaId);
  useEffect(() => {
    if (titleRef.current === mediaId) return;
    titleRef.current = mediaId;
    userPickedAudioRef.current = false;
    userPickedSubtitleRef.current = false;
    pickedBurnInAppliedRef.current = false;
    setSelectedAudioIndex(0);
    setSelectedSubtitleIndex(null);
  }, [mediaId]);

  useEffect(() => {
    if (userPickedAudioRef.current || audioStreams.length === 0) return;
    const preferred = pickPreferredStreamIndex(audioStreams, audioLangs);
    setSelectedAudioIndex(preferred ?? audioStreams.find((s) => s.isDefault)?.index ?? audioStreams[0].index);
  }, [audioStreams, audioLangs]);

  useEffect(() => {
    if (userPickedSubtitleRef.current) {
      if (pickedBurnInAppliedRef.current || typeof pick.subtitle !== "number") return;
      const picked = subtitleStreams.find((s) => s.index === pick.subtitle);
      if (!picked) return;
      pickedBurnInAppliedRef.current = true;
      if (picked.burnIn) restartWithTracks(selectedAudioIndex, picked.index, undefined);
      return;
    }
    const preferred = pickDefaultSubtitle(subtitleStreams, subtitleLangs);
    if (preferred !== null) setSelectedSubtitleIndex(preferred);
  }, [subtitleStreams, subtitleLangs, pick.subtitle, restartWithTracks, selectedAudioIndex]);

  const burnInSubtitleIndex = useMemo(
    () => pickBurnInSubtitle(selectedSubtitleIndex, subtitleStreams),
    [selectedSubtitleIndex, subtitleStreams],
  );
  const clientSubtitleIndex = useMemo(
    () => clientSubtitleTrack(selectedSubtitleIndex, subtitleStreams),
    [selectedSubtitleIndex, subtitleStreams],
  );

  return {
    selectedAudioIndex,
    setSelectedAudioIndex,
    selectedSubtitleIndex,
    setSelectedSubtitleIndex,
    userPickedAudioRef,
    userPickedSubtitleRef,
    burnInSubtitleIndex,
    clientSubtitleIndex,
  };
}
