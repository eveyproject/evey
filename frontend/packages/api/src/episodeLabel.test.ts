import { describe, expect, it } from "vitest";
import { episodeCode, episodeLabel } from "./episodeLabel";

describe("episodeLabel", () => {
  it("names the series, the episode code and the title", () => {
    expect(episodeLabel({ seriesTitle: "Slow Horses", seasonNumber: 1, episodeNumber: 3, title: "Bad Tradecraft" }))
      .toBe("Slow Horses · S01E03 · Bad Tradecraft");
  });
  it("leaves out whatever is missing", () => {
    expect(episodeLabel({ seriesTitle: "Slow Horses", episodeNumber: 3, title: "Bad Tradecraft" })).toBe("Slow Horses · E03 · Bad Tradecraft");
    expect(episodeLabel({ seasonNumber: 2, episodeNumber: 10, title: "Finale" })).toBe("S02E10 · Finale");
    expect(episodeLabel({ seriesTitle: " Slow Horses ", title: "  " })).toBe("Slow Horses");
    expect(episodeLabel({ title: "Prédestination" })).toBe("Prédestination");
    expect(episodeLabel({})).toBe("");
  });
  it("pads to two digits and ignores zero or negative numbers", () => {
    expect(episodeCode(1, 3)).toBe("S01E03");
    expect(episodeCode(12, 105)).toBe("S12E105");
    expect(episodeCode(0, 4)).toBe("E04");
    expect(episodeCode(3, 0)).toBe("S03");
    expect(episodeCode(null, undefined)).toBe("");
  });
});
