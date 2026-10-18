import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { fetchSubtitleTrack, findActivePGSCue, isPGSBlob, parsePGSBlob, type PGSTrack } from "@evey/api";

/**
 * Drawing a BITMAP subtitle over the video instead of having the server burn it in.
 *
 * A `<track>` element cannot help here: it parses WebVTT, and a bitmap subtitle
 * carries pictures, not text. Until now that left burn-in as the only way to
 * show one - and a burn-in denies the video copy rung, so a single forced
 * subtitle (the track a retail rip flags DEFAULT) turned a 4K remux that
 * direct-played fine into a full transcode. The server now decodes the track
 * once into an image blob; this draws it.
 *
 * One fetch per track, not per cue: a track is around a thousand cues.
 */

interface Props {
  mediaId: string;
  /** Subtitle-relative ordinal of the bitmap track, or null when none is shown. */
  trackIndex: number | null;
  streamToken: string | null;
  /** The <video> to overlay. Its box is the reference for scaling the cues. */
  videoRef: React.RefObject<HTMLVideoElement | null>;
  /** Viewer resync, milliseconds, same sign convention as the text path. */
  offsetMs?: number;
}

// Cached per (media, track): switching back to a track already loaded, or a
// next-episode remount reusing a sibling's stream, must not re-fetch megabytes.
const trackCache = new Map<string, PGSTrack>();
// One object URL per cue, built once per track. Revoking them is what keeps a
// long viewing session from leaking a blob URL per subtitle shown.
const urlCache = new Map<string, string[]>();
// A binge is many tracks, and each one is megabytes of PNG held twice (the
// parsed cues and their blobs). Past this many the oldest is dropped and its
// URLs revoked; the one on screen is always the most recently touched.
const maxCachedTracks = 3;

function touchCached(key: string): { track: PGSTrack; urls: string[] } | null {
  const track = trackCache.get(key);
  const urls = urlCache.get(key);
  if (!track || !urls) return null;
  trackCache.delete(key);
  urlCache.delete(key);
  trackCache.set(key, track);
  urlCache.set(key, urls);
  return { track, urls };
}

function remember(key: string, track: PGSTrack, built: string[]): string[] {
  // Two mounts fetching the same track at once: the second to finish keeps
  // the first one's URLs instead of leaking its own on top.
  const existing = touchCached(key);
  if (existing) {
    for (const u of built) URL.revokeObjectURL(u);
    return existing.urls;
  }
  while (trackCache.size >= maxCachedTracks) {
    const oldest = trackCache.keys().next().value;
    if (oldest === undefined) break;
    for (const u of urlCache.get(oldest) ?? []) URL.revokeObjectURL(u);
    trackCache.delete(oldest);
    urlCache.delete(oldest);
  }
  trackCache.set(key, track);
  urlCache.set(key, built);
  return built;
}

export function PGSSubtitleOverlay({ mediaId, trackIndex, streamToken, videoRef, offsetMs = 0 }: Props) {
  const { t } = useTranslation();
  const [track, setTrack] = useState<PGSTrack | null>(null);
  const [urls, setUrls] = useState<string[]>([]);
  const [active, setActive] = useState(-1);
  // The server is still producing the track (202 + Retry-After). Shown as a
  // line where the subtitle would be, because the alternative - a menu entry
  // that draws nothing for minutes - reads as broken.
  const [preparing, setPreparing] = useState(false);
  const hintRef = useRef(0);

  useEffect(() => {
    // The previous track's cue index means nothing in the next one (a forced
    // track has a few dozen cues where the full one had a thousand), and its
    // images must not stay drawn while the next loads or if it fails.
    setTrack(null);
    setUrls([]);
    setActive(-1);
    setPreparing(false);
    hintRef.current = 0;
    if (trackIndex === null || !mediaId || !streamToken) return;
    const key = `${mediaId}:${trackIndex}`;
    const cached = touchCached(key);
    if (cached) {
      setTrack(cached.track);
      setUrls(cached.urls);
      return;
    }
    const aborter = new AbortController();
    void (async () => {
      try {
        // Follows the server's 202s until the blob exists - a track without a
        // block index is a read of the whole film.
        const result = await fetchSubtitleTrack({
          doFetch: () => fetch(
            `/api/v1/media/${encodeURIComponent(mediaId)}/subtitle/${trackIndex}?token=${encodeURIComponent(streamToken)}`,
          ),
          onPreparing: () => setPreparing(true),
          signal: aborter.signal,
        });
        if (aborter.signal.aborted) return;
        setPreparing(false);
        const buf = result.bytes;
        if (result.state !== "ready" || !buf) return;
        // The same route answers WebVTT for a text track. Reading the magic is
        // what keeps this component from trying to draw one.
        if (!isPGSBlob(buf)) return;
        const parsed = parsePGSBlob(buf);
        // Copy each slice out of the shared buffer: a subarray is a view, and
        // Blob wants a standalone ArrayBuffer.
        const built = parsed.cues.map((c) =>
          URL.createObjectURL(new Blob([new Uint8Array(c.png)], { type: "image/png" })),
        );
        setTrack(parsed);
        setUrls(remember(key, parsed, built));
      } catch {
        // A subtitle that fails to load must never take playback with it.
        setPreparing(false);
      }
    })();
    return () => {
      aborter.abort();
    };
  }, [mediaId, trackIndex, streamToken]);

  // Follow the playhead off the video's own event rather than a React tick: the
  // text path does the same, and re-rendering the player at 4 Hz to move one
  // image would cost more than the drawing.
  useEffect(() => {
    const v = videoRef.current;
    if (!v || !track) return;
    const tick = () => {
      const idx = findActivePGSCue(track.cues, v.currentTime * 1000 - offsetMs, hintRef.current);
      if (idx >= 0) hintRef.current = idx;
      setActive((prev) => (prev === idx ? prev : idx));
    };
    tick();
    v.addEventListener("timeupdate", tick);
    v.addEventListener("seeked", tick);
    return () => {
      v.removeEventListener("timeupdate", tick);
      v.removeEventListener("seeked", tick);
    };
  }, [videoRef, track, offsetMs]);

  if (preparing && !track) {
    return (
      <div className="pointer-events-none absolute inset-x-0 bottom-[8%] flex justify-center">
        <span className="rounded bg-black/70 px-3 py-1 text-sm text-white/90" data-testid="subtitle-preparing">
          {t("player.subtitlesPreparing")}
        </span>
      </div>
    );
  }
  if (!track || active < 0 || track.screenW <= 0 || track.screenH <= 0) return null;
  const cue = track.cues[active];
  if (!cue) return null;

  // Percentages, not pixels: the player box is resized by fullscreen, rotation
  // and window drags, and a percentage follows all three without a listener.
  // The reference is the SUBTITLE plane, which is not the video resolution -
  // every 2160p disc measured carries a 1920x1080 plane, so using the media's
  // dimensions would offset and mis-size every cue by a factor of two.
  return (
    <div className="pointer-events-none absolute inset-0 overflow-hidden">
      <img
        src={urls[active]}
        alt=""
        className="absolute"
        style={{
          left: `${(cue.x / track.screenW) * 100}%`,
          top: `${(cue.y / track.screenH) * 100}%`,
          width: `${(cue.w / track.screenW) * 100}%`,
          height: `${(cue.h / track.screenH) * 100}%`,
        }}
      />
    </div>
  );
}
