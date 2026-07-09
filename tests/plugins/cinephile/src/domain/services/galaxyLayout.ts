// Radial layout of the "taste galaxy": one node per decade, evenly spaced
// around a circle in chronological order, sized by a sqrt(share) scale (area
// proportional to count, the standard bubble-chart convention so relative
// size actually reads correctly). When the device's quality tier caps the
// node count below the number of decades present, the smallest decades are
// merged into a single "other" node rather than dropped - every title stays
// represented somewhere. Pure and fully deterministic: positions/jitter are
// derived from the node's own id (hashString), never from wall-clock time or
// Math.random(), so the same library always draws the same constellation.

import type { CinephileItem } from "../model/media";
import type { DecadeBucket } from "./decadeAggregation";
import { hashString } from "./prng";
import { pickRepresentativeItems } from "./representativeSelection";

export interface GalaxyNode {
  id: string; // `decade-${decade}` or "other"
  decade: number | null; // null for the merged "other" node
  count: number;
  share: number; // 0..1 of all watchable items
  x: number; // normalized, roughly [-0.85, 0.85]
  y: number;
  radius: number; // normalized node radius, [0.07, 0.24]
  representative: CinephileItem[];
}

export interface GalaxyLayoutOptions {
  maxNodes: number;
  representativePerNode: number;
}

const MIN_RADIUS = 0.07;
const MAX_RADIUS = 0.24;
const MIN_ORBIT = 0.5;
const ORBIT_JITTER = 0.16;

export function radiusForShare(share: number): number {
  const clamped = Math.max(0, Math.min(1, share));
  return MIN_RADIUS + (MAX_RADIUS - MIN_RADIUS) * Math.sqrt(clamped);
}

// 0..1 deterministic jitter derived purely from the node's own id.
function jitterFor(id: string): number {
  return (hashString(id) % 1000) / 1000;
}

export function buildGalaxyLayout(buckets: DecadeBucket[], opts: GalaxyLayoutOptions): GalaxyNode[] {
  const maxNodes = Math.max(1, Math.floor(opts.maxNodes));
  const repCount = Math.max(0, Math.floor(opts.representativePerNode));
  const totalCount = buckets.reduce((sum, b) => sum + b.items.length, 0);
  if (totalCount === 0) return [];

  const chronological = buckets.filter((b) => b.items.length > 0).slice().sort((a, b) => a.decade - b.decade);

  let kept = chronological;
  let other: CinephileItem[] | null = null;
  if (chronological.length > maxNodes) {
    const bySizeDesc = chronological.slice().sort((a, b) => b.items.length - a.items.length);
    const keepDecades = new Set(bySizeDesc.slice(0, maxNodes - 1).map((b) => b.decade));
    kept = chronological.filter((b) => keepDecades.has(b.decade));
    const overflow = chronological.filter((b) => !keepDecades.has(b.decade));
    const overflowItems = overflow.flatMap((b) => b.items);
    if (overflowItems.length > 0) other = overflowItems;
  }

  interface Slot {
    id: string;
    decade: number | null;
    items: CinephileItem[];
  }
  const slots: Slot[] = kept.map((b) => ({ id: `decade-${b.decade}`, decade: b.decade, items: b.items }));
  if (other) slots.push({ id: "other", decade: null, items: other });

  const n = slots.length;
  const nodes: GalaxyNode[] = [];
  for (let i = 0; i < n; i++) {
    const slot = slots[i];
    const count = slot.items.length;
    const share = count / totalCount;
    const angle = (i / n) * Math.PI * 2 - Math.PI / 2;
    const orbit = MIN_ORBIT + jitterFor(slot.id) * ORBIT_JITTER;
    nodes.push({
      id: slot.id,
      decade: slot.decade,
      count,
      share,
      x: Math.cos(angle) * orbit,
      y: Math.sin(angle) * orbit,
      radius: radiusForShare(share),
      representative: pickRepresentativeItems(slot.items, repCount, hashString(slot.id)),
    });
  }
  return nodes;
}
