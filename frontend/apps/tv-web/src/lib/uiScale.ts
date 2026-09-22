import { DEFAULT_UI_SCALE, effectiveUiScale } from "@evey/client";

// A browser scales by zooming its document: the stylesheet is in viewport
// units, so a font-size multiplier would change nothing. The vocabulary (the
// options, the default, the reader) is shared with the Android television.
const SCALE_KEY = "evey-ui-scale";

export function storedUiScale(): number {
  try {
    const raw = localStorage.getItem(SCALE_KEY);
    return effectiveUiScale(raw ? Number(raw) : null);
  } catch {
    return DEFAULT_UI_SCALE;
  }
}

export function applyUiScale(percent: number): void {
  const scale = effectiveUiScale(percent);
  // `zoom` is what the television browsers (Chromium) honour for a whole
  // document; a transform would leave the layout at its old size.
  (document.documentElement.style as CSSStyleDeclaration & { zoom?: string }).zoom = `${scale}%`;
  try {
    localStorage.setItem(SCALE_KEY, String(scale));
  } catch {
    /* private window: the choice just won't stick */
  }
}
