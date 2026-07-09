// Keeps only the items the galaxy can meaningfully place: a watchable kind
// (movie/series - episodes, seasons, tracks, photos, books add noise to a
// "cinephile" taste map), a sane year (cinema didn't exist before ~1888;
// reject obvious garbage rather than let it skew a decade bucket), a title
// and a stable id. Pure, no I/O, safe to call on whatever the bridge handed
// back.

import type { CinephileItem } from "../model/media";

const MIN_YEAR = 1870;
const MAX_YEAR = 2100;

export function filterWatchable(items: CinephileItem[]): CinephileItem[] {
  const seen = new Set<string>();
  const out: CinephileItem[] = [];
  for (const item of items) {
    if (!item || !item.id || !item.title) continue;
    if (item.type !== "movie" && item.type !== "series") continue;
    if (!item.year || item.year < MIN_YEAR || item.year > MAX_YEAR) continue;
    if (seen.has(item.id)) continue;
    seen.add(item.id);
    out.push(item);
  }
  return out;
}
