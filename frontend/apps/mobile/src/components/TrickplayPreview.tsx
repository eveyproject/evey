import { useEffect, useState, useRef } from "react";
import { View, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { eveyFetchRaw } from "@evey/api";
import { useServerUrl } from "../hooks/useServerUrl";
import { useAuthHeaders } from "../hooks/useAuthenticatedImage";

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
const cueCache = new Map<string, TrickplayCue[]>();

interface Props {
  mediaId: string;
  currentTime: number;
  visible: boolean;
}

export function TrickplayPreview({ mediaId, currentTime, visible }: Props) {
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const [cues, setCues] = useState<TrickplayCue[]>(() => cueCache.get(mediaId) ?? []);
  const fetched = useRef(false);

  useEffect(() => {
    if (!mediaId || fetched.current) return;
    fetched.current = true;
    (async () => {
      try {
        const res = await eveyFetchRaw({ url: `/api/v1/media/${mediaId}/trickplay`, method: "GET" });
        const text = await res.text();
        const parsed = parseVTT(text);
        cueCache.set(mediaId, parsed);
        setCues(parsed);
      } catch {
        cueCache.set(mediaId, []);
      }
    })();
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
    <View style={[styles.container, { width: thumbW, height: thumbH }]}>
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
    backgroundColor: "rgba(0,0,0,0.8)",
  },
});
