import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

// Where the filter panel's options come from.
//
// The audio- and subtitle-language chips are derived from the STREAMS of media
// items the app holds. Derived from the infinite list alone, that is the first
// page - 30 items out of a thousand - so a viewer saw three audio languages and
// three subtitle languages, always the same three, and could not filter on
// anything the top of their library did not happen to contain. It looked
// hardcoded because it never changed. Web has always fired a dedicated sampling
// query for this; mobile had not (user, 2026-09-05).
//
// The other half of the same panel: the genre chips merged a hardcoded English
// list into the library's real genres. Removed from web on 2026-09-04 and left
// on mobile, so a French library still listed every genre twice - and each
// English chip that was not also a real genre filtered down to nothing.
//
// Static, because both are about the SOURCE a hook is given, and a rendered
// test would only prove that the fixture it was handed is the fixture it got.

const LIBRARY = join(__dirname, "..", "..", "app", "(auth)", "library", "[libraryId].tsx");

function source(): string {
  return readFileSync(LIBRARY, "utf8");
}

describe("library filter panel: where the options come from", () => {
  it("samples a dedicated page for the facets instead of reading the loaded list alone", () => {
    const src = source();
    expect(
      src,
      "collectFilterOptions fed only by rawItems sees one page: the language chips then show " +
        "whatever the top of the library happens to contain, and never change",
    ).not.toMatch(/collectFilterOptions\(rawItems\)/);
    expect(src).toMatch(/"pagination\.pageSize": PAGE_SIZE \* 4/);
  });

  it("merges the sample with what is already on screen", () => {
    expect(
      source(),
      "a language the viewer can see on a poster in front of them must never be missing from the panel",
    ).toMatch(/collectFilterOptions\(\[\.\.\.\(facetSample\?\.items \?\? \[\]\), \.\.\.rawItems\]\)/);
  });

  it("gates the sample on the panel being open", () => {
    expect(
      source(),
      "an extra request on every library visit, for a panel most visits never open",
    ).toMatch(/enabled: !!libraryId && showFilters/);
  });

  it("offers only the genres the library really has", () => {
    const src = source();
    expect(
      src,
      "a hardcoded English genre list shows every genre twice on a French library, and half the " +
        "chips match nothing at all",
    ).not.toMatch(/COMMON_GENRES/);
    expect(src).toMatch(/genresData\?\.genres \?\? \[\]/);
  });
});
