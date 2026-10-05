// The cue fetch, cache and parse are @evey/client's (useTrickplayCues), shared
// with both televisions and the phone. The web's own copy cached an empty list
// for good after a transient failure (sprites still generating), so the preview
// stayed off until a reload, and its cache grew without bound. This module
// keeps the web's signature: a media type instead of a "wanted" flag, and a
// lookup in milliseconds.
import { cueForTime, useTrickplayCues as useSharedTrickplayCues, type TrickplayCue } from "@evey/client";

export type { TrickplayCue };

// Only films and episodes carry trickplay sprites. The admin sessions API
// spells them "movie" / "episode", the catalog the enum way.
const TRICKPLAY_TYPES = new Set(["MEDIA_ITEM_TYPE_MOVIE", "MEDIA_ITEM_TYPE_EPISODE", "movie", "episode"]);

export function useTrickplayCues(mediaId: string | undefined, mediaType?: string): TrickplayCue[] {
  return useSharedTrickplayCues(mediaId, !mediaType || TRICKPLAY_TYPES.has(mediaType));
}

// The cue covering positionMs, or null.
export function tileForTime(cues: TrickplayCue[], positionMs: number): TrickplayCue | null {
  return cueForTime(cues, positionMs / 1000);
}
