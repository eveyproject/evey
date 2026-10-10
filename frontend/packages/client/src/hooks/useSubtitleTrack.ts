import { useEffect, useState } from "react";
import {
  eveyFetchRaw,
  fetchSubtitleTrack,
  isPGSBlob,
  parsePGSBlob,
  parseVttCues,
  pgsCueDataURI,
  type PGSTrack,
  type SubtitleCue,
} from "@evey/api";
import {
  mergeCues,
  parseSubtitlePlaylist,
  segmentWindow,
  subtitleRenditionURI,
  type SubtitleSegment,
} from "../lib/segmentedSubtitles";

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
  /**
   * The server is still producing the track (202 + Retry-After: a read of the
   * whole film when no block index exists). The hook keeps polling; the
   * overlay says so instead of showing nothing.
   */
  preparing: boolean;
}

// Cached per (media, track): re-selecting a track already loaded, or a remount
// onto the next episode, must not re-fetch megabytes.
//
// Bounded, like the web overlay's cache: a decoded bitmap track is 15-17 MB of
// PNG (measured), held here as data URIs, and a binge is many tracks. Past the
// cap the least recently used goes; the one on screen is always the freshest.
const MAX_CACHED_TRACKS = 3;
const textCache = new Map<string, SubtitleCue[]>();
const pgsCache = new Map<string, { track: PGSTrack; uris: string[] }>();
// null = the packager has no rendition for this track: /subtitle it is.
const renditionCache = new Map<string, SubtitleSegment[] | null>();
const segmentCueCache = new Map<string, SubtitleCue[]>();

// Map iteration is insertion-ordered: re-inserting on every touch makes the
// first key the least recently used.
function lruGet<V>(cache: Map<string, V>, key: string): V | undefined {
  const v = cache.get(key);
  if (v === undefined) return undefined;
  cache.delete(key);
  cache.set(key, v);
  return v;
}

function lruSet<V>(cache: Map<string, V>, key: string, value: V): void {
  cache.delete(key);
  while (cache.size >= MAX_CACHED_TRACKS) {
    const oldest = cache.keys().next().value;
    if (oldest === undefined) break;
    cache.delete(oldest);
  }
  cache.set(key, value);
}

// Segments loaded ahead of the playhead, and how often the window moves.
const AHEAD_SEC = 60;
const BEHIND_SEC = 6;
const WINDOW_STEP_SEC = 10;

async function fetchText(url: string): Promise<string | null> {
  try {
    const res = await eveyFetchRaw({ url, method: "GET" });
    return res.ok ? await res.text() : null;
  } catch {
    return null;
  }
}

function hlsBase(mediaId: string): string {
  return `/api/v1/media/${encodeURIComponent(mediaId)}/hls/`;
}

// A master that answered is a definitive "no rendition"; a failed fetch is not
// cached, so the next selection asks again.
async function resolveSegments(mediaId: string, subtitleIndex: number): Promise<SubtitleSegment[] | null> {
  const key = `${mediaId}:${subtitleIndex}`;
  const known = renditionCache.get(key);
  if (known !== undefined) return known;
  const master = await fetchText(`${hlsBase(mediaId)}od_master.m3u8`);
  if (master === null) return null;
  const uri = subtitleRenditionURI(master, subtitleIndex);
  const playlist = uri ? await fetchText(hlsBase(mediaId) + uri) : null;
  if (uri && playlist === null) return null;
  const segments = playlist ? parseSubtitlePlaylist(playlist) : [];
  const result = segments.length > 0 ? segments : null;
  renditionCache.set(key, result);
  return result;
}

/**
 * `positionSec` opts into the packager's segmented rendition: only the
 * segments around the playhead are fetched, in about a second each, where
 * /subtitle reads the whole film first. Without it, or when the packager has
 * no rendition for the track (bitmap, a container it refuses), /subtitle.
 */
