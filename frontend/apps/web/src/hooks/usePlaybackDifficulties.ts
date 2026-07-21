import { useMemo } from "react";
import { useQueries } from "@tanstack/react-query";
import { playbackServiceGetPlaybackDifficulties } from "@evey/api";
import { getBrowserCapabilities, canWasmDecodeAC3 } from "@/utils/codec";
import {
  classifyPlaybackFacts,
  type PlaybackDifficulty,
} from "@/features/library/playbackDifficulty";

// usePlaybackDifficulties grades a whole list in ONE request.
//
// The per-item GetStreamInfo is what kept this off list views: an episode list
// would have fired a round trip per row. This asks the server once for the
// batch, and the answer is still device-specific because the same capability
// set travels with it.
//
// It goes through the raw generated function under useQueries rather than the
// generated mutation: this is a read, it should be cached and shared between
// the rows that need it, and it must not re-run on every render.

/** The server bounds one GetPlaybackDifficulties call at this many ids. */
const CHUNK_SIZE = 100;

export function usePlaybackDifficulties(mediaIds: string[]): Map<string, PlaybackDifficulty> {
  const caps = useMemo(() => getBrowserCapabilities(), []);
  const wasmAudio = useMemo(() => canWasmDecodeAC3(), []);

  // Chunked rather than capped. The server bounds one call at 100 ids, and an
  // infinite grid blows past that fast - a silent cap would leave later cards
  // quietly unbadged, which reads as "nothing to warn about" rather than "not
  // asked". Each chunk is its own cache entry, so scrolling costs one request
  // per new 100 items and never refetches the ones already answered.
  // Measured against the dev server: ~100ms for a 100-id batch.
  const chunks = useMemo(() => {
    const sorted = [...mediaIds].sort();
    const out: string[][] = [];
    for (let i = 0; i < sorted.length; i += CHUNK_SIZE) out.push(sorted.slice(i, i + CHUNK_SIZE));
    return out;
  }, [mediaIds]);

  const results = useQueries({
    queries: chunks.map((ids) => ({
      queryKey: ["playback-difficulties", ids.join(","), caps.videoCodecs, caps.audioCodecs, caps.hdr],
      // A title's codecs don't change while someone browses; the device's don't
      // either. Refetching on focus would be pure noise.
      staleTime: 5 * 60_000,
      refetchOnWindowFocus: false,
      retry: false,
      queryFn: ({ signal }: { signal?: AbortSignal }) =>
        playbackServiceGetPlaybackDifficulties(
          {
            mediaItemIds: ids,
            supportedCodecs: caps.videoCodecs,
            supportedAudioCodecs: caps.audioCodecs,
            supportedContainers: caps.containers,
            // No maxWidth/maxHeight, same as usePlaySession: screen size is not
            // a decode capability, and sending it makes the server report a
            // transcode for titles the device plays natively.
            supportsHdr: caps.hdr,
          },
          signal,
        ),
    })),
  });

  return useMemo(() => {
    const out = new Map<string, PlaybackDifficulty>();
    for (const r of results) {
      for (const item of r.data?.items ?? []) {
        if (!item.mediaItemId) continue;
        const d = classifyPlaybackFacts(item, wasmAudio);
        if (d) out.set(item.mediaItemId, d);
      }
    }
    return out;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [results.map((r) => r.dataUpdatedAt).join(","), wasmAudio]);
}
