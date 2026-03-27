import type { Chapter } from "@/hooks/useChapters";

interface ChapterMarkersProps {
  chapters: Chapter[];
  durationSec: number;
  currentTimeSec: number;
  onSeek: (timeSec: number) => void;
}

export function ChapterMarkers({ chapters, durationSec, currentTimeSec, onSeek }: ChapterMarkersProps) {
  if (durationSec <= 0 || chapters.length === 0) return null;

  const currentMs = currentTimeSec * 1000;

  return (
    <>
      {chapters.map((ch, i) => {
        const startPct = (ch.startTimeMs / 1000 / durationSec) * 100;
        const endPct = (ch.endTimeMs / 1000 / durationSec) * 100;
        const widthPct = endPct - startPct;
        if (widthPct <= 0) return null;

        const isActive = currentMs >= ch.startTimeMs && currentMs < ch.endTimeMs;

        return (
          <button
            key={i}
            type="button"
            aria-label={`Jump to chapter: ${ch.title}`}
            title={ch.title}
            onClick={(e) => {
              e.stopPropagation();
              onSeek(ch.startTimeMs / 1000);
            }}
            className="absolute top-0 bottom-0 flex items-center cursor-pointer z-float"
            style={{ left: `${startPct}%`, width: `${widthPct}%` }}
          >
            <span
              className="block w-full h-1 rounded group-hover:h-1.5 transition-all"
              style={{
                backgroundColor: isActive ? "var(--accent)" : "var(--overlay-chapter-marker)",
                opacity: isActive ? 0.5 : 0.3,
              }}
            />
            {/* separator line between chapters (skip first) */}
            {i > 0 && (
              <span
                className="absolute left-0 top-0 bottom-0 flex items-center justify-center -translate-x-1/2"
                style={{ width: 3 }}
              >
                <span
                  className="block rounded-full group-hover:scale-y-125 transition-all"
                  style={{ width: 2, height: "80%", backgroundColor: "var(--overlay-chapter-marker)" }}
                />
              </span>
            )}
          </button>
        );
      })}
    </>
  );
}
