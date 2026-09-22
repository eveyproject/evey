// The television interface scale (Settings > Interface size): the percentage
// the interface's density is scaled by. Each client owns the mechanism - the
// Android television scales at process start through its native module, a
// browser zooms its document - and this is the vocabulary they share.

// What a fresh install runs at. Must equal the plugin's defaultPercent in
// app.json - the native side falls back to it when nothing was ever stored.
export const DEFAULT_UI_SCALE = 80;

export const UI_SCALE_OPTIONS = [70, 80, 90, 100] as const;
export type UiScaleOption = (typeof UI_SCALE_OPTIONS)[number];

// The scale in effect for a stored value: null (never set) is the default;
// a stored value outside the picker (an old build, a hand edit) still shows
// as itself so the checkmark never lies.
export function effectiveUiScale(stored: number | null | undefined): number {
  if (typeof stored !== "number" || !Number.isFinite(stored) || stored <= 0) return DEFAULT_UI_SCALE;
  return Math.round(stored);
}

export function isUiScaleOption(value: number): value is UiScaleOption {
  return (UI_SCALE_OPTIONS as readonly number[]).includes(value);
}
