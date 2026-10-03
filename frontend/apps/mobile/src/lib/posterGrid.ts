import { Dimensions } from "react-native";

// A phone shows 3 poster columns. Proportional alone, a 914 dp tablet drew 3
// posters of ~295 dp: past the widest phone the poster keeps a phone-ish size
// (~170 dp) and the grid adds columns instead.
export const POSTER_GRID_SPACING = 8;
const PHONE_COLUMNS = 3;
const TABLET_MIN_WIDTH = 600;
const TARGET_POSTER_WIDTH = 170;
// Tablet screens sit beside the nav rail, which the cells must not overflow.
const RAIL_WIDTH = 52;

export function posterGrid(windowWidth: number, spacing = POSTER_GRID_SPACING, phoneColumns = PHONE_COLUMNS) {
  const width = windowWidth >= TABLET_MIN_WIDTH ? windowWidth - RAIL_WIDTH : windowWidth;
  const columns = Math.max(phoneColumns, Math.round(width / TARGET_POSTER_WIDTH));
  return { columns, itemWidth: (width - spacing * (columns + 1)) / columns };
}

export const POSTER_GRID = posterGrid(Dimensions.get("window").width);
