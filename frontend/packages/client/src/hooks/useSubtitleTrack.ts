import { useEffect, useState } from "react";
import {
  eveyFetchRaw,
  isPGSBlob,
  parsePGSBlob,
  parseVttCues,
  pgsCueDataURI,
  type PGSTrack,
  type SubtitleCue,
} from "@evey/api";

/**
 * The subtitle track a player draws itself: the server answers WebVTT for a
 * text track and a decoded-image blob for a bitmap one, and the first bytes
 * say which. Drawing it here rather than asking the server to burn it in is
 * what keeps a direct-playing remux from turning into a full transcode.
 *
 * Both televisions fetched and sniffed this the same way in two copies; only
 * the painting differs between React Native and the DOM.
 */
export interface SubtitleTrackData {
  cues: SubtitleCue[];
  pgs: { track: PGSTrack; uris: string[] } | null;
}

// Cached per (media, track): re-selecting a track already loaded, or a remount
// onto the next episode, must not re-fetch megabytes.
const textCache = new Map<string, SubtitleCue[]>();
const pgsCache = new Map<string, { track: PGSTrack; uris: string[] }>();

export function useSubtitleTrack(mediaId: string | undefined, subtitleIndex: number | null): SubtitleTrackData {
  const [cues, setCues] = useState<SubtitleCue[]>([]);
  const [pgs, setPgs] = useState<{ track: PGSTrack; uris: string[] } | null>(null);

  useEffect(() => {
    if (!mediaId || subtitleIndex === null) {
      setCues([]);
      setPgs(null);
      return;
    }
    const key = `${mediaId}:${subtitleIndex}`;
    const cachedPgs = pgsCache.get(key);
    if (cachedPgs) {
      setPgs(cachedPgs);
      setCues([]);
      return;
    }
    const cachedText = textCache.get(key);
    if (cachedText) {
      setCues(cachedText);
      setPgs(null);
      return;
    }
    setCues([]);
    setPgs(null);
    let cancelled = false;
    void (async () => {
      try {
        const res = await eveyFetchRaw({
          url: `/api/v1/media/${encodeURIComponent(mediaId)}/subtitle/${subtitleIndex}`,
          method: "GET",
        });
        // Read as bytes: decoding an image blob as UTF-8 would corrupt it
        // before anything could tell the two kinds apart.
        const buf = new Uint8Array(await res.arrayBuffer());
        if (cancelled) return;
        if (isPGSBlob(buf)) {
          const track = parsePGSBlob(buf);
          const entry = { track, uris: track.cues.map(pgsCueDataURI) };
          pgsCache.set(key, entry);
          setPgs(entry);
          return;
        }
        const parsed = parseVttCues(new TextDecoder().decode(buf));
        textCache.set(key, parsed);
        setCues(parsed);
      } catch {
        // A subtitle that fails to load must never take playback with it.
      }
    })();
    return () => { cancelled = true; };
  }, [mediaId, subtitleIndex]);

  return { cues, pgs };
}
