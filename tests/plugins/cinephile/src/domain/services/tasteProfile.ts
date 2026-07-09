// Computes the "your cinephile profile" summary: the small, personal payoff
// of the whole widget. Built only from fields the `public` projection
// actually exposes (year, type) - no rating, no genre, those aren't in
// PublicMediaItem, so the profile deliberately doesn't claim to know them.

import type { CinephileItem } from "../model/media";
import { aggregateByDecade } from "./decadeAggregation";

export interface TasteProfile {
  totalCount: number;
  dominantDecade: number | null;
  dominantDecadeShare: number; // 0..1 of totalCount
  movieCount: number;
  seriesCount: number;
  oldestYear: number | null;
  newestYear: number | null;
}

export function computeTasteProfile(items: CinephileItem[]): TasteProfile {
  let movieCount = 0;
  let seriesCount = 0;
  let oldestYear: number | null = null;
  let newestYear: number | null = null;

  for (const item of items) {
    if (item.type === "movie") movieCount++;
    else if (item.type === "series") seriesCount++;
    if (item.year) {
      if (oldestYear === null || item.year < oldestYear) oldestYear = item.year;
      if (newestYear === null || item.year > newestYear) newestYear = item.year;
    }
  }

  let dominantDecade: number | null = null;
  let dominantCount = 0;
  for (const bucket of aggregateByDecade(items)) {
    if (bucket.items.length > dominantCount) {
      dominantCount = bucket.items.length;
      dominantDecade = bucket.decade;
    }
  }

  const totalCount = items.length;
  return {
    totalCount,
    dominantDecade,
    dominantDecadeShare: totalCount > 0 ? dominantCount / totalCount : 0,
    movieCount,
    seriesCount,
    oldestYear,
    newestYear,
  };
}
