import { describe, it, expect } from "vitest";
import { buildWallLayout, type WallLayoutOptions } from "../src/domain/services/wallLayout";

const BASE: WallLayoutOptions = {
  count: 24,
  rows: 3,
  colSpacing: 1,
  rowRise: 0.6,
  rowScaleFalloff: 0.8,
  rowStaggerFraction: 0.5,
};

describe("buildWallLayout", () => {
  it("produces exactly `count` slots", () => {
    const layout = buildWallLayout(BASE);
    expect(layout.slots).toHaveLength(24);
  });

  it("distributes posters across rows filling the front row first", () => {
    const layout = buildWallLayout(BASE);
    const perRow = new Map<number, number>();
    for (const slot of layout.slots) perRow.set(slot.row, (perRow.get(slot.row) ?? 0) + 1);
    expect(perRow.get(0)).toBe(layout.colsPerRow);
    expect([...perRow.keys()].every((r) => r < layout.rows)).toBe(true);
  });

  it("makes the front row the biggest (scale 1) and further rows smaller", () => {
    const layout = buildWallLayout(BASE);
    const scaleByRow = new Map<number, number>();
    for (const slot of layout.slots) scaleByRow.set(slot.row, slot.scale);
    expect(scaleByRow.get(0)).toBe(1);
    const rows = [...scaleByRow.keys()].sort((a, b) => a - b);
    for (let i = 1; i < rows.length; i++) {
      expect(scaleByRow.get(rows[i])!).toBeLessThan(scaleByRow.get(rows[i - 1])!);
    }
  });

  it("centers each row's columns around x=0", () => {
    const layout = buildWallLayout(BASE);
    const row0 = layout.slots.filter((s) => s.row === 0).map((s) => s.x);
    const sum = row0.reduce((a, b) => a + b, 0);
    expect(Math.abs(sum)).toBeLessThan(1e-9);
  });

  it("gives further rows a z closer to 1 and the front row z=0", () => {
    const layout = buildWallLayout(BASE);
    const front = layout.slots.find((s) => s.row === 0)!;
    const back = layout.slots.find((s) => s.row === layout.rows - 1)!;
    expect(front.z).toBe(0);
    expect(back.z).toBe(1);
  });

  it("computes the loop period from the widest row", () => {
    const layout = buildWallLayout(BASE);
    expect(layout.period).toBeCloseTo(layout.colsPerRow * BASE.colSpacing, 9);
  });

  it("handles an empty library without NaN or throwing", () => {
    const layout = buildWallLayout({ ...BASE, count: 0 });
    expect(layout.slots).toHaveLength(0);
    expect(Number.isFinite(layout.period)).toBe(true);
    expect(layout.period).toBeGreaterThan(0);
  });

  it("handles a partial last row without NaN", () => {
    const layout = buildWallLayout({ ...BASE, count: 7 });
    expect(layout.slots).toHaveLength(7);
    for (const slot of layout.slots) {
      expect(Number.isFinite(slot.x)).toBe(true);
      expect(Number.isFinite(slot.y)).toBe(true);
      expect(Number.isFinite(slot.scale)).toBe(true);
    }
  });

  it("clamps rows to at least 1", () => {
    const layout = buildWallLayout({ ...BASE, rows: 0 });
    expect(layout.rows).toBe(1);
    expect(layout.slots.every((s) => s.row === 0)).toBe(true);
  });
});
