// Picks a small, deterministic sample of titles to represent a cluster (the
// hover tooltip's "a few titles", and the item a click navigates to). Seeded
// so the same cluster always surfaces the same picks across reloads - a
// "constellation" should feel stable, not re-roll on every page view.

import type { CinephileItem } from "../model/media";
import { mulberry32, shuffleInPlace } from "./prng";

export function pickRepresentativeItems(items: CinephileItem[], count: number, seed: number): CinephileItem[] {
  const wanted = Math.max(0, Math.floor(count));
  if (wanted === 0 || items.length === 0) return [];
  const rng = mulberry32(seed);
  const pool = shuffleInPlace(items.slice(), rng);
  return pool.slice(0, Math.min(wanted, pool.length));
}
