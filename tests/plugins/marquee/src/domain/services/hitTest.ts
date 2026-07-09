// Pointer picking against the wall layout: given a point already expressed
// in the same normalized "wall space" as the slots (the render adapter owns
// converting client/canvas pixels into that space), returns the slot the user
// actually SEES under the pointer, or null. Wraps the horizontal axis by
// `period` so a poster drawn in its "phantom" copy (for seamless looping) is
// still clickable.
//
// Overlap resolution follows the renderer's painter order, NOT z: the wall is
// drawn back-to-front with no depth test (blending only), and `slots` is passed
// in that same draw order, so where two posters overlap the LAST one in the list
// is the one painted on top - hence the one the user is looking at and means to
// click. (Picking the frontmost-z instead made a middle-row poster's click land
// on the bigger front-row poster drawn beneath it.)

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
        // Painter's order: keep overwriting so the last (topmost) match wins.
        best = slot;
        break;
      }
    }
  }

  return best ? best.index : null;
}
