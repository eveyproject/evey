import { describe, it, expect } from "vitest";
import type { V1LibraryType } from "@evey/api";
import { selectHomeLibraryRows, MAX_HOME_LIBRARY_ROWS } from "./homeLibraryRows";

function lib(id: string, name: string, type: V1LibraryType = "LIBRARY_TYPE_MOVIES") {
  return { id, name, type };
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
    const libraries = [lib("lib-1", "Movies"), { name: "No Id", type: "LIBRARY_TYPE_MOVIES" as V1LibraryType }];
    expect(selectHomeLibraryRows(libraries)).toEqual([{ id: "lib-1", name: "Movies" }]);
  });

  // A row only exists for a type the TV has somewhere to SEND the viewer.
  // Music and photos earned one on 2026-08-02, when the home gained rows that
  // land on the album and photos screens instead of the video detail page.
  // A type the TV has no screen for gets no row: a card that lands nowhere is
  // worse than an absent row.
  it("rows video, music and photo libraries, but not an unrouted type", () => {
    const libraries = [
      lib("lib-mov", "Films", "LIBRARY_TYPE_MOVIES"),
      lib("lib-pho", "Photos", "LIBRARY_TYPE_PHOTOS"),
      lib("lib-mus", "Musique", "LIBRARY_TYPE_MUSIC"),
      lib("lib-unk", "Inconnu", "LIBRARY_TYPE_UNSPECIFIED"),
      lib("lib-sho", "Séries", "LIBRARY_TYPE_SHOWS"),
    ];
    expect(selectHomeLibraryRows(libraries)).toEqual([
      { id: "lib-mov", name: "Films" },
      { id: "lib-pho", name: "Photos" },
      { id: "lib-mus", name: "Musique" },
      { id: "lib-sho", name: "Séries" },
    ]);
  });

  it("skips a library with no type rather than guessing it is video", () => {
    expect(selectHomeLibraryRows([{ id: "lib-1", name: "Mystery" }])).toEqual([]);
  });

  it("skips a library missing a name (cannot be labeled)", () => {
    const libraries = [lib("lib-1", "Movies"), { id: "lib-2", type: "LIBRARY_TYPE_MOVIES" as V1LibraryType }];
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
