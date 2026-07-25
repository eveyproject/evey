import { useCallback, useEffect, useRef, useState } from "react";

// Matches the "keep the bar up longer" ask (2026-07-25 feedback): the previous
// 3000ms hid the progress bar/buttons before a user could react to what they
// were looking at. Web's PlayerControls uses 3000ms with a keyboard-focus
// override; mobile has no keyboard focus signal, so instead we extend the base
// delay and additionally never arm the timer while paused or a sheet is open.
export const CONTROLS_HIDE_DELAY_MS = 4500;

interface UseControlsAutoHideOptions {
  /** Auto-hide must never fire while paused - a paused frame has nothing else
   * moving on screen, so hiding the controls leaves the user with no way back in. */
  isPlaying: boolean;
  /** Auto-hide must never fire while a track/settings/sync sheet is open, same as
   * web's PlayerControls pinning visible for settingsOpen/episodesOpen/chaptersOpen. */
  hasOpenPanel: boolean;
  delayMs?: number;
}

export function useControlsAutoHide({
  isPlaying,
  hasOpenPanel,
  delayMs = CONTROLS_HIDE_DELAY_MS,
}: UseControlsAutoHideOptions) {
  const [showControls, setShowControls] = useState(true);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearHideTimer = useCallback(() => {
    if (hideTimerRef.current) {
      clearTimeout(hideTimerRef.current);
      hideTimerRef.current = null;
    }
  }, []);

  // Every interaction (tap, seek, opening a panel) calls this to both reveal
  // the controls immediately and push the auto-hide back out to a full delay.
  const resetHideTimer = useCallback(() => {
    clearHideTimer();
    setShowControls(true);
    if (hasOpenPanel || !isPlaying) return;
    hideTimerRef.current = setTimeout(() => setShowControls(false), delayMs);
  }, [clearHideTimer, hasOpenPanel, isPlaying, delayMs]);

  // Re-run on every pause/panel transition, not just on mount: pausing or
  // opening a sheet must cancel any timer already in flight, and resuming or
  // closing must re-arm one from a full delay rather than leave it cancelled.
  useEffect(() => {
    resetHideTimer();
    return clearHideTimer;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hasOpenPanel, isPlaying, delayMs]);

  const toggleControls = useCallback(() => {
    if (showControls) {
      clearHideTimer();
      setShowControls(false);
    } else {
      resetHideTimer();
    }
  }, [showControls, resetHideTimer, clearHideTimer]);

  return { showControls, resetHideTimer, toggleControls };
}
