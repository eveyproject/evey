import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

// Where the web filter panel's codec, resolution and language chips come from.
//
// They were read off a dedicated sample, the first PAGE_SIZE * 4 = 192 items,
// so a language or a codec only the rest of the library carried was never
// offered. They now come from GetLibraryFilterFacets, the library's full
// distinct set. Static, like the phone's guard (library-facets-source.test.ts):
// the point is the SOURCE the panel is given, which a rendered test would only
// see as the fixture it was handed.

const ROUTE = join(dirname(fileURLToPath(import.meta.url)), "..", "..", "routes", "_auth", "library", "$libraryId.lazy.tsx");

function source(): string {
  return readFileSync(ROUTE, "utf8");
}

describe("web library filter panel: where the options come from", () => {
  it("asks the server for the whole library's facets instead of sampling items", () => {
    const src = source();
    expect(src, "the 192-item sample only ever saw the top of the library").not.toMatch(/PAGE_SIZE \* 4/);
    expect(src).toMatch(/useLibraryServiceGetLibraryFilterFacets\(/);
    expect(src).toMatch(/libraryFilterOptions\(facetsData\)/);
  });

  it("narrows the facets by the grid's type and asks only once the panel is open", () => {
    const src = source();
    expect(src).toMatch(/typeFilter \? \{ type: typeFilter \} : undefined/);
    expect(src).toMatch(/enabled: !!libraryId && filtersOpen/);
  });
});
