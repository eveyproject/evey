import { useEffect, useRef, useState } from "react";
import { useAuthStore } from "@evey/store";

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
        /(\d{2}):(\d{2}):(\d{2})\.(\d{3})\s*-->\s*(\d{2}):(\d{2}):(\d{2})\.(\d{3})/
      );
      if (!match) continue;

      const startTime =
        parseInt(match[1]) * 3600 +
        parseInt(match[2]) * 60 +
        parseInt(match[3]) +
        parseInt(match[4]) / 1000;
      const endTime =
        parseInt(match[5]) * 3600 +
        parseInt(match[6]) * 60 +
        parseInt(match[7]) +
        parseInt(match[8]) / 1000;

      const urlLine = lines[i + 1]?.trim();
      if (!urlLine) continue;

      const xywh = urlLine.match(/#xywh=(\d+),(\d+),(\d+),(\d+)/);
      if (!xywh) continue;

      const url = urlLine.split("#")[0];
      cues.push({
        startTime,
        endTime,
        url,
        x: parseInt(xywh[1]),
        y: parseInt(xywh[2]),
        w: parseInt(xywh[3]),
        h: parseInt(xywh[4]),
      });
    }
  }
  return cues;
}

interface TrickplayPreviewProps {
  mediaId: string;
  hoverTime: number | null;
}

export function TrickplayPreview({ mediaId, hoverTime }: TrickplayPreviewProps) {
  const [cues, setCues] = useState<TrickplayCue[]>([]);
  const [loaded, setLoaded] = useState(false);
  const accessToken = useAuthStore((s) => s.accessToken);
  const imageCache = useRef<Map<string, HTMLImageElement>>(new Map());

  useEffect(() => {
    if (!mediaId || !accessToken) return;
    let cancelled = false;

    fetch(`/api/v1/media/${mediaId}/trickplay`, {
      headers: { Authorization: `Bearer ${accessToken}` },
    })
      .then((res) => {
        if (!res.ok) throw new Error("no trickplay");
        return res.text();
      })
      .then((text) => {
        if (cancelled) return;
        const parsed = parseVTT(text);
        setCues(parsed);
        setLoaded(parsed.length > 0);

        // Preload sprite images
        const urls = new Set(parsed.map((c) => c.url));
        for (const url of urls) {
          if (!imageCache.current.has(url)) {
            const img = new Image();
            img.src = `${url}?token=${accessToken}`;
            imageCache.current.set(url, img);
          }
        }
      })
      .catch(() => {
        if (!cancelled) setLoaded(false);
      });

    return () => { cancelled = true; };
  }, [mediaId, accessToken]);

  if (!loaded || hoverTime === null) return null;

  const cue = cues.find((c) => hoverTime >= c.startTime && hoverTime < c.endTime);
  if (!cue) return null;

  const imgSrc = imageCache.current.get(cue.url)?.src ?? `${cue.url}?token=${accessToken}`;
  const thumbW = cue.w;
  const thumbH = cue.h;

  return (
    <div
      className="rounded overflow-hidden shadow-lg pointer-events-none"
      style={{
        width: thumbW,
        height: thumbH,
        backgroundColor: "rgba(0,0,0,0.8)",
      }}
    >
      <div
        style={{
          width: thumbW,
          height: thumbH,
          backgroundImage: `url(${imgSrc})`,
          backgroundPosition: `-${cue.x}px -${cue.y}px`,
          backgroundSize: "auto",
          backgroundRepeat: "no-repeat",
        }}
      />
    </div>
  );
}
