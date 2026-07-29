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

interface TrickplaySeekPreviewProps {
  mediaId: string;
  // The seek target currently being previewed, in seconds - null whenever the
  // user isn't actively holding a seek key (see the player screen's
  // seekPreviewSec / playerSeek.ts).
  previewTimeSec: number | null;
}

// A thumbnail shown above the seek bar while the user holds left/right on the
// remote - the point of trickplay on TV: finding a scene blind, with no live
// video preview available during a D-pad seek. Purely a comfort layer: no
// manifest, no generated sprites, or a failed request all resolve to
// rendering nothing, exactly like the seek behaves today without it.
export function TrickplaySeekPreview({ mediaId, previewTimeSec }: TrickplaySeekPreviewProps) {
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
  useEffect(() => {
    if (previewTimeSec === null || !mediaId || fetchedIds.has(mediaId)) return;
    fetchedIds.add(mediaId);
    let cancelled = false;
    (async () => {
      try {
        const res = await eveyFetchRaw({ url: `/api/v1/media/${mediaId}/trickplay`, method: "GET" });
        const text = await res.text();
        if (cancelled) return;
        const parsed = parseTrickplayVTT(text);
        cueCache.set(mediaId, parsed);
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
  }, [mediaId, previewTimeSec]);

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
