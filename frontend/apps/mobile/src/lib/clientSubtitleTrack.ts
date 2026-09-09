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
 *
 * A PGS track is NOT a burn-in any more - the server decodes it once into an
 * image blob and this returns its ordinal like any other, so the overlay fetches
 * and draws it. That is what stops a bitmap subtitle from costing a transcode.
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

/**
 * The other half of the same question: which subtitle index the SERVER is
 * allowed to see, i.e. what may go in `?subtitleTrack=` on the stream URL.
 *
 * That parameter is not "the subtitle the viewer chose", it is an instruction to
 * burn a track into the frames. Burn-in denies the video copy rung, so passing
 * it turns a DirectPlay into a full transcode - on a 4K HEVC HDR remux, a
 * tonemapped four-rung x264 ladder on a box calibrated for one encode.
 *
 * Only a track the player CANNOT draw is a server concern - a bitmap with no
 * decoder on our side (VobSub, DVB). Text is drawn by SubtitleOverlay, and so is
 * PGS now, so neither must reach the URL. Nor must a burn-in track the viewer
 * has not actually asked to see:
 * pickDefaultSubtitle auto-selects the track the FILE flags as default, image
 * tracks included, without restarting the session for it. The selection and the
 * running stream therefore diverge by design - the subtitle is selected in the
 * UI and burnt into nothing - and every restart for an unrelated reason (audio
 * track, quality, a retry after a dead token) used to quietly close that gap by
 * sending the index along. Prod, 2026-09-08: a 4K remux direct-playing fine
 * became a transcoded ladder the instant the viewer changed audio track, and
 * the encoder never kept up.
 *
 * Returns undefined - the "omit the param" value buildStreamUrl expects - unless
 * the selected track is a bitmap one.
 */
export function burnInSubtitleIndex(
  selectedIndex: number | null,
  streams: ClientSubtitleCandidate[],
): number | undefined {
  if (selectedIndex === null) return undefined;
  return streams.find((s) => s.index === selectedIndex)?.burnIn ? selectedIndex : undefined;
}
