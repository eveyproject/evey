import { Check, BookOpen } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { Chapter } from "@/hooks/useChapters";
import { formatTime } from "@evey/api";

export interface ChaptersPanelProps {
  chapters: Chapter[];
  currentTimeSec: number;
  onSeek: (timeSec: number) => void;
  open: boolean;
  onToggle: () => void;
}

export function ChaptersPanel({ chapters, currentTimeSec, onSeek, open, onToggle }: ChaptersPanelProps) {
  const { t } = useTranslation();
  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    onToggle();
  }

  const currentMs = currentTimeSec * 1000;

  return (
    <div className="relative">
      <button onClick={toggle} aria-label={t("player.controls.chapters")} aria-expanded={open} aria-haspopup="true" title={t("player.controls.chapters")} className="text-white hover:text-[var(--accent)] transition-colors-fast p-1">
        <BookOpen size={20}  aria-hidden="true" />
      </button>
      {open && (
        <div
          className="absolute bottom-full mb-2 right-0 w-72 rounded-[var(--radius-md)] shadow-xl overflow-hidden z-badge bg-[var(--surface)] border border-[var(--border)]"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="px-3 pt-2 pb-1 text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">
            {t("player.controls.chapters")}
          </div>
          <div className="max-h-72 overflow-y-auto">
            {chapters.map((ch, i) => {
              const startSec = ch.startTimeMs / 1000;
              const isActive = currentMs >= ch.startTimeMs && (i === chapters.length - 1 || currentMs < chapters[i + 1].startTimeMs);
              return (
                <button
                  key={i}
                  onClick={() => { onSeek(startSec); onToggle(); }}
                  aria-current={isActive || undefined}
                  className={`flex items-center gap-2 w-full px-4 py-2 text-sm hover:bg-[var(--surface-raised)] transition-colors-fast text-left ${isActive ? "text-[var(--accent)]" : "text-[var(--text)]"}`}
                >
                  <span className="w-12 flex-shrink-0 font-mono text-xs text-[var(--text-muted)]">
                    {formatTime(startSec)}
                  </span>
                  <span className="flex-1 truncate">{ch.title || t("media.chapter", { number: i + 1 })}</span>
                  {isActive && (
                    <Check size={12} className="flex-shrink-0" aria-hidden="true" />
                  )}
                </button>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
