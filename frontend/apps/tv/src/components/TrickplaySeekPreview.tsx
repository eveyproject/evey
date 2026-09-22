import { useEffect, useState } from "react";
import { View, StyleSheet } from "react-native";
import { Image, type ImageRef } from "expo-image";
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

// One decoded sprite sheet per (token-bearing) cue url, loaded once through
// expo-image's loadAsync and handed to every tile as its source. React
// Native has no CSS background-size:auto - cropping a tile means rendering
// the WHOLE sheet at its own pixel size and shifting it, so that size has to
// come from the decoded image. It used to come from the onLoad of an
// invisible 1x1 probe <Image>, and on Android expo-image decodes to the
// view's size: the probe reported a sheet a few pixels wide, the tile was
// drawn at that size shifted out of its frame, and the viewer never saw a
// picture (measured on the emulator, 2026-09-22). loadAsync decodes at the
// original size and reports it.
interface Sheet {
  ref: ImageRef;
  // Pixel size of the sheet, the unit the manifest's #xywh tiles are in.
  width: number;
  height: number;
}
const sheetCache = new Map<string, Sheet>();
const sheetLoading = new Map<string, Promise<Sheet | null>>();

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
  // The sheet cache is keyed by cue URL, not media id, so it cannot be swept
  // by media id. A decoded sheet is a bitmap of a few megabytes: release them
  // wholesale once past a few titles' worth, re-decoding one costs one load.
  if (sheetCache.size > TRICKPLAY_CACHE_MAX * 4) {
    for (const sheet of sheetCache.values()) sheet.ref.release();
    sheetCache.clear();
  }
}

// loadSheet decodes a sprite sheet once and remembers it. A failure (network,
// sprites still generating, a token that expired) resolves to null and is
// forgotten, so the next seek tries again.
function loadSheet(url: string, headers: Record<string, string> | undefined): Promise<Sheet | null> {
  const cached = sheetCache.get(url);
  if (cached) return Promise.resolve(cached);
  const inFlight = sheetLoading.get(url);
  if (inFlight) return inFlight;
  const p = (async () => {
    try {
      const ref = await Image.loadAsync({ uri: url, headers });
      // `width`/`height` are logical, `scale` turns them into pixels: 1 on
      // Android, the asset scale on Apple TV.
      const scale = ref.scale > 0 ? ref.scale : 1;
      const sheet: Sheet = { ref, width: Math.round(ref.width * scale), height: Math.round(ref.height * scale) };
      if (sheet.width <= 0 || sheet.height <= 0) return null;
      sheetCache.set(url, sheet);
      return sheet;
    } catch {
      return null;
    } finally {
      sheetLoading.delete(url);
    }
  })();
  sheetLoading.set(url, p);
  return p;
}

interface TrickplaySeekPreviewProps {
  mediaId: string;
  // The seek target currently being previewed, in seconds - null whenever the
  // user isn't actively holding a seek key (see the player screen's
  // seekPreviewSec / playerSeek.ts).
  previewTimeSec: number | null;
  // The controls are on screen: a seek is likely next. Fetch the manifest
  // and decode the first sprite sheet now, so the FIRST seek shows a picture
  // too - fetched on the seek itself, the manifest and a sheet of a few
  // hundred kilobytes landed after the seek had committed, and the viewer
  // saw nothing until the second try.
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
  // Bumped whenever a sheet finishes decoding, purely to trigger the
  // re-render that lets the crop below read the now-populated sheetCache.
  const [, setSheetVersion] = useState(0);

  useEffect(() => {
    setCues(cueCache.get(mediaId) ?? []);
  }, [mediaId]);

  // Lazy on purpose: nothing is fetched at rest, only once the user actually
  // starts seeking (previewTimeSec goes non-null) or the controls come up. A
  // weak NAS shouldn't pay for a trickplay manifest on every playback
  // session, only the ones where someone reaches for the remote.
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

  const cue = previewTimeSec === null ? null : cueForTime(cues, previewTimeSec);
  // The sheet to have decoded: the previewed tile's, else the first one while
  // the controls show, so the first tile drawn does not wait on a download.
  const wantedSheetUrl = cue ? cue.url : warm && cues.length > 0 ? cues[0].url : null;
  const wantedSheetFull = wantedSheetUrl && serverUrl ? `${serverUrl}${wantedSheetUrl}` : null;
  useEffect(() => {
    if (!wantedSheetFull || sheetCache.has(wantedSheetFull)) return;
    let cancelled = false;
    void loadSheet(wantedSheetFull, authHeaders).then((sheet) => {
      if (!cancelled && sheet) setSheetVersion((n) => n + 1);
    });
    return () => {
      cancelled = true;
    };
  }, [wantedSheetFull, authHeaders]);

  if (!cue || !serverUrl) return null;

  const sheet = sheetCache.get(`${serverUrl}${cue.url}`);
  // Still decoding: nothing rather than a guessed (wrong) crop. The re-render
  // once it lands draws the tile on its first frame.
  if (!sheet) return null;

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
        // The decoded sheet, drawn at its pixel size and shifted so the
        // frame's clip shows the tile: one image pixel per layout unit, the
        // same unit the manifest's #xywh is in.
        source={sheet.ref}
        style={{
          position: "absolute",
          left: -cue.x,
          top: -cue.y,
          width: sheet.width,
          height: sheet.height,
        }}
        contentFit="fill"
        allowDownscaling={false}
      />
    </View>
  );
}

const styles = StyleSheet.create({
  frame: {
    overflow: "hidden",
    borderWidth: 1,
  },
});
