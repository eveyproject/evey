import { useEffect, useState } from "react";
import type { Chapter } from "@/hooks/useChapters";
import { usePlayerTime } from "./PlayerContext";
import { PlayerProgressBar } from "./PlayerProgressBar";
import { ChaptersPanel } from "./PlayerChaptersPanel";
import { formatTime } from "@evey/api";

// These leaf components subscribe to PlayerTimeContext (the ~4×/sec position
// tick) so that the seek bar, time label and chapter highlight re-render
// without dragging the whole PlayerControls tree (buttons, panels) with them.
// When casting, the position comes from the device (a prop, ~1×/sec) instead of
// the local <video>; `castTimeSec` overrides the context value in that case.

interface LiveProgressBarProps {
  castTimeSec?: number;
  effectiveDuration: number;
  transcodeOffset: number;
  chapters: Chapter[];
  mediaId?: string;
  mediaType?: string;
  onSeek: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onChapterSeek: (timeSec: number) => void;
}

export function LiveProgressBar({ castTimeSec, ...rest }: LiveProgressBarProps) {
  const time = usePlayerTime();
  const absoluteTime = castTimeSec ?? time.absoluteTime;
  return (
    <PlayerProgressBar
      absoluteTime={absoluteTime}
      bufferedEnd={time.bufferedEnd}
      {...rest}
    />
  );
}

interface LiveTimeLabelProps {
  castTimeSec?: number;
  effectiveDuration: number;
}

export function LiveTimeLabel({ castTimeSec, effectiveDuration }: LiveTimeLabelProps) {
  const time = usePlayerTime();
  const absoluteTime = castTimeSec ?? time.absoluteTime;
  return (
    <span className="text-[var(--overlay-text)] text-xs tabular-nums">
      {formatTime(absoluteTime)} / {formatTime(effectiveDuration)}
    </span>
  );
}

interface LiveChaptersPanelProps {
  castTimeSec?: number;
  chapters: Chapter[];
  onSeek: (timeSec: number) => void;
  open: boolean;
  onToggle: () => void;
}

export function LiveChaptersPanel({ castTimeSec, ...rest }: LiveChaptersPanelProps) {
  const time = usePlayerTime();
  const currentTimeSec = castTimeSec ?? time.absoluteTime;
  return <ChaptersPanel currentTimeSec={currentTimeSec} {...rest} />;
}

// useActiveChapterIndex tracks the chapter the playhead is in WITHOUT
// re-rendering its host every tick: it listens to the <video> timeupdate event
// (or the cast position prop) and only setState when the index actually
// changes - i.e. on a chapter boundary crossing, which is rare. This is what
// lets PlayerControls keep the chapter nav buttons inline yet stop re-rendering
// 4×/sec.
export function useActiveChapterIndex(
  chapters: Chapter[],
  videoRef: React.RefObject<HTMLVideoElement | null>,
  transcodeOffset: number,
  castTimeSec?: number,
): number {
  const [index, setIndex] = useState(-1);
  useEffect(() => {
    const find = (absSec: number) => {
      const ms = absSec * 1000;
      const next = chapters.findIndex((ch) => ms >= ch.startTimeMs && ms < ch.endTimeMs);
      setIndex((prev) => (prev === next ? prev : next));
    };
    if (castTimeSec !== undefined) {
      find(castTimeSec);
      return;
    }
    const video = videoRef.current;
    if (!video) {
      setIndex(-1);
      return;
    }
    const onTime = () => find(transcodeOffset + video.currentTime);
    onTime();
    video.addEventListener("timeupdate", onTime);
    return () => video.removeEventListener("timeupdate", onTime);
  }, [chapters, videoRef, transcodeOffset, castTimeSec]);
  return index;
}
