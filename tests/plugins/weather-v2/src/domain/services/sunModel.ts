// Daily sun "skin" rotation. The plugin ships 7 real SDO (Solar Dynamics
// Observatory) full-disc images (NASA, public domain), one per wavelength
// channel. A different one is shown each day, cycling every 7 days.
//
// Pure domain: no assets, no DOM. Maps a timestamp to a model id; the render
// adapter owns the actual textures keyed by these ids.

export interface SunModel {
  id: string;      // stable key, matches a bundled texture in the renderer
  channel: string; // SDO channel label (for docs / credits)
  label: string;   // human name
}

// Order defines the daily rotation (one skin per day, cycling every
// SUN_MODELS.length days). All are warm/golden "AIA 171-style" images so the
// widget keeps a consistent look while varying the surface detail.
export const SUN_MODELS: SunModel[] = [
  { id: "aia171", channel: "AIA 171", label: "Gold corona" },
  { id: "aia193", channel: "AIA 193", label: "Bronze corona" },
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
