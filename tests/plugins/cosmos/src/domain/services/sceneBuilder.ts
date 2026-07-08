// Composes every astronomy service into one fully-projected SkyScene for a
// given instant and observer location. Pure function: no Date.now(), no DOM,
// no WebGL - the application layer calls this on a timer and hands the
// result straight to the renderer.

import { localSiderealTimeDeg } from "./siderealTime";
import { equatorialToHorizontal } from "./horizontal";
import { stereographicProject } from "./projection";
import { sunPosition } from "./sunPosition";
import { moonPosition, moonEclipticPosition } from "./moonPosition";
import { moonPhaseFromElongation } from "./moonPhase";
import { allPlanetPositions } from "./planetPositions";
import { bvToRgb } from "./starColor";
import { starVisibility, twilightWarmth, skyMood } from "./skyPalette";
import { buildTonightSummary } from "./tonightSummary";
import { BRIGHT_STARS } from "../data/brightStars";
import { CONSTELLATIONS } from "../data/constellations";
import type { SkyScene, ProjectedStar, ProjectedLine, ConstellationLabel } from "../model/scene";

export interface Observer {
  latDeg: number;
  lonDeg: number;
}

export function buildSkyScene(nowMs: number, observer: Observer): SkyScene {
  const lst = localSiderealTimeDeg(nowMs, observer.lonDeg);

  const project = (raHours: number, decDeg: number) => {
    const { altDeg, azDeg } = equatorialToHorizontal(raHours, decDeg, lst, observer.latDeg);
    const p = stereographicProject(altDeg, azDeg);
    return { ...p, altDeg, azDeg };
  };

  // Stars: project once, keep a lookup by id for the constellation lines.
  const projectedById = new Map<string, ProjectedStar & { altDeg: number; azDeg: number }>();
  const stars: ProjectedStar[] = BRIGHT_STARS.map((s) => {
    const p = project(s.raH, s.decDeg);
    const star: ProjectedStar & { altDeg: number; azDeg: number } = {
      x: p.x,
      y: p.y,
      visible: p.visible,
      mag: s.mag,
      color: bvToRgb(s.bv),
      altDeg: p.altDeg,
      azDeg: p.azDeg,
    };
    projectedById.set(s.id, star);
    return star;
  });

  const lines: ProjectedLine[] = CONSTELLATIONS.flatMap((con) =>
    con.segments.map(([aId, bId]) => {
      const a = projectedById.get(aId);
      const b = projectedById.get(bId);
      if (!a || !b) return { ax: 0, ay: 0, bx: 0, by: 0, visible: false };
      return { ax: a.x, ay: a.y, bx: b.x, by: b.y, visible: a.visible && b.visible };
    }),
  );

  const constellationLabels: ConstellationLabel[] = CONSTELLATIONS.map((con) => {
    const ids = new Set<string>();
    for (const [a, b] of con.segments) {
      ids.add(a);
      ids.add(b);
    }
    const members = [...ids].map((id) => projectedById.get(id)).filter((s): s is NonNullable<typeof s> => !!s);
    const visibleMembers = members.filter((s) => s.visible);
    const pool = visibleMembers.length > 0 ? visibleMembers : members;
    const x = pool.reduce((sum, s) => sum + s.x, 0) / (pool.length || 1);
    const y = pool.reduce((sum, s) => sum + s.y, 0) / (pool.length || 1);
    return { labelKey: con.labelKey, x, y, visible: visibleMembers.length > 0 };
  });

  const sun = sunPosition(nowMs);
  const sunP = project(sun.raHours, sun.decDeg);

  const moonEq = moonPosition(nowMs);
  const moonEcl = moonEclipticPosition(nowMs);
  const moonP = project(moonEq.raHours, moonEq.decDeg);
  const phase = moonPhaseFromElongation(moonEcl.lonDeg, sun.eclipticLonDeg);

  const planets = allPlanetPositions(nowMs).map((pl) => {
    const p = project(pl.raHours, pl.decDeg);
    return { x: p.x, y: p.y, visible: p.visible, altDeg: p.altDeg, azDeg: p.azDeg, name: pl.name };
  });

  const banner = buildTonightSummary(
    phase.phaseKey,
    phase.elongationDeg,
    planets.map((p) => ({ name: p.name, altDeg: p.altDeg, azDeg: p.azDeg })),
  );

  return {
    stars,
    lines,
    constellationLabels,
    moon: { x: moonP.x, y: moonP.y, visible: moonP.visible, altDeg: moonP.altDeg, azDeg: moonP.azDeg, phaseFraction: phase.illuminatedFraction, waxing: phase.waxing },
    planets,
    sun: { x: sunP.x, y: sunP.y, visible: sunP.visible, altDeg: sunP.altDeg, azDeg: sunP.azDeg },
    starVisibility: starVisibility(sunP.altDeg),
    twilightWarmth: twilightWarmth(sunP.altDeg),
    mood: skyMood(sunP.altDeg),
    banner: {
      moonPhaseKey: banner.moonPhaseKey,
      daysToFullMoon: banner.daysToFullMoon,
      planetName: banner.brightestVisiblePlanet?.name ?? null,
      planetCompass: banner.brightestVisiblePlanet?.compass ?? null,
    },
  };
}
