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

// Standard gap between cards/rows in grids and horizontal lists.
export const GRID_GAP = 16;

// Height reserved below a CARD_WIDTH_GRID poster for its title/year label
// block, used when computing FlatList getItemLayout row heights.
export const LABEL_BLOCK_HEIGHT = 40;
