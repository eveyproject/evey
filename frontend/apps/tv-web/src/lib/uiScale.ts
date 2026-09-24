import { DEFAULT_UI_SCALE, effectiveUiScale } from "@evey/client";

// The Android television lays out in dp on a 960 dp wide screen, scaled by the
// interface size. Here 1rem is 10 of those dp, computed from the real width,
// so the sheet copies the Android values and a 4K panel reads like a 1080p one.
const SCALE_KEY = "evey-ui-scale";
const DP_PER_SCREEN = 960;

export function storedUiScale(): number {
  try {
    const raw = localStorage.getItem(SCALE_KEY);
    return effectiveUiScale(raw ? Number(raw) : null);
  } catch {
    return DEFAULT_UI_SCALE;
  }
}

let current = DEFAULT_UI_SCALE;
let listening = false;

function paint(): void {
  const dp = (window.innerWidth / DP_PER_SCREEN) * (current / 100);
  document.documentElement.style.fontSize = `${dp * 10}px`;
}

export function applyUiScale(percent: number): void {
  current = effectiveUiScale(percent);
  paint();
  if (!listening) {
    listening = true;
    window.addEventListener("resize", paint);
  }
  try {
    localStorage.setItem(SCALE_KEY, String(current));
  } catch {
    /* private window: the choice just won't stick */
  }
}

/** Screen pixels per dp right now. */
export function dpPx(): number {
  const rem = parseFloat(document.documentElement.style.fontSize);
  return rem > 0 ? rem / 10 : 1;
}
