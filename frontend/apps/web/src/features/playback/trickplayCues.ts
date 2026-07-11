import { useEffect, useState } from "react";
import { eveyFetchRaw } from "@evey/api";

// Shared trickplay-cue access: the sprite-sheet WebVTT (one cue per interval,
// each carrying a `#xywh=` tile rect) that the player scrubber and the admin
// thumbnails both crop from. Trickplay assets are public (no auth), so a plain
// <img> can display a tile without a token.
export interface TrickplayCue {
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
      const startTime =
        parseInt(match[1]) * 3600 + parseInt(match[2]) * 60 + parseInt(match[3]) + parseInt(match[4]) / 1000;
      const endTime =
        parseInt(match[5]) * 3600 + parseInt(match[6]) * 60 + parseInt(match[7]) + parseInt(match[8]) / 1000;
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

const TRICKPLAY_TYPES = new Set(["MEDIA_ITEM_TYPE_MOVIE", "MEDIA_ITEM_TYPE_EPISODE"]);
// Module-level cache: one VTT fetch per mediaId per page load, shared by every
// admin thumbnail for the same media.
const fetchedIds = new Set<string>();
const cueCache = new Map<string, TrickplayCue[]>();

// useTrickplayCues returns the trickplay cues for a media, or [] when there is no
// trickplay (yet) - the caller then falls back to the on-demand /snapshot frame.
export function useTrickplayCues(mediaId: string | undefined, mediaType?: string): TrickplayCue[] {
  const [cues, setCues] = useState<TrickplayCue[]>(() => (mediaId ? cueCache.get(mediaId) ?? [] : []));
  const hasTrickplay = !mediaType || TRICKPLAY_TYPES.has(mediaType);

  useEffect(() => {
    setCues(mediaId ? cueCache.get(mediaId) ?? [] : []);
  }, [mediaId]);

  useEffect(() => {
    if (!mediaId || !hasTrickplay || fetchedIds.has(mediaId)) return;
    fetchedIds.add(mediaId);
    (async () => {
      try {
        const res = await eveyFetchRaw({ url: `/api/v1/media/${mediaId}/trickplay`, method: "GET" });
        const parsed = parseVTT(await res.text());
        cueCache.set(mediaId, parsed);
        setCues(parsed);
      } catch {
        cueCache.set(mediaId, []);
      }
    })();
  }, [mediaId, hasTrickplay]);

  return cues;
}

// tileForTime finds the cue covering positionMs, or null.
export function tileForTime(cues: TrickplayCue[], positionMs: number): TrickplayCue | null {
  const t = positionMs / 1000;
  return cues.find((c) => t >= c.startTime && t < c.endTime) ?? null;
}
