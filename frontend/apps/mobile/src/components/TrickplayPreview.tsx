import { useEffect, useState } from "react";
import { View, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { useTheme } from "../providers/ThemeProvider";
// From "/legacy", not the package root: in expo-file-system 55 the root's
// getInfoAsync/readAsStringAsync/makeDirectoryAsync/writeAsStringAsync are
// stubs that throw on the first call. See downloads.ts for the full story.
import * as FileSystem from "expo-file-system/legacy";
import { eveyFetchRaw } from "@evey/api";
import { useServerUrl } from "../hooks/useServerUrl";
import { useAuthHeaders } from "../hooks/useAuthenticatedImage";

const cacheDir = FileSystem.cacheDirectory ?? "";
const TRICKPLAY_CACHE_DIR = `${cacheDir}trickplay/`;

interface TrickplayCue {
  startTime: number;
  endTime: number;
  url: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

function parseVTT(text: string): TrickplayCue[] {
  const cues: TrickplayCue[] = [];
  const blocks = text.split("\n\n");
  for (const block of blocks) {
    const lines = block.trim().split("\n");
    for (let i = 0; i < lines.length; i++) {
      const match = lines[i].match(
        /(\d{2}):(\d{2}):(\d{2})\.(\d{3})\s*-->\s*(\d{2}):(\d{2}):(\d{2})\.(\d{3})/,
      );
      if (!match) continue;
      const startTime = parseInt(match[1]) * 3600 + parseInt(match[2]) * 60 + parseInt(match[3]) + parseInt(match[4]) / 1000;
      const endTime = parseInt(match[5]) * 3600 + parseInt(match[6]) * 60 + parseInt(match[7]) + parseInt(match[8]) / 1000;
      const urlLine = lines[i + 1]?.trim();
      if (!urlLine) continue;
      const xywh = urlLine.match(/#xywh=(\d+),(\d+),(\d+),(\d+)/);
      if (!xywh) continue;
      cues.push({
        startTime,
        endTime,
        url: urlLine.split("#")[0],
        x: parseInt(xywh[1]),
        y: parseInt(xywh[2]),
        w: parseInt(xywh[3]),
        h: parseInt(xywh[4]),
      });
    }
  }
  return cues;
}

const SCALE = 1;
const memCache = new Map<string, TrickplayCue[]>();

async function readCachedCues(mediaId: string): Promise<TrickplayCue[] | null> {
  try {
    const path = `${TRICKPLAY_CACHE_DIR}${mediaId}.json`;
    const info = await FileSystem.getInfoAsync(path);
    if (!info.exists) return null;
    const json = await FileSystem.readAsStringAsync(path);
    return JSON.parse(json);
  } catch { return null; }
}

async function writeCachedCues(mediaId: string, cues: TrickplayCue[]): Promise<void> {
  try {
    await FileSystem.makeDirectoryAsync(TRICKPLAY_CACHE_DIR, { intermediates: true });
    await FileSystem.writeAsStringAsync(`${TRICKPLAY_CACHE_DIR}${mediaId}.json`, JSON.stringify(cues));
  } catch (err) { console.warn("best effort", err); }
}

interface Props {
  mediaId: string;
  currentTime: number;
  visible: boolean;
}

export function TrickplayPreview({ mediaId, currentTime, visible }: Props) {
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const [cues, setCues] = useState<TrickplayCue[]>(() => memCache.get(mediaId) ?? []);

  // Re-fetch when mediaId changes (e.g. next episode in the player). The
  // memCache + on-disk cache short-circuits repeat fetches so this stays
  // cheap. The previous `fetched` ref was set on first mount and never
  // reset, so navigating to a sibling episode left cues empty for the rest
  // of the player session.
  useEffect(() => {
    if (!mediaId) return;
    const cachedSync = memCache.get(mediaId);
    if (cachedSync) {
      setCues(cachedSync);
      if (cachedSync.length > 0) return;
    } else {
      setCues([]);
    }
    let cancelled = false;
    (async () => {
      const cached = memCache.get(mediaId) ?? await readCachedCues(mediaId);
      if (cancelled) return;
      if (cached && cached.length > 0) {
        memCache.set(mediaId, cached);
        setCues(cached);
        return;
      }
      try {
        const res = await eveyFetchRaw({ url: `/api/v1/media/${mediaId}/trickplay`, method: "GET" });
        const text = await res.text();
        if (cancelled) return;
        const parsed = parseVTT(text);
        memCache.set(mediaId, parsed);
        setCues(parsed);
        writeCachedCues(mediaId, parsed);
      } catch {
        memCache.set(mediaId, []);
      }
    })();
    return () => { cancelled = true; };
  }, [mediaId]);

  if (!visible || cues.length === 0) return null;

  const cue = cues.find((c) => currentTime >= c.startTime && currentTime < c.endTime);
  if (!cue) return null;

  // Calculate sprite sheet dimensions from all cues sharing this URL
  const sameSrc = cues.filter((c) => c.url === cue.url);
  const spriteW = Math.max(...sameSrc.map((c) => c.x + c.w));
  const spriteH = Math.max(...sameSrc.map((c) => c.y + c.h));

  const thumbW = cue.w * SCALE;
  const thumbH = cue.h * SCALE;
  const imageUrl = serverUrl ? `${serverUrl}${cue.url}` : cue.url;

  return (
    <View style={[styles.container, { width: thumbW, height: thumbH, backgroundColor: theme.colors.overlay }]}>
      <Image
        source={{ uri: imageUrl, headers: authHeaders }}
        style={{
          position: "absolute",
          left: -cue.x * SCALE,
          top: -cue.y * SCALE,
          width: spriteW * SCALE,
          height: spriteH * SCALE,
        }}
        contentFit="fill"
      />
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    overflow: "hidden",
    borderRadius: 4,
  },
});
