import { describe, it, expect } from "vitest";
import { hitTestWall } from "../src/domain/services/hitTest";
import type { WallSlot } from "../src/domain/model/wall";

function slot(overrides: Partial<WallSlot>): WallSlot {
  return { index: 0, row: 0, col: 0, x: 0, y: 0, scale: 1, z: 0, ...overrides };
}

const BOX = { halfWidth: 0.4, halfHeight: 0.6 };

describe("hitTestWall", () => {
  it("hits the slot whose box contains the point", () => {
    const slots = [slot({ index: 0, x: 0, y: 0 }), slot({ index: 1, x: 2, y: 0 })];
    expect(hitTestWall(slots, 0, 4, { x: 0.1, y: 0.1 }, BOX)).toBe(0);
    expect(hitTestWall(slots, 0, 4, { x: 2.1, y: 0.1 }, BOX)).toBe(1);
  });

  it("returns null when the point is outside every box", () => {
    const slots = [slot({ index: 0, x: 0, y: 0 })];
    expect(hitTestWall(slots, 0, 4, { x: 5, y: 5 }, BOX)).toBeNull();
  });

  it("accounts for the current scroll offset", () => {
    const slots = [slot({ index: 0, x: 0, y: 0 })];
    // A positive scroll offset shifts every slot's effective screen position
    // by -offset, so the point that now lands on the slot moved with it.
    expect(hitTestWall(slots, 1, 4, { x: -1, y: 0 }, BOX)).toBe(0);
    expect(hitTestWall(slots, 1, 4, { x: 0, y: 0 }, BOX)).toBeNull();
  });

  it("wraps horizontally so the phantom copy is also clickable", () => {
    const slots = [slot({ index: 0, x: 0, y: 0 })];
    const period = 4;
    // Point lands near the +period phantom copy of slot 0.
    expect(hitTestWall(slots, 0, period, { x: period, y: 0 }, BOX)).toBe(0);
    expect(hitTestWall(slots, 0, period, { x: -period, y: 0 }, BOX)).toBe(0);
  });

  it("scales the hit box by the slot's scale", () => {
    const slots = [slot({ index: 0, x: 0, y: 0, scale: 0.1 })];
    expect(hitTestWall(slots, 0, 4, { x: 0.3, y: 0 }, BOX)).toBeNull();
  });

  it("prefers the poster drawn last (painted on top) when boxes overlap", () => {
    // slots are supplied in the renderer's draw order (back-to-front, no depth
    // test), so the later slot is painted on top and is the one the user sees
    // and clicks - even though it is a higher (further) row than the one below.
    const frontRow = slot({ index: 0, row: 0, x: 0, y: 0, z: 0 }); // drawn first
    const middleRow = slot({ index: 1, row: 1, x: 0, y: 0, z: 0.5 }); // drawn later, on top
    expect(hitTestWall([frontRow, middleRow], 0, 4, { x: 0, y: 0 }, BOX)).toBe(1);
  });

  it("handles an empty slot list", () => {
    expect(hitTestWall([], 0, 4, { x: 0, y: 0 }, BOX)).toBeNull();
  });
});
