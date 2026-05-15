import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import type { Chapter } from "@/hooks/useChapters";
import { TrickplayPreview } from "./TrickplayPreview";
import { ChapterMarkers } from "./ChapterMarkers";
import { formatTime } from "./playerUtils";

export interface PlayerProgressBarProps {
  absoluteTime: number;
  effectiveDuration: number;
  bufferedEnd: number;
  transcodeOffset: number;
  chapters: Chapter[];
  mediaId?: string;
  mediaType?: string;
  onSeek: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onChapterSeek: (timeSec: number) => void;
}

export function PlayerProgressBar({
  absoluteTime,
  effectiveDuration,
  bufferedEnd,
  transcodeOffset,
  chapters,
  mediaId,
  mediaType,
  onSeek,
  onChapterSeek,
}: PlayerProgressBarProps) {
  const { t } = useTranslation();
  const seekBarRef = useRef<HTMLDivElement | null>(null);
  const [seekHoverPos, setSeekHoverPos] = useState<number | null>(null);
  const [seekHoverX, setSeekHoverX] = useState(0);
  // Local drag value so the thumb tracks the cursor smoothly during a
  // scrub. With `value={absoluteTime}` and a synchronous onSeek that
  // fires per drag tick, an expensive seek (transcode restart) would
  // tug the thumb back to the still-old absoluteTime mid-drag → the
  // user sees the timeline "snap back" and feels broken. Commit the
  // seek only on pointerup / touchend / keyup so the transcode restarts
  // once per scrub, not per pixel.
  const [dragValue, setDragValue] = useState<number | null>(null);
  const dragValueRef = useRef<number | null>(null);
  dragValueRef.current = dragValue;

  function handleSeekHover(e: React.MouseEvent) {
    const bar = seekBarRef.current;
    if (!bar || effectiveDuration <= 0) { setSeekHoverPos(null); return; }
    const rect = bar.getBoundingClientRect();
    const x = Math.max(0, Math.min(e.clientX - rect.left, rect.width));
    const ratio = x / rect.width;
    setSeekHoverPos(ratio * effectiveDuration);
    setSeekHoverX(x);
  }

  return (
    <div
      ref={seekBarRef}
      className="relative w-full h-5 flex items-center group cursor-pointer"
      onMouseMove={handleSeekHover}
      onMouseLeave={() => setSeekHoverPos(null)}
      data-controls
    >
      {seekHoverPos !== null && (() => {
        const hoverMs = seekHoverPos * 1000;
        const hoverChapter = chapters.find((ch, i) =>
          hoverMs >= ch.startTimeMs && (i === chapters.length - 1 || hoverMs < chapters[i + 1].startTimeMs)
        );
        return (
          <div
            className="absolute bottom-full mb-2 -translate-x-1/2 flex flex-col items-center pointer-events-none"
            style={{ left: seekHoverX }}
          >
            {mediaId && (
              <TrickplayPreview mediaId={mediaId} hoverTime={seekHoverPos} mediaType={mediaType} />
            )}
            <div
              className="px-2 py-1 rounded text-xs text-white mt-1 text-center bg-[var(--overlay-scrim)] max-w-[200px]"
            >
              {hoverChapter?.title && <div className="truncate">{hoverChapter.title}</div>}
              <div>{formatTime(seekHoverPos)}</div>
            </div>
          </div>
        );
      })()}
      {/* background track */}
      <div className="absolute left-0 right-0 h-1 rounded bg-white/20 group-hover:h-1.5 transition-all" />
      {/* buffered track */}
      <div
        className="absolute left-0 h-1 rounded bg-white/40 group-hover:h-1.5 transition-all"
        style={{ width: effectiveDuration > 0 ? `${((transcodeOffset + bufferedEnd) / effectiveDuration) * 100}%` : "0%" }}
      />
      {/* progress track */}
      <div
        className="absolute left-0 h-1 rounded group-hover:h-1.5 transition-all bg-[var(--accent)]"
        style={{ width: effectiveDuration > 0 ? `${(absoluteTime / effectiveDuration) * 100}%` : "0%" }}
      />
      <ChapterMarkers
        chapters={chapters}
        durationSec={effectiveDuration}
        currentTimeSec={absoluteTime}
        onSeek={onChapterSeek}
      />
      <input
        type="range"
        min={0}
        max={effectiveDuration || 0}
        step={0.5}
        value={dragValue ?? absoluteTime}
        onChange={(e) => setDragValue(Number(e.target.value))}
        onMouseUp={() => {
          const v = dragValueRef.current;
          if (v !== null) {
            onSeek({ target: { value: String(v) } } as React.ChangeEvent<HTMLInputElement>);
            setDragValue(null);
          }
        }}
        onTouchEnd={() => {
          const v = dragValueRef.current;
          if (v !== null) {
            onSeek({ target: { value: String(v) } } as React.ChangeEvent<HTMLInputElement>);
            setDragValue(null);
          }
        }}
        onKeyUp={() => {
          const v = dragValueRef.current;
          if (v !== null) {
            onSeek({ target: { value: String(v) } } as React.ChangeEvent<HTMLInputElement>);
            setDragValue(null);
          }
        }}
        className="absolute inset-0 w-full h-full opacity-0 cursor-pointer z-raised"
        aria-label={t("player.controls.seek")}
      />
    </div>
  );
}
