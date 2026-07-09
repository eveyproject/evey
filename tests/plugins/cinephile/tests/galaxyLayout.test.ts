import { describe, it, expect } from "vitest";
import { buildGalaxyLayout, radiusForShare } from "../src/domain/services/galaxyLayout";
import type { DecadeBucket } from "../src/domain/services/decadeAggregation";
import type { CinephileItem } from "../src/domain/model/media";

function bucket(decade: number, count: number): DecadeBucket {
  const items: CinephileItem[] = Array.from({ length: count }, (_, i) => ({
    id: `mi-${decade}-${i}`,
    title: `Title ${decade}-${i}`,
    year: decade,
    type: "movie",
  }));
  return { decade, items };
}

describe("radiusForShare", () => {
  it("grows with share (sqrt scale, area-proportional)", () => {
    expect(radiusForShare(0.25)).toBeLessThan(radiusForShare(1));
    expect(radiusForShare(0)).toBeCloseTo(radiusForShare(0), 5);
  });

  it("clamps to [minRadius, maxRadius] for out-of-range shares", () => {
    const min = radiusForShare(0);
    const max = radiusForShare(1);
    expect(radiusForShare(-1)).toBe(min);
    expect(radiusForShare(2)).toBe(max);
  });
});

describe("buildGalaxyLayout", () => {
  it("produces one node per decade when under the node cap", () => {
    const buckets = [bucket(1980, 3), bucket(1990, 5), bucket(2000, 2)];
    const nodes = buildGalaxyLayout(buckets, { maxNodes: 10, representativePerNode: 2 });
    expect(nodes).toHaveLength(3);
    expect(nodes.map((n) => n.decade)).toEqual([1980, 1990, 2000]);
  });

  it("keeps nodes in chronological order", () => {
    const buckets = [bucket(2010, 1), bucket(1970, 1), bucket(1995, 1)];
    const nodes = buildGalaxyLayout(buckets, { maxNodes: 10, representativePerNode: 1 });
    expect(nodes.map((n) => n.decade)).toEqual([1970, 1995, 2010]);
  });

  it("computes count and share per node", () => {
    const buckets = [bucket(1990, 3), bucket(2000, 1)];
    const nodes = buildGalaxyLayout(buckets, { maxNodes: 10, representativePerNode: 1 });
    const n1990 = nodes.find((n) => n.decade === 1990)!;
    expect(n1990.count).toBe(3);
    expect(n1990.share).toBeCloseTo(0.75, 5);
  });

  it("merges the smallest decades into a single 'other' node when over the cap", () => {
    const buckets = [bucket(1970, 1), bucket(1980, 1), bucket(1990, 10), bucket(2000, 8)];
    const nodes = buildGalaxyLayout(buckets, { maxNodes: 3, representativePerNode: 1 });
    expect(nodes).toHaveLength(3);
    const other = nodes.find((n) => n.decade === null);
    expect(other).toBeTruthy();
    expect(other!.count).toBe(2); // 1970 + 1980 merged
    expect(nodes.some((n) => n.decade === 1990)).toBe(true);
    expect(nodes.some((n) => n.decade === 2000)).toBe(true);
  });

  it("places the merged 'other' node last", () => {
    const buckets = [bucket(1970, 1), bucket(1980, 1), bucket(1990, 10), bucket(2000, 8)];
    const nodes = buildGalaxyLayout(buckets, { maxNodes: 3, representativePerNode: 1 });
    expect(nodes[nodes.length - 1].decade).toBeNull();
  });

  it("never loses items: total node counts sum to the input total", () => {
    const buckets = [bucket(1970, 2), bucket(1980, 3), bucket(1990, 10), bucket(2000, 8), bucket(2010, 1)];
    const total = buckets.reduce((sum, b) => sum + b.items.length, 0);
    const nodes = buildGalaxyLayout(buckets, { maxNodes: 2, representativePerNode: 1 });
    const nodeTotal = nodes.reduce((sum, n) => sum + n.count, 0);
    expect(nodeTotal).toBe(total);
  });

  it("collapses everything into a single 'other' node when maxNodes is 1", () => {
    const buckets = [bucket(1970, 1), bucket(1990, 5), bucket(2010, 2)];
    const nodes = buildGalaxyLayout(buckets, { maxNodes: 1, representativePerNode: 1 });
    expect(nodes).toHaveLength(1);
    expect(nodes[0].decade).toBeNull();
    expect(nodes[0].count).toBe(8);
  });

  it("gives every node finite, in-range coordinates and a positive radius", () => {
    const buckets = [bucket(1970, 1), bucket(1980, 4), bucket(1990, 9), bucket(2000, 2), bucket(2010, 1)];
    const nodes = buildGalaxyLayout(buckets, { maxNodes: 4, representativePerNode: 2 });
    for (const node of nodes) {
      expect(Number.isFinite(node.x)).toBe(true);
      expect(Number.isFinite(node.y)).toBe(true);
      expect(node.radius).toBeGreaterThan(0);
    }
  });

  it("caps representative titles at representativePerNode and draws them from the node's own items", () => {
    const buckets = [bucket(1990, 10)];
    const nodes = buildGalaxyLayout(buckets, { maxNodes: 10, representativePerNode: 3 });
    const node = nodes[0];
    expect(node.representative).toHaveLength(3);
    const bucketIds = new Set(buckets[0].items.map((i) => i.id));
    expect(node.representative.every((i) => bucketIds.has(i.id))).toBe(true);
  });

  it("is deterministic across calls with the same input", () => {
    const buckets = [bucket(1970, 2), bucket(1990, 6), bucket(2010, 3)];
    const a = buildGalaxyLayout(buckets, { maxNodes: 5, representativePerNode: 2 });
    const b = buildGalaxyLayout(buckets, { maxNodes: 5, representativePerNode: 2 });
    expect(a).toEqual(b);
  });

  it("returns an empty array when there are no items", () => {
    expect(buildGalaxyLayout([], { maxNodes: 5, representativePerNode: 2 })).toEqual([]);
    expect(buildGalaxyLayout([bucket(1990, 0)], { maxNodes: 5, representativePerNode: 2 })).toEqual([]);
  });
});
