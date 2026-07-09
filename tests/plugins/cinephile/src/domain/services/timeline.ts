// Builds the horizontal decade timeline: one bar per decade from the oldest
// to the newest title in the library, INCLUDING gap decades (count 0) so the
// frise reads as a continuous span rather than a sparse, misleading strip.

import type { DecadeBucket } from "./decadeAggregation";

export interface TimelineBar {
  decade: number;
  count: number;
  heightFraction: number; // 0..1, relative to the tallest bar
}

export function buildTimeline(buckets: DecadeBucket[]): TimelineBar[] {
  const nonEmpty = buckets.filter((b) => b.items.length > 0);
  if (nonEmpty.length === 0) return [];

  const sorted = nonEmpty.slice().sort((a, b) => a.decade - b.decade);
  const minDecade = sorted[0].decade;
  const maxDecade = sorted[sorted.length - 1].decade;
  const countByDecade = new Map(sorted.map((b) => [b.decade, b.items.length]));
  const maxCount = Math.max(...sorted.map((b) => b.items.length));

  const bars: TimelineBar[] = [];
  for (let decade = minDecade; decade <= maxDecade; decade += 10) {
    const count = countByDecade.get(decade) ?? 0;
    bars.push({ decade, count, heightFraction: maxCount > 0 ? count / maxCount : 0 });
  }
  return bars;
}
