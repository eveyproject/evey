import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

// The library grid declares its row height to FlatList through getItemLayout,
// and FlatList TRUSTS it: with getItemLayout supplied it never measures a row.
// So every constant that feeds ROW_HEIGHT has to describe what is actually
// drawn, and the cell has to be unable to grow past it.
//
// Two ways that broke at once, both live on 2026-09-05:
//
//   - POSTER_HEIGHT was ITEM_WIDTH * 1.5, but MediaCard carries a marginRight
//     of SPACING on its own box, so the poster it draws is (ITEM_WIDTH -
//     SPACING) wide. The declared row was ~12px taller than any real row;
//   - the technical badges (rating, resolution, HDR) were positioned `bottom: 6`
//     of the CELL - poster plus title plus year - so the quality chip rendered
//     under the artwork, in the gap before the next row. "La qualité est entre 2
//     vignettes."
//
// Read as source rather than rendered: these are layout constants and an
// absolute-position anchor, and a jsdom FlatList measures nothing.

const LIBRARY = join(__dirname, "..", "..", "app", "(auth)", "library", "[libraryId].tsx");

function source(): string {
  return readFileSync(LIBRARY, "utf8");
}

describe("library grid geometry", () => {
  it("derives the poster height from the width MediaCard actually receives", () => {
    const src = source();
    expect(
      src,
      "MediaCard reserves SPACING for its own marginRight, so a poster derived from ITEM_WIDTH " +
        "describes one nobody draws - and every row height built on it is wrong",
    ).toMatch(/const CARD_WIDTH = ITEM_WIDTH - SPACING;/);
    expect(src).toMatch(/const POSTER_HEIGHT = CARD_WIDTH \* 1\.5;/);
  });

  it("passes that same width to the card", () => {
    expect(source()).toMatch(/width=\{CARD_WIDTH\}/);
  });

  it("builds the row height from the pinned parts, never from a guessed constant", () => {
    const src = source();
    expect(src).toMatch(/const ROW_HEIGHT = POSTER_HEIGHT \+ META_HEIGHT \+ SPACING;/);
    expect(
      src,
      "ROW_HEIGHT + '30' was a guess at the title block; a cell that renders taller than the " +
        "number handed to getItemLayout drifts a few pixels per row, forever",
    ).not.toMatch(/ROW_HEIGHT = POSTER_HEIGHT \+ 30/);
  });

  // The caption height must come from the component that DRAWS the caption.
  // Guessed at 34 here, it was two pixels short of the two lines MediaCard
  // renders, and the cell's own clip sliced the year in half - a fix that
  // introduced a defect of its own (emulator, 2026-09-05).
  it("takes the caption height from MediaCard instead of guessing it", () => {
    const src = source();
    expect(src).toMatch(/const META_HEIGHT = CARD_CAPTION_HEIGHT;/);
    expect(src).toMatch(/import \{ MediaCard, CARD_CAPTION_HEIGHT \}/);
  });

  it("pins the caption line height in MediaCard, so the total is arithmetic", () => {
    const card = readFileSync(
      join(__dirname, "..", "components", "MediaCard.tsx"),
      "utf8",
    );
    expect(
      card,
      "left to the platform's default leading, two 13px lines are not a number this grid can rely on",
    ).toMatch(/export const CARD_CAPTION_HEIGHT = CARD_TEXT_GAP \+ CARD_LINE_HEIGHT \* 2;/);
    // Both lines must actually use it, or the constant describes nothing.
    expect(card.match(/lineHeight: CARD_LINE_HEIGHT/g) ?? []).toHaveLength(2);
  });

  it("clips the cell to the height it promised, so a long title cannot stretch a row", () => {
    expect(source()).toMatch(/height: POSTER_HEIGHT \+ META_HEIGHT, overflow: "hidden"/);
  });

  it("anchors the badge layer to the poster, not to the cell", () => {
    const src = source();
    expect(
      src,
      "the badges must live in a layer the size of the POSTER; anchored to the cell they land " +
        "below the artwork, between two thumbnails",
    ).toMatch(/position: "absolute", top: 0, left: 0, width: CARD_WIDTH, height: POSTER_HEIGHT/);
  });
});
