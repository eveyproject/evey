// Deterministic "dominant color" stand-in for a poster that hasn't finished
// loading yet: a warm/cool cinema-toned gradient keyed off the media id, so
// the wall never shows a flat gray hole while textures stream in.

import { hashString } from "./prng";

const PALETTE: ReadonlyArray<readonly [number, number, number]> = [
  [0.55, 0.18, 0.16], // marquee red
  [0.16, 0.22, 0.42], // deep blue
  [0.3, 0.22, 0.1], // amber brown
  [0.14, 0.32, 0.28], // teal
  [0.35, 0.14, 0.3], // magenta
  [0.22, 0.22, 0.22], // neutral charcoal
  [0.45, 0.3, 0.1], // gold
  [0.12, 0.14, 0.24], // navy
];

export function placeholderColorForId(id: string): readonly [number, number, number] {
  const h = hashString(id ?? "");
  return PALETTE[h % PALETTE.length];
}
