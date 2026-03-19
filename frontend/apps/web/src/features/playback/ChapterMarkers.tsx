import { useTranslation } from "react-i18next";
import type { Chapter } from "@/hooks/useChapters";

interface ChapterMarkersProps {
  chapters: Chapter[];
  durationSec: number;
  onSeek: (timeSec: number) => void;
}

export function ChapterMarkers({ chapters, durationSec, onSeek }: ChapterMarkersProps) {
  const { t } = useTranslation("media");
  if (durationSec <= 0 || chapters.length === 0) return null;

  return (
    <>
      {chapters.map((ch, i) => {
        const pct = (ch.startTimeMs / 1000 / durationSec) * 100;
        if (pct <= 0) return null;
        const startSec = ch.startTimeMs / 1000;
        const label = ch.title || t("chapter", { number: i + 1 });
        return (
          <button
            key={i}
            type="button"
            aria-label={label}
            title={label}
            onClick={(e) => {
              e.stopPropagation();
              onSeek(startSec);
            }}
            className="absolute top-0 bottom-0 -translate-x-1/2 flex items-center justify-center z-30 cursor-pointer"
            style={{ left: `${pct}%`, width: 12 }}
          >
            <span
              className="rounded-full block transition-all group-hover:scale-y-125"
              style={{ width: 3, height: "60%", backgroundColor: "rgba(255,255,255,0.85)" }}
            />
          </button>
        );
      })}
    </>
  );
}