export function useSubtitleTrack(mediaId: string | undefined, subtitleIndex: number | null, positionSec?: number): SubtitleTrackData {
  const [cues, setCues] = useState<SubtitleCue[]>([]);
  const [pgs, setPgs] = useState<{ track: PGSTrack; uris: string[] } | null>(null);
  const [preparing, setPreparing] = useState(false);
  // Tagged with the track they belong to: a window effect still running for
  // the previous track must not pour its cues into the new one.
  const [rendition, setRendition] = useState<{ key: string; segments: SubtitleSegment[] } | null>(null);
  const segmented = positionSec !== undefined;
  const trackKey = mediaId && subtitleIndex !== null ? `${mediaId}:${subtitleIndex}` : null;

  useEffect(() => {
    setRendition(null);
    setPreparing(false);
    if (!mediaId || subtitleIndex === null) {
      setCues([]);
      setPgs(null);
      return;
    }
    const key = `${mediaId}:${subtitleIndex}`;
    const cachedPgs = lruGet(pgsCache, key);
    if (cachedPgs) {
      setPgs(cachedPgs);
      setCues([]);
      return;
    }
    const cachedText = lruGet(textCache, key);
    if (cachedText) {
      setCues(cachedText);
      setPgs(null);
      return;
    }
    setCues([]);
    setPgs(null);
    const aborter = new AbortController();
    void (async () => {
      try {
        if (segmented) {
          const segments = await resolveSegments(mediaId, subtitleIndex);
          if (aborter.signal.aborted) return;
          if (segments) {
            setRendition({ key, segments });
            return;
          }
        }
        // Read as bytes: decoding an image blob as UTF-8 would corrupt it
        // before anything could tell the two kinds apart. A 202 is followed
        // until the server has the track; the overlay shows "preparing".
        const result = await fetchSubtitleTrack({
          doFetch: () => eveyFetchRaw({
            url: `/api/v1/media/${encodeURIComponent(mediaId)}/subtitle/${subtitleIndex}`,
            method: "GET",
          }),
          onPreparing: () => setPreparing(true),
          signal: aborter.signal,
        });
        if (aborter.signal.aborted) return;
        setPreparing(false);
        const buf = result.bytes;
        if (result.state !== "ready" || !buf) return;
        if (isPGSBlob(buf)) {
          const track = parsePGSBlob(buf);
          const entry = { track, uris: track.cues.map(pgsCueDataURI) };
          lruSet(pgsCache, key, entry);
          setPgs(entry);
          return;
        }
        const parsed = parseVttCues(new TextDecoder().decode(buf));
        lruSet(textCache, key, parsed);
        setCues(parsed);
      } catch {
        // A subtitle that fails to load must never take playback with it.
        setPreparing(false);
      }
    })();
    return () => { aborter.abort(); };
  }, [mediaId, subtitleIndex, segmented]);

  // The window follows the playhead in steps, not on every time update.
  const windowStep = segmented ? Math.floor((positionSec ?? 0) / WINDOW_STEP_SEC) : 0;
  useEffect(() => {
    if (!mediaId || !rendition || rendition.key !== trackKey) return;
    const { segments } = rendition;
    const at = windowStep * WINDOW_STEP_SEC;
    let cancelled = false;
    void (async () => {
      // In order from the playhead: the segment on screen first.
      for (const i of segmentWindow(segments, at, AHEAD_SEC + WINDOW_STEP_SEC, BEHIND_SEC)) {
        if (cancelled) return;
        const cacheKey = `${rendition.key}:${segments[i].uri}`;
        let segCues = segmentCueCache.get(cacheKey);
        if (!segCues) {
          const vtt = await fetchText(hlsBase(mediaId) + segments[i].uri);
          if (vtt === null) continue; // the next window step retries it
          segCues = parseVttCues(vtt);
          segmentCueCache.set(cacheKey, segCues);
        }
        if (cancelled) return;
        const add = segCues;
        setCues((prev) => mergeCues(prev, add));
      }
    })();
    return () => { cancelled = true; };
  }, [mediaId, rendition, trackKey, windowStep]);

  return { cues, pgs, preparing };
}

/** Test seam: the number of tracks kept per kind. */
export const SUBTITLE_TRACK_CACHE_CAP = MAX_CACHED_TRACKS;
/** Test seam: how many tracks are cached right now, per kind. */
export function subtitleTrackCacheSizes(): { text: number; pgs: number } {
  return { text: textCache.size, pgs: pgsCache.size };
}
