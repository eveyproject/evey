import { memo, useMemo } from "react";
import type { Chapter } from "@/hooks/useChapters";

interface ChapterMarkersProps {
  chapters: Chapter[];
  durationSec: number;
}

// Per-chapter geometry: depends only on the chapter list + duration, never on the
// playhead, so it is computed once (useMemo) instead of on every time tick.
interface ChapterGeometry {
  startPct: number;
  startMs: number;
  title: string;
}

// Chapters DIVIDE the bar. They do not paint it, they do not own the click, and
// they say nothing about the playhead. Two bugs came from forgetting that:
//
//  1. They used to sit in z-float (30) ABOVE the seek input (z-raised, 10) and
//     swallow the click: clicking mid-bar jumped to the START of the chapter
//     instead of seeking where you actually clicked. They are pointer-events-none
//     now - the range input owns every click, and jumping to a chapter is what the
//     chapters PANEL is for.
//  2. They used to paint a translucent block across their whole width, which sat on
//     top of the played + buffered layers and washed the bar into one flat grey -
//     you could no longer see where you were.
//
// What is left is the only thing a chapter should contribute to a progress bar: a
// notch at its boundary. Nothing under the bar (that space reads as "buffered"),
// nothing over the progress.
const ChapterNotch = memo(function ChapterNotch({ geo }: { geo: ChapterGeometry }) {
  return (
    <span
      aria-hidden
      title={geo.title}
      className="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-[2px] h-[10px]
                 rounded-full group-hover:h-3 transition-all"
      style={{ left: `${geo.startPct}%`, backgroundColor: "var(--bg)", opacity: 0.75 }}
    />
  );
});

export function ChapterMarkers({ chapters, durationSec }: ChapterMarkersProps) {
  const notches = useMemo<ChapterGeometry[]>(() => {
    if (durationSec <= 0) return [];
    return chapters
      .map((ch) => ({
        startPct: (ch.startTimeMs / 1000 / durationSec) * 100,
        startMs: ch.startTimeMs,
        title: ch.title,
      }))
      // The first chapter starts at 0: a notch on the bar's own edge is noise.
      .filter((g) => g.startPct > 0 && g.startPct < 100);
  }, [chapters, durationSec]);

  if (notches.length === 0) return null;

  return (
    <div aria-hidden className="pointer-events-none absolute inset-0">
      {notches.map((geo) => (
        <ChapterNotch key={geo.startMs} geo={geo} />
      ))}
    </div>
  );
}
