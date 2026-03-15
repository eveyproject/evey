import type { Chapter } from "@/hooks/useChapters";

interface ChapterMarkersProps {
  chapters: Chapter[];
  durationSec: number;
  onSeek: (timeSec: number) => void;
}

export function ChapterMarkers({ chapters, durationSec, onSeek }: ChapterMarkersProps) {
  if (durationSec <= 0 || chapters.length === 0) return null;

  return (
    <>
      {chapters.map((ch, i) => {
        const pct = (ch.startTimeMs / 1000 / durationSec) * 100;
        if (pct <= 0) return null;
        const startSec = ch.startTimeMs / 1000;
        return (
          <button
            key={i}
            type="button"
            aria-label={`Jump to chapter: ${ch.title || `Chapter ${i + 1}`}`}
            title={ch.title || `Chapter ${i + 1}`}
            onClick={(e) => {
              e.stopPropagation();
              onSeek(startSec);
            }}
            className="absolute top-1/2 -translate-y-1/2 -translate-x-1/2 w-3 h-5 flex items-center justify-center z-20 cursor-pointer"
            style={{ left: `${pct}%` }}
          >
            <span className="w-0.5 h-3 bg-white/70 rounded-full block group-hover:h-4 transition-all" />
          </button>
        );
      })}
    </>
  );
}
