import { describe, it, expect } from "vitest";
import { buildTimeline } from "../src/domain/services/timeline";
import type { DecadeBucket } from "../src/domain/services/decadeAggregation";
import type { CinephileItem } from "../src/domain/model/media";

function bucket(decade: number, count: number): DecadeBucket {
  const items: CinephileItem[] = Array.from({ length: count }, (_, i) => ({
    id: `mi-${decade}-${i}`,
    title: `Title ${i}`,
    year: decade,
    type: "movie",
  }));
  return { decade, items };
}

describe("buildTimeline", () => {
  it("returns one bar per decade in the observed span, including gaps", () => {
    const bars = buildTimeline([bucket(1990, 3), bucket(2020, 1)]);
    expect(bars.map((b) => b.decade)).toEqual([1990, 2000, 2010, 2020]);
  });

  it("fills gap decades with count 0 and heightFraction 0", () => {
    const bars = buildTimeline([bucket(1990, 3), bucket(2020, 1)]);
    const gap = bars.find((b) => b.decade === 2000)!;
    expect(gap.count).toBe(0);
    expect(gap.heightFraction).toBe(0);
  });

  it("normalizes heightFraction relative to the tallest bar", () => {
    const bars = buildTimeline([bucket(1990, 2), bucket(2000, 10)]);
    const tall = bars.find((b) => b.decade === 2000)!;
    const short = bars.find((b) => b.decade === 1990)!;
    expect(tall.heightFraction).toBe(1);
    expect(short.heightFraction).toBeCloseTo(0.2, 5);
  });

  it("returns a single bar when there is only one decade", () => {
    const bars = buildTimeline([bucket(2010, 5)]);
    expect(bars).toEqual([{ decade: 2010, count: 5, heightFraction: 1 }]);
  });

  it("ignores empty buckets", () => {
    const bars = buildTimeline([bucket(1990, 0), bucket(2000, 4)]);
    expect(bars.map((b) => b.decade)).toEqual([2000]);
  });

  it("returns an empty array when there is nothing to show", () => {
    expect(buildTimeline([])).toEqual([]);
    expect(buildTimeline([bucket(1990, 0)])).toEqual([]);
  });
});
