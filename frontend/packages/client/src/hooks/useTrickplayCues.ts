import { useEffect, useState } from "react";
import { eveyFetchRaw } from "@evey/api";
import { parseTrickplayVTT, type TrickplayCue } from "../lib/trickplayCues";

// A television is left running for weeks, and every distinct title seeked into
// adds a parsed cue list that is never removed. Each entry is small, but
// "small times unbounded" is still unbounded. Bound it to the last few titles:
// trickplay is only ever consulted for what is playing now, so a tiny window
// loses nothing, and a re-seek into an evicted title just re-fetches it.
const TRICKPLAY_CACHE_MAX = 8;

const cueCache = new Map<string, TrickplayCue[]>();
const fetchedIds = new Set<string>();

function rememberCues(mediaId: string, parsed: TrickplayCue[]): void {
  cueCache.set(mediaId, parsed);
  while (cueCache.size > TRICKPLAY_CACHE_MAX) {
    const oldest = cueCache.keys().next().value;
    if (oldest === undefined) break;
    cueCache.delete(oldest);
    fetchedIds.delete(oldest); // else the evicted title can never re-fetch
  }
}

/**
 * The trickplay manifest for a title, fetched at most once and only when
 * someone is actually seeking.
 *
 * Lazy on purpose: nothing is fetched at rest, only once the viewer starts
 * seeking or the controls come up. A weak server should not pay for a
 * manifest on every playback session, only the ones where someone reaches for
 * the remote. A failure resolves to no cues: trickplay is a comfort layer,
 * never a reason for a seek to fail.
 */
export function useTrickplayCues(mediaId: string | undefined, wanted: boolean): TrickplayCue[] {
  const [cues, setCues] = useState<TrickplayCue[]>(() => (mediaId ? cueCache.get(mediaId) ?? [] : []));

  useEffect(() => {
    setCues(mediaId ? cueCache.get(mediaId) ?? [] : []);
  }, [mediaId]);

  useEffect(() => {
    if (!wanted || !mediaId || fetchedIds.has(mediaId)) return;
    fetchedIds.add(mediaId);
    let cancelled = false;
    void (async () => {
      try {
        const res = await eveyFetchRaw({ url: `/api/v1/media/${mediaId}/trickplay`, method: "GET" });
        const text = await res.text();
        if (cancelled) {
          // Nothing was stored, so the guard must not survive: it is only ever
          // cleaned by an eviction from the cue cache, and an id that never
          // entered it would never be evicted from it either. Left in place it
          // killed seek previews for this title until the app restarted.
          fetchedIds.delete(mediaId);
          return;
        }
        const parsed = parseTrickplayVTT(text);
        rememberCues(mediaId, parsed);
        setCues(parsed);
      } catch {
        // Transient failure (network blip, sprites still generating): let it
        // be asked again next time.
        fetchedIds.delete(mediaId);
      }
    })();
    return () => { cancelled = true; };
  }, [mediaId, wanted]);

  return cues;
}
