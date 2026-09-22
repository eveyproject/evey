import { useEffect, useState } from "react";
import { View, StyleSheet } from "react-native";
import { Image, type ImageRef } from "expo-image";
import { useTheme } from "../providers/ThemeProvider";
import { useServerUrl } from "../hooks/useServerUrl";
import { useAuthHeaders } from "../hooks/useAuthenticatedImage";
import { cueForTime } from "../lib/trickplayCues";
import { useTrickplayCues } from "@evey/client";

// One decoded sprite sheet per cue url. The manifest itself, its bounded
// cache and the fetch guard live in @evey/client (useTrickplayCues), shared
// with the browser television.
interface Sheet {
  ref: ImageRef;
  // Pixel size of the sheet, the unit the manifest's #xywh tiles are in.
  width: number;
  height: number;
}
const sheetCache = new Map<string, Sheet>();
const sheetLoading = new Map<string, Promise<Sheet | null>>();

// A decoded sheet is a bitmap of a few megabytes and the cache is keyed by
// cue URL, so it cannot be swept by media id: release them wholesale once
// past a few titles' worth. Re-decoding one costs a single load.
const SHEET_CACHE_MAX = 32;

function sweepSheets(): void {
  if (sheetCache.size <= SHEET_CACHE_MAX) return;
  for (const sheet of sheetCache.values()) sheet.ref.release();
  sheetCache.clear();
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
  const cues = useTrickplayCues(mediaId, warm || previewTimeSec !== null);
  useEffect(() => { sweepSheets(); }, [cues]);
  // Bumped whenever a sheet finishes decoding, purely to trigger the
  // re-render that lets the crop below read the now-populated sheetCache.
  const [, setSheetVersion] = useState(0);


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
