// Pure layout of the poster wall: a perspective grid of `rows` rows (front
// row biggest/closest, back rows smaller/further), columns spread evenly
// around x=0. Called once per mount (or media-selection change) - never from
// the render loop.

import type { WallLayout, WallSlot } from "../model/wall";

export interface WallLayoutOptions {
  count: number;
  rows: number;
  colSpacing: number; // horizontal distance between poster centers
  rowRise: number; // vertical rise per row further back
  rowScaleFalloff: number; // e.g. 0.82 -> each row back is 82% the scale of the one in front
  rowStaggerFraction: number; // odd rows offset by this fraction of colSpacing (brick pattern)
}

export function buildWallLayout(opts: WallLayoutOptions): WallLayout {
  const count = Math.max(0, Math.floor(opts.count));
  const rows = Math.max(1, Math.floor(opts.rows));
  const colsPerRow = count === 0 ? 1 : Math.max(1, Math.ceil(count / rows));
  const period = colsPerRow * opts.colSpacing;

  const slots: WallSlot[] = [];
  for (let i = 0; i < count; i++) {
    const row = Math.floor(i / colsPerRow);
    const col = i % colsPerRow;
    const stagger = row % 2 === 1 ? opts.colSpacing * opts.rowStaggerFraction : 0;
    const x = (col - (colsPerRow - 1) / 2) * opts.colSpacing + stagger;
    const y = row * opts.rowRise;
    const z = rows > 1 ? row / (rows - 1) : 0;
    const scale = Math.pow(opts.rowScaleFalloff, row);
    slots.push({ index: i, row, col, x, y, scale, z });
  }

  return { slots, rows, colsPerRow, period: period > 0 ? period : opts.colSpacing };
}
