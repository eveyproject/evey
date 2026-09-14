import { useEffect, useState } from "react";

/** Fade-out length of the auto-hidden control bars (the --fade-duration they
 *  set); the display switch waits for it. */
export const CONTROLS_FADE_MS = 300;

/**
 * Two-phase hide for the auto-hidden overlays. `visible` false fades the bar
 * out (opacity, CSS); once the fade has run, the bar is PARKED: display:none,
 * out of layout. A bar that only faded kept its progress tracks and range
 * input reflowing on every timeupdate for the whole playback - measured
 * 2026-09-14 at ~34 reflows/s with the controls "hidden", and 11 missed
 * video-frame callbacks a minute against 0 once parked.
 *
 * Done here rather than with a CSS `display` transition: Firefox 155 runs
 * the fade-in from @starting-style but drops the fade-out when the same
 * style change lands on display:none, even with transition-behavior:
 * allow-discrete (measured on the built bundle - opacity went 1 → 0 with no
 * intermediate frame). A timer the length of the fade is what every browser
 * honours. Reveal un-parks at once so the fade-in has a box to run on.
 */
export function useFadeParked(visible: boolean, fadeMs = CONTROLS_FADE_MS): boolean {
  const [parked, setParked] = useState(!visible);
  useEffect(() => {
    if (visible) {
      setParked(false);
      return;
    }
    const t = setTimeout(() => setParked(true), fadeMs);
    return () => clearTimeout(t);
  }, [visible, fadeMs]);
  return parked && !visible;
}

/** Class list for one auto-hidden overlay: the fade state plus the parked
 *  display switch. */
export function fadeClasses(visible: boolean, parked: boolean): string {
  if (visible) return "fade-shown";
  return parked ? "fade-hidden fade-parked" : "fade-hidden";
}
