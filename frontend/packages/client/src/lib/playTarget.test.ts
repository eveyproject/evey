import { describe, it, expect } from "vitest";
import { compareEpisodeIndex, resolvePlayTarget, sortByIndexNumber, sortEpisodesByIndex } from "./playTarget";

describe("resolvePlayTarget", () => {
  it("plays a movie by its own id", () => {
    expect(
      resolvePlayTarget({ id: "mi-movie", isSeries: false, isSeason: false, episodeIds: [] }),
    ).toBe("mi-movie");
  });

  it("plays the first episode of a series", () => {
    expect(
      resolvePlayTarget({ id: "mi-series", isSeries: true, isSeason: false, episodeIds: ["mi-ep1", "mi-ep2"] }),
    ).toBe("mi-ep1");
  });

  it("plays the first episode of a season", () => {
    expect(
      resolvePlayTarget({ id: "mi-season", isSeries: false, isSeason: true, episodeIds: ["mi-ep7"] }),
    ).toBe("mi-ep7");
  });

  // The regression: the episode list arrives in a second request, and handing
  // the series id to the player streams a file that does not exist.
  it("returns nothing for a series whose episodes have not loaded yet", () => {
    expect(
      resolvePlayTarget({ id: "mi-series", isSeries: true, isSeason: false, episodeIds: [] }),
    ).toBeNull();
  });

  it("returns nothing for a season whose episodes have not loaded yet", () => {
    expect(
      resolvePlayTarget({ id: "mi-season", isSeries: false, isSeason: true, episodeIds: [] }),
    ).toBeNull();
  });

  it("skips episodes the server returned without an id", () => {
    expect(
      resolvePlayTarget({ id: "mi-series", isSeries: true, isSeason: false, episodeIds: [undefined, "mi-ep2"] }),
    ).toBe("mi-ep2");
  });

  it("returns nothing without a media id", () => {
    expect(
      resolvePlayTarget({ id: undefined, isSeries: false, isSeason: false, episodeIds: [] }),
    ).toBeNull();
  });
});

// resolvePlayTarget takes the FIRST id it is given, so "plays the first
// episode" is only true while the caller hands it an index-ordered array. The
// server cannot provide that order - ListMediaItems has no sort key for the
// episode index and answers in sort_title order - so the two functions are one
// contract, and this is the half that was missing on the TV.
describe("sortByIndexNumber", () => {
  it("puts episode 1 first whatever order the server sent", () => {
    // sort_title order for episodes titled "Alpha", "Beta", "Zulu": the server
    // would hand back 3, 1, 2 and the play button would start on episode 3.
    const fromServer = [
      { id: "mi-ep3", indexNumber: 3 },
      { id: "mi-ep1", indexNumber: 1 },
      { id: "mi-ep2", indexNumber: 2 },
    ];

    const ordered = sortByIndexNumber(fromServer);

    expect(ordered.map((e) => e.id)).toEqual(["mi-ep1", "mi-ep2", "mi-ep3"]);
    expect(
      resolvePlayTarget({
        id: "mi-season",
        isSeries: false,
        isSeason: true,
        episodeIds: ordered.map((e) => e.id),
      }),
    ).toBe("mi-ep1");
  });

  it("does not mutate the array it was given", () => {
    const original = [{ indexNumber: 2 }, { indexNumber: 1 }];
    sortByIndexNumber(original);
    expect(original.map((e) => e.indexNumber)).toEqual([2, 1]);
  });

  it("keeps items with no index number ahead rather than dropping them", () => {
    const ordered = sortByIndexNumber([{ id: "b", indexNumber: 2 }, { id: "a" }]);
    expect(ordered.map((e) => e.id)).toEqual(["a", "b"]);
  });
});

// An episode the server holds without a number (a file of a season folder
// whose name states none) reaches the client as indexNumber 0 or absent: the
// proto field is a plain int32. Sorted as 0 it went ahead of episode 1, so
// Play on the season, "next" and "previous" started on it.
describe("sortEpisodesByIndex", () => {
  it("puts unnumbered episodes after the numbered ones, never ahead of episode 1", () => {
    const fromServer = [
      { id: "mi-extra", indexNumber: 0 },
      { id: "mi-ep2", indexNumber: 2 },
      { id: "mi-bonus" },
      { id: "mi-ep1", indexNumber: 1 },
    ];

    const ordered = sortEpisodesByIndex(fromServer);

    expect(ordered.map((e) => e.id)).toEqual(["mi-ep1", "mi-ep2", "mi-extra", "mi-bonus"]);
    expect(
      resolvePlayTarget({ id: "mi-season", isSeries: false, isSeason: true, episodeIds: ordered.map((e) => e.id) }),
    ).toBe("mi-ep1");
  });

  it("compares two unnumbered episodes as equal, so the sort keeps the server order", () => {
    expect(compareEpisodeIndex({ indexNumber: 0 }, {})).toBe(0);
    expect(compareEpisodeIndex({ indexNumber: 3 }, {})).toBeLessThan(0);
  });
});
