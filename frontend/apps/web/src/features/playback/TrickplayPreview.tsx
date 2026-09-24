import { useEffect, useRef, useState } from "react";
import { eveyFetchRaw } from "@evey/api";
// Parse and cue lookup are @evey/client's, shared with both televisions.
import { cueForTime, parseTrickplayVTT, type TrickplayCue } from "@evey/client";

const TRICKPLAY_TYPES = new Set(["MEDIA_ITEM_TYPE_MOVIE", "MEDIA_ITEM_TYPE_EPISODE"]);
// Module-level cache: one fetch per mediaId per page load
const fetchedIds = new Set<string>();
const cueCache = new Map<string, TrickplayCue[]>();

interface TrickplayPreviewProps {
  mediaId: string;
  hoverTime: number | null;
  mediaType?: string;
}

export function TrickplayPreview({ mediaId, hoverTime, mediaType }: TrickplayPreviewProps) {
  const [cues, setCues] = useState<TrickplayCue[]>(() => cueCache.get(mediaId) ?? []);
  const [loaded, setLoaded] = useState(() => (cueCache.get(mediaId)?.length ?? 0) > 0);
  const imageCache = useRef<Map<string, HTMLImageElement>>(new Map());
  const hasTrickplay = !mediaType || TRICKPLAY_TYPES.has(mediaType);

  // When mediaId changes without remount (e.g. next episode), evict stale cues
  // immediately so the previous episode's thumbnails are never shown on hover.
  useEffect(() => {
    setCues(cueCache.get(mediaId) ?? []);
    setLoaded((cueCache.get(mediaId)?.length ?? 0) > 0);
  }, [mediaId]);

  useEffect(() => {
    if (!mediaId || !hasTrickplay || fetchedIds.has(mediaId)) return;
    fetchedIds.add(mediaId);

    // The fetch spans two awaits. If mediaId changes in between (next episode),
    // a late resolve would setCues the PREVIOUS item's thumbnails - exactly the
    // invariant the mediaId effect above exists to protect. Today the player
    // remounts this component with key={mediaId} so it never bites, but that is
    // one refactor away from breaking silently; the cancelled flag makes the
    // guarantee local.
    let cancelled = false;
    (async () => {
      try {
        const res = await eveyFetchRaw({ url: `/api/v1/media/${mediaId}/trickplay`, method: "GET" });
        const text = await res.text();
        if (cancelled) return;
        const parsed = parseTrickplayVTT(text);
        cueCache.set(mediaId, parsed);
        setCues(parsed);
        setLoaded(parsed.length > 0);

        const urls = new Set(parsed.map((c) => c.url));
        for (const url of urls) {
          if (!imageCache.current.has(url)) {
            const img = new Image();
            img.src = url;
            imageCache.current.set(url, img);
          }
        }
      } catch {
        // A transient failure (network blip, sprite sheet still generating
        // server-side) must NOT permanently disable previews for this item:
        // drop the in-flight guard so a later hover can retry instead of being
        // blacklisted for the whole page session.
        fetchedIds.delete(mediaId);
      }
    })();
    return () => { cancelled = true; };
  }, [mediaId, hasTrickplay]);

  if (!hasTrickplay || !loaded || hoverTime === null) return null;

  const cue = cueForTime(cues, hoverTime);
  if (!cue) return null;

  const imgSrc = imageCache.current.get(cue.url)?.src ?? cue.url;
  const thumbW = cue.w;
  const thumbH = cue.h;

  return (
    <div
      className="rounded overflow-hidden shadow-lg pointer-events-none"
      style={{
        width: thumbW,
        height: thumbH,
        backgroundColor: "var(--overlay-bg-strong)",
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
