import { useRef, useState, useCallback } from "react";

interface UseTouchGesturesArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  positionRef: React.MutableRefObject<number>;
  knownDurationSecRef: React.MutableRefObject<number>;
  seekRef: React.MutableRefObject<(absoluteTimeSec: number) => void>;
  skipFwdRef: React.MutableRefObject<number>;
  skipBwdRef: React.MutableRefObject<number>;
  showActionFlash: (action: "play" | "pause") => void;
}

export function useTouchGestures({
  videoRef,
  positionRef,
  knownDurationSecRef,
  seekRef,
  skipFwdRef,
  skipBwdRef,
  showActionFlash,
}: UseTouchGesturesArgs) {
  const touchStartRef = useRef<{ x: number; y: number; time: number } | null>(null);
  const lastTapRef = useRef<{ x: number; time: number } | null>(null);
  const singleTapTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [seekFlash, setSeekFlash] = useState<"left" | "right" | null>(null);
  const seekFlashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const handleTouchStart = useCallback((e: React.TouchEvent) => {
    const t = e.touches[0];
    touchStartRef.current = { x: t.clientX, y: t.clientY, time: Date.now() };
  }, []);

  const handleTouchEnd = useCallback((e: React.TouchEvent) => {
    const start = touchStartRef.current;
    if (!start) return;
    touchStartRef.current = null;
    e.preventDefault();
    const t = e.changedTouches[0];
    const dx = t.clientX - start.x;
    const dy = t.clientY - start.y;
    const dt = Date.now() - start.time;

    const el = e.currentTarget as HTMLElement;
    const rect = el.getBoundingClientRect();
    const isRightSide = t.clientX > rect.width * 0.6;
    if (Math.abs(dy) > 30 && Math.abs(dy) > Math.abs(dx) && dt < 500) {
      if (isRightSide) {
        const video = videoRef.current;
        if (video) {
          video.volume = Math.max(0, Math.min(1, video.volume + (dy < 0 ? 0.1 : -0.1)));
        }
        return;
      }
    }

    const isSmallMove = Math.abs(dx) < 20 && Math.abs(dy) < 20;
    if (!isSmallMove || dt > 300) {
      lastTapRef.current = null;
      return;
    }
    const now = Date.now();
    const last = lastTapRef.current;
    if (last && now - last.time < 400) {
      if (singleTapTimer.current) { clearTimeout(singleTapTimer.current); singleTapTimer.current = null; }
      const isLeft = t.clientX < rect.width / 2;
      lastTapRef.current = null;
      const pos = positionRef.current;
      const video = videoRef.current;
      if (!video) return;
      const dur = knownDurationSecRef.current > 0 ? knownDurationSecRef.current : (video.duration && isFinite(video.duration)) ? video.duration : 0;
      const target = isLeft
        ? Math.max(0, pos - skipBwdRef.current)
        : Math.min(dur || pos + skipFwdRef.current, pos + skipFwdRef.current);
      seekRef.current(target);
      setSeekFlash(isLeft ? "left" : "right");
      if (seekFlashTimerRef.current) clearTimeout(seekFlashTimerRef.current);
      seekFlashTimerRef.current = setTimeout(() => setSeekFlash(null), 600);
    } else {
      lastTapRef.current = { x: t.clientX, time: now };
      if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
      singleTapTimer.current = setTimeout(() => {
        singleTapTimer.current = null;
        const video = videoRef.current;
        if (!video) return;
        if (video.paused) { video.play().catch(() => {}); showActionFlash("play"); }
        else { video.pause(); showActionFlash("pause"); }
      }, 300);
    }
  }, []);

  const handleWheel = useCallback((e: React.WheelEvent) => {
    const video = videoRef.current;
    if (!video) return;
    e.preventDefault();
    const delta = e.deltaY < 0 ? 0.05 : -0.05;
    video.volume = Math.min(1, Math.max(0, video.volume + delta));
  }, []);

  // Cleanup timers on unmount
  const cleanupTimers = useCallback(() => {
    if (seekFlashTimerRef.current) clearTimeout(seekFlashTimerRef.current);
    if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
  }, []);

  return {
    seekFlash,
    handleTouchStart,
    handleTouchEnd,
    handleWheel,
    cleanupTimers,
  };
}
