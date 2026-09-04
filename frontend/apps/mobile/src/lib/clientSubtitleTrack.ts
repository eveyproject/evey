/**
 * Which subtitle track the overlay should fetch and draw itself, given the
 * user's selection.
 *
 * Two different indices are in play on this screen and mixing them is silent:
 *
 *   index      - the ABSOLUTE media-stream index (video 0, audio 1, subtitle 2…).
 *                What the gRPC playback calls and the stream URL want.
 *   trackIndex - the SUBTITLE-RELATIVE ordinal (first subtitle 0, second 1…).
 *                What GET /api/v1/media/{id}/subtitle/{track} counts.
 *
 * The selection is carried as the absolute index (the restart path needs it),
 * and it used to be handed to the overlay unchanged - so on any ordinary file,
 * where the first subtitle sits at absolute index 2, picking it fetched
 * "subtitle/2", i.e. the THIRD subtitle track, or a 404 when there was no third.
 * The subtitle was simply wrong or absent, with nothing in the UI to say so.
 *
 * Returns null when nothing is selected, when the selection is unknown, or when
 * it is a burn-in track: those the server has already drawn into the video, and
 * drawing them again would double every line.
 */
export interface ClientSubtitleCandidate {
  index: number;
  trackIndex: number;
  burnIn?: boolean;
}

export function clientSubtitleTrack(
  selectedIndex: number | null,
  streams: ClientSubtitleCandidate[],
): number | null {
  if (selectedIndex === null) return null;
  const stream = streams.find((s) => s.index === selectedIndex);
  if (!stream || stream.burnIn) return null;
  return stream.trackIndex;
}
