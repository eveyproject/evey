import { useRef, useState, useCallback } from "react";

interface UseTouchGesturesArgs {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  positionRef: React.MutableRefObject<number>;
  knownDurationSecRef: React.MutableRefObject<number>;
  seekRef: React.MutableRefObject<(absoluteTimeSec: number) => void>;
  skipFwdRef: React.MutableRefObject<number>;
  skipBwdRef: React.MutableRefObject<number>;
  onToggleFullscreen: () => void;
  onSingleTap: () => void;
}

const BRIGHTNESS_KEY = "evey-player-brightness";
const SWIPE_THRESHOLD = 15;
const MIN_BRIGHTNESS = 0.2;
const MAX_BRIGHTNESS = 1.5;
const SENSITIVITY = 0.004; // per pixel of vertical movement

function loadBrightness(): number {
  try {
    const v = localStorage.getItem(BRIGHTNESS_KEY);
    if (v) return Math.max(MIN_BRIGHTNESS, Math.min(MAX_BRIGHTNESS, Number(v)));
  } catch (err) { console.warn("localStorage write failed", err); }
  return 1;
}

export function useTouchGestures({
  videoRef,
  positionRef,
  knownDurationSecRef,
  seekRef,
  skipFwdRef,
  skipBwdRef,
  onToggleFullscreen,
  onSingleTap,
}: UseTouchGesturesArgs) {
  const touchStartRef = useRef<{ x: number; y: number; time: number } | null>(null);
  const lastTapRef = useRef<{ x: number; time: number } | null>(null);
  const singleTapTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [seekFlash, setSeekFlash] = useState<"left" | "right" | null>(null);
  const seekFlashTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Swipe gesture state
  const swipeActiveRef = useRef<"volume" | "brightness" | null>(null);
  const swipeStartVolumeRef = useRef(1);
  const swipeStartBrightnessRef = useRef(1);
  const [brightness, setBrightness] = useState(loadBrightness);
  const brightnessRef = useRef(loadBrightness());
  const [swipeIndicator, setSwipeIndicator] = useState<{ type: "volume" | "brightness"; value: number } | null>(null);

  const handleTouchStart = useCallback((e: React.TouchEvent) => {
    const target = e.target as HTMLElement;
    if (target.closest("button, input, [data-controls], a")) {
      touchStartRef.current = null;
      return;
    }
    const t = e.touches[0];
    touchStartRef.current = { x: t.clientX, y: t.clientY, time: Date.now() };
    swipeActiveRef.current = null;
    const video = videoRef.current;
    swipeStartVolumeRef.current = video?.volume ?? 1;
    swipeStartBrightnessRef.current = brightnessRef.current;
  }, [videoRef]);

  const handleTouchMove = useCallback((e: React.TouchEvent) => {
    const start = touchStartRef.current;
    if (!start) return;
    const t = e.touches[0];
    const dx = t.clientX - start.x;
    const dy = start.y - t.clientY; // positive = swipe up

    // Determine swipe direction on first significant movement
    if (!swipeActiveRef.current) {
      if (Math.abs(dy) < SWIPE_THRESHOLD) return;
      if (Math.abs(dx) > Math.abs(dy)) return; // horizontal swipe, ignore

      const el = e.currentTarget as HTMLElement;
      const rect = el.getBoundingClientRect();
      const relX = (t.clientX - rect.left) / rect.width;

      if (relX > 0.6) {
        swipeActiveRef.current = "volume";
      } else if (relX < 0.4) {
        swipeActiveRef.current = "brightness";
      } else {
        return; // middle zone - no gesture
      }
    }

    e.preventDefault();

    if (swipeActiveRef.current === "volume") {
      const video = videoRef.current;
      if (!video) return;
      const newVol = Math.max(0, Math.min(1, swipeStartVolumeRef.current + dy * SENSITIVITY));
      video.volume = newVol;
      // Same intent as the volume slider: a swipe up past 0 clears the
      // muted-autoplay flag. Otherwise the indicator shows the new level
      // while the user still hears nothing.
      if (newVol > 0 && video.muted) video.muted = false;
      setSwipeIndicator({ type: "volume", value: newVol });
    } else if (swipeActiveRef.current === "brightness") {
      const newBri = Math.max(MIN_BRIGHTNESS, Math.min(MAX_BRIGHTNESS, swipeStartBrightnessRef.current + dy * SENSITIVITY));
      brightnessRef.current = newBri;
      setBrightness(newBri);
      setSwipeIndicator({ type: "brightness", value: newBri });
    }
  }, [videoRef]);

  const handleTouchEnd = useCallback((e: React.TouchEvent) => {
    const start = touchStartRef.current;
    if (!start) return;
    touchStartRef.current = null;

    // If a swipe gesture was active, finalize and dismiss indicator
    if (swipeActiveRef.current) {
      if (swipeActiveRef.current === "brightness") {
        try { localStorage.setItem(BRIGHTNESS_KEY, String(brightnessRef.current)); } catch (err) { console.warn("localStorage write failed", err); }
      }
      swipeActiveRef.current = null;
      setSwipeIndicator(null);
      e.preventDefault();
      return;
    }

    // Don't intercept taps on controls (buttons, inputs, sliders)
    const target = e.target as HTMLElement;
    if (target.closest("button, input, [data-controls], a")) return;

    e.preventDefault();
    const t = e.changedTouches[0];
    const dx = t.clientX - start.x;
    const dy = t.clientY - start.y;

    // Tap detection (small movement)
    const isSmallMove = Math.abs(dx) < 20 && Math.abs(dy) < 20;
    const dt = Date.now() - start.time;
    if (!isSmallMove || dt > 300) {
      lastTapRef.current = null;
      return;
    }

    const el = e.currentTarget as HTMLElement;
    const rect = el.getBoundingClientRect();
    const now = Date.now();
    const last = lastTapRef.current;
    if (last && now - last.time < 400) {
      // Double tap
      if (singleTapTimer.current) { clearTimeout(singleTapTimer.current); singleTapTimer.current = null; }
      lastTapRef.current = null;
      const relX = (t.clientX - rect.left) / rect.width;
      if (relX > 0.35 && relX < 0.65) {
        // Center zone → fullscreen
        onToggleFullscreen();
      } else {
        // Side zones → seek
        const isLeft = relX < 0.35;
        const pos = positionRef.current;
        const video = videoRef.current;
        if (!video) return;
        const dur = knownDurationSecRef.current > 0 ? knownDurationSecRef.current : (video.duration && isFinite(video.duration)) ? video.duration : 0;
        const fwdTarget = pos + skipFwdRef.current;
        const target = isLeft
          ? Math.max(0, pos - skipBwdRef.current)
          // Clamp forward to duration only when known; the old `dur || ...`
          // fallback never clamped (and used a local-ish ceiling).
          : (dur > 0 ? Math.min(dur, fwdTarget) : fwdTarget);
        seekRef.current(target);
        setSeekFlash(isLeft ? "left" : "right");
        if (seekFlashTimerRef.current) clearTimeout(seekFlashTimerRef.current);
        seekFlashTimerRef.current = setTimeout(() => setSeekFlash(null), 600);
      }
    } else {
      // Single tap → toggle controls visibility
      lastTapRef.current = { x: t.clientX, time: now };
      if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
      singleTapTimer.current = setTimeout(() => {
        singleTapTimer.current = null;
        onSingleTap();
      }, 300);
    }
  }, [videoRef, positionRef, knownDurationSecRef, seekRef, skipFwdRef, skipBwdRef, onToggleFullscreen, onSingleTap]);


  const setBrightnessValue = useCallback((value: number) => {
    const clamped = Math.max(MIN_BRIGHTNESS, Math.min(MAX_BRIGHTNESS, value));
    brightnessRef.current = clamped;
    setBrightness(clamped);
    try { localStorage.setItem(BRIGHTNESS_KEY, String(clamped)); } catch (err) { console.warn("localStorage write failed", err); }
  }, []);

  const cleanupTimers = useCallback(() => {
    if (seekFlashTimerRef.current) clearTimeout(seekFlashTimerRef.current);
    if (singleTapTimer.current) clearTimeout(singleTapTimer.current);
  }, []);

  return {
    seekFlash,
    brightness,
    setBrightnessValue,
    swipeIndicator,
    handleTouchStart,
    handleTouchMove,
    handleTouchEnd,
    cleanupTimers,
  };
}
