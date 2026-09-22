import { memo, useEffect, useState } from "react";
import { eveyFetchRaw, getBaseURL } from "@evey/api";
import { parseTrickplayVTT, cueForTime, type TrickplayCue } from "@evey/client";

// The picture at the seek target while the remote scrubs: the sprite sheet
// the server generated, cropped to the tile with background-position, the
// way the web player does it. Nothing to draw resolves to nothing.
const cueCache = new Map<string, TrickplayCue[]>();

interface Props {
  mediaId: string;
  previewTimeSec: number | null;
  // Fetch the manifest while the controls show so the first seek has a
  // picture too.
  warm: boolean;
}

function TrickplayTileImpl({ mediaId, previewTimeSec, warm }: Props) {
  const [cues, setCues] = useState<TrickplayCue[]>(() => cueCache.get(mediaId) ?? []);
  const wanted = warm || previewTimeSec !== null;
  useEffect(() => {
    if (!wanted || cueCache.has(mediaId)) { setCues(cueCache.get(mediaId) ?? []); return; }
    let cancelled = false;
    (async () => {
      try {
        const res = await eveyFetchRaw({ url: `/api/v1/media/${mediaId}/trickplay`, method: "GET" });
        const parsed = parseTrickplayVTT(await res.text());
        if (cancelled) return;
        cueCache.set(mediaId, parsed);
        setCues(parsed);
      } catch { /* no sprites yet */ }
    })();
    return () => { cancelled = true; };
  }, [mediaId, wanted]);

  // Warm the first sheet into the browser cache too.
  useEffect(() => {
    if (!warm || cues.length === 0) return;
    const img = new Image();
    img.src = `${getBaseURL()}${cues[0].url}`;
  }, [warm, cues]);

  if (previewTimeSec === null) return null;
  const cue = cueForTime(cues, previewTimeSec);
  if (!cue) return null;
  return (
    <div
      className="trickplay"
      style={{
        width: cue.w,
        height: cue.h,
        backgroundImage: `url("${getBaseURL()}${cue.url}")`,
        backgroundPosition: `-${cue.x}px -${cue.y}px`,
      }}
    />
  );
}

export const TrickplayTile = memo(TrickplayTileImpl);
