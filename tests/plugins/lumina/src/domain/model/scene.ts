// Core domain types for the lumina scene: named palette presets, the
// continuously-drifting color frame derived from them, and the render
// target the application layer pushes through the SceneRenderer port.
// Lumina has no external data source (no weather, no sky, no library data):
// it is a pure ambient light-art wallpaper, so the whole "scene" is just a
// palette + a couple of user-tunable knobs.

export type RGB = [number, number, number];

export type PresetId = "aurore" | "neon" | "braise" | "ocean";

export interface PresetDef {
  id: PresetId;
  // Anchor colors the palette drifts through, in order (looped). At least 3
  // so the drift always has somewhere new to go.
  anchors: RGB[];
  // Warm accent for the vignette tint. Kept fixed per preset (not drifted)
  // so the frame edges always read as one coherent "frame" around the veils.
  vignette: RGB;
}

// A single instant of the drifting palette, ready to hand to the shader as
// uniforms - all channels already in 0..1, no further mixing needed.
export interface PaletteFrame {
  veilA: RGB;
  veilB: RGB;
  veilC: RGB;
  caustic: RGB;
  vignette: RGB;
}

export interface RenderTarget {
  paletteId: PresetId;
  intensity: number; // 0..1, user setting
  speedMul: number; // user setting, drift/animation speed multiplier
}
