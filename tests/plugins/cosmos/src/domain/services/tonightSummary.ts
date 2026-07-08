// Builds the structured data behind the "Tonight" info banner
// ("Jupiter visible SE - Full moon in 3d"). Pure function: the UI layer
// turns this into a localized sentence (see i18n/labels.ts + ui/overlay.ts).

import { normalizeDeg } from "./mathUtils";
import type { PlanetName } from "./planetPositions";
import { daysToFullMoon, type PhaseKey } from "./moonPhase";

const COMPASS_POINTS = ["N", "NE", "E", "SE", "S", "SW", "W", "NW"] as const;
export type CompassPoint = (typeof COMPASS_POINTS)[number];

export function azToCompass(azDeg: number): CompassPoint {
  const idx = Math.round(normalizeDeg(azDeg) / 45) % 8;
  return COMPASS_POINTS[idx];
}

export interface VisiblePlanet {
  name: PlanetName;
  altDeg: number;
  azDeg: number;
}

export interface TonightSummary {
  moonPhaseKey: PhaseKey;
  daysToFullMoon: number;
  brightestVisiblePlanet: { name: PlanetName; compass: CompassPoint } | null;
}

export function buildTonightSummary(
  moonPhaseKey: PhaseKey,
  elongationDeg: number,
  planets: VisiblePlanet[],
): TonightSummary {
  const visible = planets.filter((p) => p.altDeg > 0);
  visible.sort((a, b) => b.altDeg - a.altDeg);
  const top = visible[0];

  return {
    moonPhaseKey,
    daysToFullMoon: daysToFullMoon(elongationDeg),
    brightestVisiblePlanet: top ? { name: top.name, compass: azToCompass(top.azDeg) } : null,
  };
}
