import { memo, useCallback, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import type { Chapter } from "@/hooks/useChapters";

interface ChapterMarkersProps {
  chapters: Chapter[];
  durationSec: number;
  currentTimeSec: number;
  onSeek: (timeSec: number) => void;
}

// Per-chapter geometry: depends only on the chapter list + duration, never on
// the playhead, so it is computed once (useMemo) instead of on every time tick.
interface ChapterGeometry {
  startPct: number;
  widthPct: number;
  startMs: number;
  endMs: number;
  startSec: number;
  label: string;
  title: string;
  showSeparator: boolean;
}

interface ChapterMarkerRowProps {
  geo: ChapterGeometry;
  isActive: boolean;
  onSeek: (timeSec: number) => void;
}

// memo so only the two markers whose active state flips at a chapter boundary
// re-render on a time tick, not the whole list. Relies on a stable `onSeek`
// (the parent stabilises it via a ref) and a memoised `geo` (stable identity
// across ticks).
const ChapterMarkerRow = memo(function ChapterMarkerRow({ geo, isActive, onSeek }: ChapterMarkerRowProps) {
  return (
    <button
      type="button"
      aria-label={geo.label}
      title={geo.title}
      onClick={(e) => {
        e.stopPropagation();
        onSeek(geo.startSec);
      }}
      className="absolute top-0 bottom-0 flex items-center cursor-pointer z-float"
      style={{ left: `${geo.startPct}%`, width: `${geo.widthPct}%` }}
    >
      <span
        className="block w-full h-1 rounded group-hover:h-1.5 transition-all"
        style={{
          backgroundColor: isActive ? "var(--accent)" : "var(--overlay-chapter-marker)",
          opacity: isActive ? 0.5 : 0.3,
        }}
      />
      {/* separator line between chapters (skip first) */}
      {geo.showSeparator && (
        <span className="absolute left-0 top-0 bottom-0 flex items-center justify-center -translate-x-1/2 w-[3px]">
          <span className="block rounded-full group-hover:scale-y-125 transition-all w-[2px] h-[80%] bg-[var(--overlay-chapter-marker)]" />
        </span>
      )}
    </button>
  );
});

export function ChapterMarkers({ chapters, durationSec, currentTimeSec, onSeek }: ChapterMarkersProps) {
  const { t } = useTranslation();

  // Stabilise onSeek so the memoised rows aren't invalidated when the parent
  // passes a freshly-allocated callback each render (it recreates the handler
  // on every tick). The ref always points at the latest handler.
  const onSeekRef = useRef(onSeek);
  onSeekRef.current = onSeek;
  const stableSeek = useCallback((timeSec: number) => onSeekRef.current(timeSec), []);

  const markers = useMemo<ChapterGeometry[]>(() => {
    if (durationSec <= 0) return [];
    return chapters
      .map((ch, i) => {
        const startPct = (ch.startTimeMs / 1000 / durationSec) * 100;
        const endPct = (ch.endTimeMs / 1000 / durationSec) * 100;
        return {
          startPct,
          widthPct: endPct - startPct,
          startMs: ch.startTimeMs,
          endMs: ch.endTimeMs,
          startSec: ch.startTimeMs / 1000,
          label: t("player.jumpToChapter", { chapter: ch.title, defaultValue: `Jump to chapter: ${ch.title}` }),
          title: ch.title,
          showSeparator: i > 0,
        };
      })
      .filter((g) => g.widthPct > 0);
  }, [chapters, durationSec, t]);

  if (durationSec <= 0 || markers.length === 0) return null;

  const currentMs = currentTimeSec * 1000;

  return (
    <>
      {markers.map((geo) => (
        <ChapterMarkerRow
          key={geo.startMs}
          geo={geo}
          isActive={currentMs >= geo.startMs && currentMs < geo.endMs}
          onSeek={stableSeek}
        />
      ))}
    </>
  );
}
