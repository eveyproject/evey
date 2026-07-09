// Buckets watchable items by decade (the only reliable clustering axis the
// `public` projection exposes - see media.ts). Items must already be filtered
// (filterWatchable) so every item here has a valid year.

import type { CinephileItem } from "../model/media";

export interface DecadeBucket {
  decade: number; // e.g. 1990 for the 1990-1999 span
  items: CinephileItem[];
}

export function decadeOf(year: number): number {
  return Math.floor(year / 10) * 10;
}

// Language-neutral label for a decade span (e.g. "1990-99"). Deliberately
// not "1990s": the English plural doesn't translate cleanly across the
// plugin's fr/es/de locales, a bare numeric range does.
export function formatDecadeRange(decade: number): string {
  const endDigits = ((decade + 9) % 100).toString().padStart(2, "0");
  return `${decade}-${endDigits}`;
}

export function aggregateByDecade(items: CinephileItem[]): DecadeBucket[] {
  const map = new Map<number, CinephileItem[]>();
  for (const item of items) {
    if (!item.year) continue;
    const decade = decadeOf(item.year);
    const bucket = map.get(decade);
    if (bucket) bucket.push(item);
    else map.set(decade, [item]);
  }
  return Array.from(map.entries())
    .map(([decade, bucketItems]) => ({ decade, items: bucketItems }))
    .sort((a, b) => a.decade - b.decade);
}
