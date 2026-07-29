import { describe, it, expect } from "vitest";
import { selectHomeLibraryRows, MAX_HOME_LIBRARY_ROWS } from "./homeLibraryRows";

function lib(id: string, name: string) {
  return { id, name };
}

describe("selectHomeLibraryRows", () => {
  it("returns an empty list when there are no libraries", () => {
    expect(selectHomeLibraryRows([])).toEqual([]);
  });

  it("maps id/name through unchanged, in server order", () => {
    const libraries = [lib("lib-1", "Movies"), lib("lib-2", "Shows")];
    expect(selectHomeLibraryRows(libraries)).toEqual([
      { id: "lib-1", name: "Movies" },
      { id: "lib-2", name: "Shows" },
    ]);
  });

  it("skips a library missing an id (cannot be queried)", () => {
    const libraries = [lib("lib-1", "Movies"), { name: "No Id" }];
    expect(selectHomeLibraryRows(libraries)).toEqual([{ id: "lib-1", name: "Movies" }]);
  });

  it("skips a library missing a name (cannot be labeled)", () => {
    const libraries = [lib("lib-1", "Movies"), { id: "lib-2" }];
    expect(selectHomeLibraryRows(libraries)).toEqual([{ id: "lib-1", name: "Movies" }]);
  });

  it("keeps exactly MAX_HOME_LIBRARY_ROWS libraries untouched at the cap", () => {
    const libraries = Array.from({ length: MAX_HOME_LIBRARY_ROWS }, (_, i) => lib(`lib-${i}`, `Library ${i}`));
    const rows = selectHomeLibraryRows(libraries);
    expect(rows).toHaveLength(MAX_HOME_LIBRARY_ROWS);
    expect(rows[rows.length - 1]).toEqual({
      id: `lib-${MAX_HOME_LIBRARY_ROWS - 1}`,
      name: `Library ${MAX_HOME_LIBRARY_ROWS - 1}`,
    });
  });

  it("truncates beyond the cap, keeping the first ones in order", () => {
    const libraries = Array.from({ length: MAX_HOME_LIBRARY_ROWS + 3 }, (_, i) => lib(`lib-${i}`, `Library ${i}`));
    const rows = selectHomeLibraryRows(libraries);
    expect(rows).toHaveLength(MAX_HOME_LIBRARY_ROWS);
    expect(rows[0]).toEqual({ id: "lib-0", name: "Library 0" });
    expect(rows[rows.length - 1]).toEqual({
      id: `lib-${MAX_HOME_LIBRARY_ROWS - 1}`,
      name: `Library ${MAX_HOME_LIBRARY_ROWS - 1}`,
    });
  });
});
