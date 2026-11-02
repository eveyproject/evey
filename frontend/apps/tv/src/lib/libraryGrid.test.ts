import { describe, it, expect } from "vitest";
import { libraryGridFit, librarySelectionSummary, type LibraryGridFitInput } from "./libraryGrid";

// The library grid's own numbers: a 165 dp card, its 3 dp focus ring, two
// 20 dp title lines under an 8 dp gap, 16 dp between rows, 12 dp above.
const BASE: Omit<LibraryGridFitInput, "availableHeight"> = {
  nominalCardWidth: 165,
  posterRatio: 1.5,
  ring: 3,
  titleGap: 8,
  titleBlock: 40,
  rowGap: 16,
  topInset: 12,
  focusScale: 1.05,
  minScale: 0.75,
};

describe("libraryGridFit", () => {
  it("shrinks the cards a little to show a second whole row instead of the top of its posters", () => {
    // A 675 dp tall screen, less its title bar and the home shortcut's band.
    const fit = libraryGridFit({ ...BASE, availableHeight: 531 });
    expect(fit.rows).toBe(2);
    expect(fit.cardWidth).toBe(129);
    expect(fit.listHeight).toBeLessThanOrEqual(531);
  });

  it("keeps full-size cards and shows one whole row when a second would need posters far smaller", () => {
    const fit = libraryGridFit({ ...BASE, availableHeight: 396 });
    expect(fit.rows).toBe(1);
    expect(fit.cardWidth).toBe(165);
    // The inset, the card and the zoom of a focused card: nothing of row two.
    expect(fit.listHeight).toBe(322);
  });

  it("shrinks the card until one whole row fits when even one does not at full size", () => {
    const fit = libraryGridFit({ ...BASE, availableHeight: 250 });
    expect(fit.rows).toBe(1);
    expect(fit.cardWidth).toBeLessThan(165);
    expect(fit.listHeight).toBeLessThanOrEqual(250);
  });

  it("never lets the list end inside a row, at any screen height", () => {
    for (let h = 200; h <= 1400; h += 7) {
      const fit = libraryGridFit({ ...BASE, availableHeight: h });
      expect(fit.rows).toBeGreaterThanOrEqual(1);
      expect(fit.listHeight).toBeLessThanOrEqual(h);
      // Every row is whole: inset + rows + gaps + the last row's zoom is the list.
      const rowsSpan = BASE.topInset + fit.rows * fit.cardHeight + (fit.rows - 1) * BASE.rowGap + fit.zoomOverhang;
      expect(fit.listHeight).toBe(Math.ceil(rowsSpan));
      // And the next row starts below the list's bottom edge.
      const nextRowTop = BASE.topInset + fit.rows * (fit.cardHeight + BASE.rowGap);
      expect(nextRowTop).toBeGreaterThanOrEqual(fit.listHeight);
      expect(fit.cardWidth).toBeLessThanOrEqual(165);
    }
  });

  it("counts a larger font: the taller title block it reserves still fits whole", () => {
    const large = libraryGridFit({ ...BASE, titleBlock: 40 * 1.3, availableHeight: 531 });
    // Two rows would need posters under three quarters of their size: one
    // full-size row, its taller title included.
    expect(large.rows).toBe(1);
    expect(large.cardHeight).toBe(2 * 3 + 8 + 52 + 165 * 1.5);
    expect(large.listHeight).toBeLessThanOrEqual(531);
  });
});

describe("librarySelectionSummary", () => {
  it("names only the sort and its direction when no filter is set", () => {
    expect(librarySelectionSummary({ sort: "Title", ascending: true })).toBe("Title ↑");
  });

  it("names the genre and the type before the sort", () => {
    expect(librarySelectionSummary({ genre: "Drama", type: "Episodes", sort: "Year", ascending: false })).toBe("Drama · Episodes · Year ↓");
  });
});
