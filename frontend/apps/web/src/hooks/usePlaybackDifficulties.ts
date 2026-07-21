import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
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
// It is a useQuery over the raw generated function rather than the generated
// mutation: this is a read, it should be cached and shared between the rows
// that need it, and it must not re-run on every render.
export function usePlaybackDifficulties(mediaIds: string[]): Map<string, PlaybackDifficulty> {
  const caps = useMemo(() => getBrowserCapabilities(), []);
  const wasmAudio = useMemo(() => canWasmDecodeAC3(), []);

  // Sorted + joined so the cache key is stable whatever order the rows arrive
  // in, and so a re-render with the same set is a cache hit rather than a refetch.
  const key = useMemo(() => [...mediaIds].sort().join(","), [mediaIds]);

  const { data } = useQuery({
    queryKey: ["playback-difficulties", key, caps.videoCodecs, caps.audioCodecs, caps.hdr],
    enabled: mediaIds.length > 0,
    // A title's codecs don't change while someone browses; the device's don't
    // either. Refetching on focus would be pure noise.
    staleTime: 5 * 60_000,
    refetchOnWindowFocus: false,
    retry: false,
    queryFn: ({ signal }) =>
      playbackServiceGetPlaybackDifficulties(
        {
          mediaItemIds: mediaIds,
          supportedCodecs: caps.videoCodecs,
          supportedAudioCodecs: caps.audioCodecs,
          supportedContainers: caps.containers,
          // No maxWidth/maxHeight, same as usePlaySession: screen size is not a
          // decode capability, and sending it makes the server report a
          // transcode for titles the device plays natively.
          supportsHdr: caps.hdr,
        },
        signal,
      ),
  });

  return useMemo(() => {
    const out = new Map<string, PlaybackDifficulty>();
    for (const item of data?.items ?? []) {
      if (!item.mediaItemId) continue;
      const d = classifyPlaybackFacts(item, wasmAudio);
      if (d) out.set(item.mediaItemId, d);
    }
    return out;
  }, [data, wasmAudio]);
}
