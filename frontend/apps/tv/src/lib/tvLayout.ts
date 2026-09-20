// Shared layout constants for TV screens. TV targets a fixed 1920px-wide
// display (10-foot UI, no per-device scaling), and every poster grid/row
// reuses one of the two card widths below. Values are named here, not
// changed - each screen previously redeclared the same numbers locally.

// Reference viewport width used by screens that need to derive a layout
// size (e.g. an even column split) without a runtime Dimensions lookup.
export const TV_SCREEN_WIDTH = 1920;

// Card width used by hero/featured rows and detail posters (home rows,
// media detail poster, music artist detail albums).
export const CARD_WIDTH_FEATURED = 220;

// Card width used by dense grid screens (favorites, library, music home rows).
export const CARD_WIDTH_GRID = 200;

// Poster height = width * POSTER_RATIO across every poster card.
export const POSTER_RATIO = 1.5;

// Horizontal breathing room for a screen that renders WITHOUT the sidebar
// (notifications, history, playlists, collections, genres, stats...). The tab
// screens get the 115 dp rail plus their own 24; these had 24 alone, which on
// an overscanning television puts the first characters of every row against
// the bezel (the owner had already hit this on the Quick Connect screen).
export const DEEP_SCREEN_GUTTER = 48;

// Standard gap between cards/rows in grids and horizontal lists. Measured
// against the platform TV library on the owner's box (2026-09-20): it spaces
// browse items by 8dp at density 2.0; our screens render at 1.6, so 10dp lands
// on the same pixel gap. The previous 16 was 60% wider than any TV launcher.
export const GRID_GAP = 10;

// Type scale. TV text is read from a couch, so these are NOT web sizes: they
// mirror the platform TV library's own scale (card 14sp, row header 16-20sp,
// details body 14sp, details title 34sp), converted for our 1.6 density.
export const FONT = {
  caption: 13,
  label: 15,
  body: 17,
  subtitle: 20,
  title: 24,
  heading: 30,
  display: 42,
} as const;

// Chrome scale: what a focusable element reserves around its label. The
// platform library gives a one-line action 36dp of height and 24dp of side
// padding; ours were 56dp tall, which is what made every screen feel oversized.
export const CONTROL = {
  paddingVertical: 10,
  paddingHorizontal: 28,
  radius: 8,
  gap: 12,
} as const;

// Height reserved below a CARD_WIDTH_GRID poster for its title/year label
// block, used when computing FlatList getItemLayout row heights.
export const LABEL_BLOCK_HEIGHT = 40;
