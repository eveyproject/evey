// Sun "skin". The plugin ships one real SDO (Solar Dynamics Observatory)
// full-disc image (NASA, public domain): a clean, warm AIA 171 frame.
//
// An earlier build rotated a second AIA 193 frame in daily, but that image
// carries a large dark coronal hole that reads as a "moth-eaten" black blotch
// on the disc, breaking the consistent golden look. The rotation was dropped
// in favour of the single clean skin. The dayIndex / sunModelForDay API is
// kept intentionally generic so more clean frames can be rotated back in later
// (add entries to SUN_MODELS) without touching the renderer's daily-swap path.
//
// Pure domain: no assets, no DOM. Maps a timestamp to a model id; the render
// adapter owns the actual texture keyed by this id.

export interface SunModel {
  id: string;      // stable key, matches a bundled texture in the renderer
  channel: string; // SDO channel label (for docs / credits)
  label: string;   // human name
}

// Order defines the daily rotation (one skin per day, cycling every
// SUN_MODELS.length days). With a single entry the sun is stable every day.
export const SUN_MODELS: SunModel[] = [
  { id: "aia171", channel: "AIA 171", label: "Gold corona" },
];

// Whole days since the Unix epoch (UTC). Deterministic and timezone-stable:
// the skin flips at UTC midnight, the same for every viewer.
export function dayIndex(nowMs: number): number {
  return Math.floor(nowMs / 86_400_000);
}

export function sunModelForDay(nowMs: number): SunModel {
  const i = ((dayIndex(nowMs) % SUN_MODELS.length) + SUN_MODELS.length) % SUN_MODELS.length;
  return SUN_MODELS[i];
}
