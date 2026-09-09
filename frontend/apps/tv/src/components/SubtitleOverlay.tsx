import { memo, useEffect, useState } from "react";
import { View, Text, StyleSheet, Image, useWindowDimensions } from "react-native";
import {
  eveyFetchRaw,
  findActivePGSCue,
  isPGSBlob,
  parsePGSBlob,
  pgsCueDataURI,
  findActiveCueIndex,
  parseVttCues,
  type PGSTrack,
  type SubtitleCue,
} from "@evey/api";


/**
 * Drawing subtitles on the TV, which until now drew none at all.
 *
 * The TV had no client-side subtitle rendering: a selected track was passed to
 * the server as `?subtitleTrack=` and that was the whole story. For a TEXT track
 * the server answers a sidecar nobody fetched, so it simply never appeared. For
 * a BITMAP track the server burnt it into the video, which denies the copy rung
 * and turns a direct-playing remux into a full transcode.
 *
 * One component covers both, exactly as on mobile: the same route answers WebVTT
 * for a text track and a decoded-image blob for a bitmap one, and the first
 * bytes say which.
 */

interface Props {
  mediaId: string;
  /** Subtitle-relative ordinal to render, or null for none. */
  subtitleIndex: number | null;
  currentTimeSec: number;
}

// Cached per (media, track): re-selecting a track already loaded, or an
// autoplay remount onto the next episode, must not re-fetch megabytes.
const textCache = new Map<string, SubtitleCue[]>();
const pgsCache = new Map<string, { track: PGSTrack; uris: string[] }>();

function SubtitleOverlayImpl({ mediaId, subtitleIndex, currentTimeSec }: Props) {
  const { width: screenWidth, height: screenHeight } = useWindowDimensions();
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
    (async () => {
      try {
        const res = await eveyFetchRaw({
          url: `/api/v1/media/${encodeURIComponent(mediaId)}/subtitle/${subtitleIndex}`,
          method: "GET",
        });
        // Read as bytes: decoding the image blob as UTF-8 would corrupt it
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
    return () => {
      cancelled = true;
    };
  }, [mediaId, subtitleIndex]);

  if (pgs) {
    const { track, uris } = pgs;
    const idx = findActivePGSCue(track.cues, currentTimeSec * 1000);
    if (idx === -1 || track.screenW <= 0 || track.screenH <= 0) return null;
    const cue = track.cues[idx];
    // Scaled from the SUBTITLE plane, which is not the video resolution: every
    // 2160p disc measured carries a 1920x1080 plane, so using the media's
    // dimensions would offset and mis-size every cue by a factor of two.
    const sx = screenWidth / track.screenW;
    const sy = screenHeight / track.screenH;
    return (
      <View pointerEvents="none" style={StyleSheet.absoluteFill}>
        <Image
          source={{ uri: uris[idx] }}
          resizeMode="stretch"
          style={{ position: "absolute", left: cue.x * sx, top: cue.y * sy, width: cue.w * sx, height: cue.h * sy }}
        />
      </View>
    );
  }

  const active = findActiveCueIndex(cues, currentTimeSec);
  if (active === -1) return null;
  return (
    <View pointerEvents="none" style={styles.textContainer}>
      <Text style={styles.text}>{cues[active].text}</Text>
    </View>
  );
}

// Memoized: this is the one piece of the TV player that re-renders on every
// playhead tick, so it stays out of the screen's own render.
export const SubtitleOverlay = memo(SubtitleOverlayImpl);

const styles = StyleSheet.create({
  textContainer: {
    position: "absolute",
    left: 0,
    right: 0,
    bottom: "10%",
    alignItems: "center",
    paddingHorizontal: 48,
  },
  text: {
    color: "#fff",
    fontSize: 32,
    textAlign: "center",
    backgroundColor: "rgba(0,0,0,0.6)",
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 6,
    overflow: "hidden",
  },
});
