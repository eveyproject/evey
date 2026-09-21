import { useEffect, useState } from "react";
import { View, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { eveyFetchRaw } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { useServerUrl } from "../hooks/useServerUrl";
import { useAuthHeaders } from "../hooks/useAuthenticatedImage";
import { parseTrickplayVTT, cueForTime, type TrickplayCue } from "../lib/trickplayCues";

// Module-level, like the web player's own trickplay cache: one manifest fetch
// per mediaId per app session, shared across every mount of this component
// (a fresh instance is created whenever the player screen remounts).
const cueCache = new Map<string, TrickplayCue[]>();
const fetchedIds = new Set<string>();
// Native pixel size of each sprite sheet, keyed by its (token-bearing) cue
// url. React Native has no CSS background-size:auto - cropping a tile means
// rendering the WHOLE sheet at its own native size and shifting it, so that
// size has to be learned from the image itself (see the render below).
const sheetSizeCache = new Map<string, { width: number; height: number }>();

// A TV app is left running for weeks, and every distinct title seeked into adds
// a parsed cue list that is never removed. Each entry is small, but "small
// times unbounded" is still unbounded - and this is the device with the least
// memory of the three. Bound it to the last few titles: trickplay is only ever
// consulted for what is playing now, so a tiny window loses nothing in
// practice, and a re-seek into an evicted title just re-fetches its manifest.
const TRICKPLAY_CACHE_MAX = 8;

// rememberCues inserts into the LRU-ish window, dropping the oldest media id
// once past the cap. Map preserves insertion order, so the first key is the
// least recently ADDED - good enough here, where the working set is one title.
function rememberCues(mediaId: string, parsed: TrickplayCue[]): void {
  cueCache.set(mediaId, parsed);
  while (cueCache.size > TRICKPLAY_CACHE_MAX) {
    const oldest = cueCache.keys().next().value;
    if (oldest === undefined) break;
    cueCache.delete(oldest);
    fetchedIds.delete(oldest); // else the evicted title can never re-fetch
  }
  // The sheet-size cache is keyed by cue URL, not media id, so it cannot be
  // swept by media id. Clear it wholesale when it grows past a few titles'
  // worth of sheets: re-learning a size costs one image load.
  if (sheetSizeCache.size > TRICKPLAY_CACHE_MAX * 8) sheetSizeCache.clear();
}

interface TrickplaySeekPreviewProps {
  mediaId: string;
  // The seek target currently being previewed, in seconds - null whenever the
  // user isn't actively holding a seek key (see the player screen's
  // seekPreviewSec / playerSeek.ts).
  previewTimeSec: number | null;
  // The controls are on screen: a seek is likely next. Fetch the manifest
  // and the first sprite sheet now, so the FIRST seek shows a picture too -
  // fetched on the seek itself, the manifest and a sheet of a few hundred
  // kilobytes landed after the seek had committed, and the viewer saw
  // nothing until the second try.
  warm?: boolean;
}

// A thumbnail shown above the seek bar while the user holds left/right on the
// remote - the point of trickplay on TV: finding a scene blind, with no live
// video preview available during a D-pad seek. Purely a comfort layer: no
// manifest, no generated sprites, or a failed request all resolve to
// rendering nothing, exactly like the seek behaves today without it.
export function TrickplaySeekPreview({ mediaId, previewTimeSec, warm = false }: TrickplaySeekPreviewProps) {
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const [cues, setCues] = useState<TrickplayCue[]>(() => cueCache.get(mediaId) ?? []);
  // Bumped whenever a sheet's native size resolves, purely to trigger the
  // re-render that lets the crop below read the now-populated sheetSizeCache.
  const [, setSizeVersion] = useState(0);

  useEffect(() => {
    setCues(cueCache.get(mediaId) ?? []);
  }, [mediaId]);

  // Lazy on purpose: nothing is fetched at rest, only once the user actually
  // starts seeking (previewTimeSec goes non-null). A weak NAS shouldn't pay
  // for a trickplay manifest on every playback session, only the ones where
  // someone reaches for the D-pad.
  const wanted = warm || previewTimeSec !== null;
  useEffect(() => {
    if (!wanted || !mediaId || fetchedIds.has(mediaId)) return;
    fetchedIds.add(mediaId);
    let cancelled = false;
    (async () => {
      try {
        const res = await eveyFetchRaw({ url: `/api/v1/media/${mediaId}/trickplay`, method: "GET" });
        const text = await res.text();
        if (cancelled) {
          // Nothing was stored, so the guard must not survive: it is only
          // ever cleaned by an LRU eviction from cueCache, and an id that
          // never entered the cache would never be evicted from it either.
          // Left in place it did two things - killed seek previews for this
          // title until the app restarted, and grew the set without bound on
          // a box that stays up for weeks.
          fetchedIds.delete(mediaId);
          return;
        }
        const parsed = parseTrickplayVTT(text);
        rememberCues(mediaId, parsed);
        setCues(parsed);
      } catch {
        // Transient failure (network blip, sprites still generating
        // server-side) must not permanently disable previews for this item -
        // drop the guard so the next seek can retry.
        fetchedIds.delete(mediaId);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [mediaId, wanted]);

  // The first sheet, warmed into expo-image's cache while the controls show,
  // so the first tile drawn does not wait on a download.
  useEffect(() => {
    if (!warm || !serverUrl || cues.length === 0) return;
    const first = cues[0];
    void Image.prefetch?.(`${serverUrl}${first.url}`, { headers: authHeaders })?.catch(() => {});
  }, [warm, serverUrl, cues, authHeaders]);

  if (previewTimeSec === null || cues.length === 0 || !serverUrl) return null;

  const cue = cueForTime(cues, previewTimeSec);
  if (!cue) return null;

  const fullUrl = `${serverUrl}${cue.url}`;
  const sheetSize = sheetSizeCache.get(cue.url);

  if (!sheetSize) {
    // The sheet's native size isn't known yet: render an invisible probe to
    // measure it via onLoad rather than draw a guessed (wrong) crop. Once
    // cached, every other tile from the same sheet renders correctly on its
    // very first frame.
    return (
      <Image
        source={{ uri: fullUrl, headers: authHeaders }}
        style={styles.probe}
        onLoad={(e) => {
          const { width, height } = e.source;
          if (width > 0 && height > 0) {
            sheetSizeCache.set(cue.url, { width, height });
            setSizeVersion((n) => n + 1);
          }
        }}
      />
    );
  }

  return (
    <View
      style={[
        styles.frame,
        {
          width: cue.w,
          height: cue.h,
          borderRadius: theme.radii.sm,
          borderColor: theme.colors.border,
          backgroundColor: theme.colors.overlay,
        },
      ]}
    >
      <Image
        // The tile url already carries its own media-scoped ?token= (see the
        // server's trickplay spriteCredential) - used verbatim, stripping the
        // query would 401 the image just like on web.
        source={{ uri: fullUrl, headers: authHeaders }}
        style={{
          position: "absolute",
          left: -cue.x,
          top: -cue.y,
          width: sheetSize.width,
          height: sheetSize.height,
        }}
        contentFit="fill"
        cachePolicy="memory-disk"
      />
    </View>
  );
}

const styles = StyleSheet.create({
  frame: {
    overflow: "hidden",
    borderWidth: 1,
  },
  probe: {
    position: "absolute",
    width: 1,
    height: 1,
    opacity: 0,
  },
});
