// Four named, premium color stories. Each cycles through its own anchors as
// time passes (see paletteDrift.ts for the interpolation). Names are kept
// untranslated across locales - stylized preset names, like a paint swatch,
// the same way weather-v2/cosmos never translate city names.

import type { PresetDef, PresetId, RGB } from "../model/scene";

export const PRESETS: Record<PresetId, PresetDef> = {
  // Dawn: indigo -> magenta -> amber -> soft rose, cool sliding into warm.
  aurore: {
    id: "aurore",
    anchors: [
      [0.24, 0.16, 0.52],
      [0.62, 0.18, 0.56],
      [0.86, 0.56, 0.22],
      [0.82, 0.36, 0.46],
    ],
    vignette: [0.30, 0.14, 0.10],
  },
  // Electric: cyan -> magenta -> violet -> deep blue, saturated club light.
  neon: {
    id: "neon",
    anchors: [
      [0.05, 0.82, 0.92],
      [0.92, 0.14, 0.72],
      [0.52, 0.16, 0.94],
      [0.14, 0.34, 0.96],
    ],
    vignette: [0.08, 0.02, 0.16],
  },
  // Embers: a slow smolder from maroon through red into amber.
  braise: {
    id: "braise",
    anchors: [
      [0.32, 0.05, 0.05],
      [0.74, 0.14, 0.08],
      [0.93, 0.46, 0.12],
      [0.86, 0.62, 0.22],
    ],
    vignette: [0.16, 0.03, 0.02],
  },
  // Depths: teal -> cobalt -> cyan -> seafoam - the caustics' natural home.
  ocean: {
    id: "ocean",
    anchors: [
      [0.02, 0.24, 0.34],
      [0.05, 0.24, 0.62],
      [0.10, 0.62, 0.78],
      [0.18, 0.72, 0.58],
    ],
    vignette: [0.02, 0.10, 0.14],
  },
};

export const PRESET_IDS = Object.keys(PRESETS) as PresetId[];
export const DEFAULT_PRESET: PresetId = "aurore";

export function isPresetId(v: unknown): v is PresetId {
  return v === "aurore" || v === "neon" || v === "braise" || v === "ocean";
}

export function presetOrDefault(v: unknown): PresetId {
  return isPresetId(v) ? v : DEFAULT_PRESET;
}

// Re-exported for callers that only need the color-anchor type.
export type { RGB };
