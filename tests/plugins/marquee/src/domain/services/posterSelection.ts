// Picks which posters make the wall: a mix of "recent" (the front slice of
// what the host returned, assumed newest-first) and a seeded random sample
// of the rest, so the wall reads as "your library", not a fixed top-N. Pure
// and deterministic given the same items/count/seed, so the roulette-worthy
// "wow, it's my movies" moment is reproducible in tests.

import type { MediaSummary } from "../model/media";
import { mulberry32, shuffleInPlace } from "./prng";

export function selectWallPosters(items: MediaSummary[], count: number, seed: number): MediaSummary[] {
  const wanted = Math.max(0, Math.floor(count));
  if (wanted === 0) return [];

  const seen = new Set<string>();
  const withPoster: MediaSummary[] = [];
  for (const item of items) {
    if (!item || !item.id || !item.posterUrl) continue;
    if (seen.has(item.id)) continue;
    seen.add(item.id);
    withPoster.push(item);
  }

  const rng = mulberry32(seed);
  if (withPoster.length <= wanted) {
    return shuffleInPlace(withPoster.slice(), rng);
  }

  const frontCount = Math.ceil(wanted / 2);
  const front = withPoster.slice(0, frontCount);
  const pool = withPoster.slice(frontCount);
  shuffleInPlace(pool, rng);
  const randomPart = pool.slice(0, wanted - front.length);

  return shuffleInPlace(front.concat(randomPart), rng);
}
