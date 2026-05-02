// Module-level shared refs so CelestialBody and Clouds can talk to each
// other without a React context dance.
import type * as THREE from "three";

// Cloud FBO texture: written by Clouds, read by Sun shader for per-pixel
// occlusion.
export const cloudTexRef: { current: THREE.Texture | null } = { current: null };

// Moon screen-space position + intensity: written by CelestialBody, read
// by the cloud shader so dense puffs in front of the moon catch a soft
// halo of moonlight.
export const moonStateRef: {
  screenX: number;       // -1..1
  screenY: number;       // -1..1
  intensity: number;     // 0..1
} = { screenX: 0, screenY: 0, intensity: 0 };
