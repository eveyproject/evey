import { describe, it, expect } from "vitest";
import { resolvePlayTarget } from "./playTarget";

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
