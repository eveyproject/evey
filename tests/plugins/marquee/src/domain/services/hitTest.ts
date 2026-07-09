// Pointer picking against the wall layout: given a point already expressed
// in the same normalized "wall space" as the slots (the render adapter owns
// converting client/canvas pixels into that space), returns the frontmost
// slot under the pointer, or null. Wraps the horizontal axis by `period` so a
// poster that's currently drawn in its "phantom" copy (for seamless looping)
// is still clickable.

import type { WallSlot } from "../model/wall";

export interface HitPoint {
  x: number;
  y: number;
}

export interface HitBox {
  halfWidth: number;
  halfHeight: number;
}

export function hitTestWall(slots: WallSlot[], scrollOffset: number, period: number, point: HitPoint, box: HitBox): number | null {
  const offsets = period > 0 ? [-period, 0, period] : [0];
  let best: WallSlot | null = null;

  for (const slot of slots) {
    const hw = box.halfWidth * slot.scale;
    const hh = box.halfHeight * slot.scale;
    for (const dx of offsets) {
      const sx = slot.x - scrollOffset + dx;
      if (Math.abs(point.x - sx) <= hw && Math.abs(point.y - slot.y) <= hh) {
        if (!best || slot.z < best.z) best = slot;
        break;
      }
    }
  }

  return best ? best.index : null;
}
