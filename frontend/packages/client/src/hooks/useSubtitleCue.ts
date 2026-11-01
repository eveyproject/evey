import { useMemo } from "react";
import {
  findActiveCueIndex,
  findActivePGSCue,
  shiftCues,
  type PGSCue,
  type SubtitleCue,
} from "@evey/api";
import { useSubtitleTrack } from "./useSubtitleTrack";
import { usePlayheadSelector } from "../lib/playheadStore";

// useSubtitleTrack loads 60s ahead and moves its window in 10s steps, so the
// time it is fed only has to be right to the window, not the frame. Quantising
// it keeps the fetch from re-evaluating on every playhead tick.
const FETCH_QUANTUM_SEC = 10;

/** The subtitle to draw right now, platform-agnostic: each player renders it. */
export interface ActiveSubtitle {
  /** The track is still being produced (202): a player says so in its place. */
  preparing: boolean;
  /** The active text cue's text, null when none is active. */
  text: string | null;
  /** The active bitmap (PGS) cue: its image URI, geometry and the plane it is
   *  measured in (NOT the video resolution), null when none is active. */
  pgs: { uri: string; cue: PGSCue; screenW: number; screenH: number } | null;
}

export interface SubtitleCueOptions {
  /** The caller drives the clock (phone, tv-web); omit to follow the playhead store (TV). */
  currentTimeSec?: number;
  /** A manual A/V offset the viewer set, in milliseconds. */
  offsetMs?: number;
  /** Cues from a source other than the server track (an embedded sidecar). */
  externalCues?: SubtitleCue[] | null;
}

/**
 * The one place the three player subtitle overlays (TV, phone, tv-web) share:
 * fetch the track, then pick the cue at the playhead - WITHOUT re-rendering the
 * overlay twice a second. The active cue holds for seconds, so each index is
 * chosen through usePlayheadSelector, which only re-renders its consumer when
 * the chosen index actually changes. With no track selected the indices stay
 * -1 and the overlay never re-renders at all.
 */
export function useSubtitleCue(
  mediaId: string,
  subtitleIndex: number | null,
  opts: SubtitleCueOptions = {},
): ActiveSubtitle {
  const { currentTimeSec: given, offsetMs = 0, externalCues = null } = opts;

  // Coarse fetch clock: quantised to the track's own window step.
  const fetchSec = usePlayheadSelector((sec) => {
    const src = Math.max(0, (given ?? sec) - offsetMs / 1000);
    return Math.floor(src / FETCH_QUANTUM_SEC) * FETCH_QUANTUM_SEC;
  });
  const { cues: fetched, pgs, preparing } = useSubtitleTrack(
    mediaId,
    externalCues ? null : subtitleIndex,
    fetchSec,
  );
  const cues = externalCues ?? fetched;
  const pgsTrack = externalCues ? null : pgs;
  const shifted = useMemo(() => (offsetMs ? shiftCues(cues, offsetMs) : cues), [cues, offsetMs]);

  // Exact-time cue selection, re-rendering the consumer only on a change.
  const textIdx = usePlayheadSelector((sec) => findActiveCueIndex(shifted, given ?? sec));
  const pgsIdx = usePlayheadSelector((sec) =>
    pgsTrack ? findActivePGSCue(pgsTrack.track.cues, (given ?? sec) * 1000 - offsetMs) : -1,
  );

  const text = textIdx >= 0 && textIdx < shifted.length ? shifted[textIdx].text : null;
  const pgsOut =
    pgsTrack && pgsIdx >= 0 && pgsTrack.track.screenW > 0 && pgsTrack.track.screenH > 0
      ? {
          uri: pgsTrack.uris[pgsIdx],
          cue: pgsTrack.track.cues[pgsIdx],
          screenW: pgsTrack.track.screenW,
          screenH: pgsTrack.track.screenH,
        }
      : null;

  return {
    preparing: preparing && !externalCues && !pgsTrack && cues.length === 0,
    text,
    pgs: pgsOut,
  };
}
