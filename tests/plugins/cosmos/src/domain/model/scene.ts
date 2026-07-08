// Fully-computed scene data handed to the renderer. All astronomy (sidereal
// time, alt/az, stereographic projection) happens upstream in the
// application layer; the renderer only smooths and draws these plain numbers
// - it never touches Date.now() or trigonometry.

import type { PlanetName } from "../services/planetPositions";
import type { SkyMood } from "../services/skyPalette";
import type { PhaseKey } from "../services/moonPhase";

export interface ProjectedStar {
  x: number;
  y: number;
  visible: boolean;
  mag: number;
  color: [number, number, number];
}

export interface ProjectedLine {
  ax: number;
  ay: number;
  bx: number;
  by: number;
  visible: boolean;
}

export interface ProjectedBody {
  x: number;
  y: number;
  visible: boolean;
  altDeg: number;
  azDeg: number;
}

export interface ProjectedPlanet extends ProjectedBody {
  name: PlanetName;
}

export interface ProjectedMoon extends ProjectedBody {
  phaseFraction: number;
  waxing: boolean;
}

export interface TonightBanner {
  moonPhaseKey: PhaseKey;
  daysToFullMoon: number;
  planetName: PlanetName | null;
  planetCompass: string | null;
}

export interface ConstellationLabel {
  labelKey: string;
  x: number;
  y: number;
  visible: boolean;
}

export interface SkyScene {
  stars: ProjectedStar[];
  lines: ProjectedLine[];
  constellationLabels: ConstellationLabel[];
  moon: ProjectedMoon;
  planets: ProjectedPlanet[];
  sun: ProjectedBody;
  starVisibility: number; // 0 day .. 1 astronomical night
  twilightWarmth: number; // 0..1, peaks at sunset/sunrise
  mood: SkyMood;
  banner: TonightBanner;
}
