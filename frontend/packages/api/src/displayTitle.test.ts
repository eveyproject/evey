import { describe, it, expect } from "vitest";
import { displayTitle, itemLabel, seasonLabel, SEASON_TYPE } from "./displayTitle";

// Stands in for i18next: proves the label went through media.season rather
// than through whatever the scan froze in the row.
const t = (key: string, options?: Record<string, unknown>) =>
  key === "media.season" ? `Saison ${options?.number}` : key;

describe("seasonLabel", () => {
  it("derives the label from the number, never from the stored title", () => {
    expect(seasonLabel({ title: "Season 3", indexNumber: 3 }, t)).toBe("Saison 3");
    // Rows enriched before 8a09537ea carry a French title written server-side.
    expect(seasonLabel({ title: "Saison 3", indexNumber: 3 }, t)).toBe("Saison 3");
    // And a provider name, which never belonged on a season either.
    expect(seasonLabel({ title: "Some Arc", indexNumber: 3 }, t)).toBe("Saison 3");
  });

  it("derives a specials season from its number too", () => {
    expect(seasonLabel({ title: "Specials", indexNumber: 0 }, t)).toBe("Saison 0");
  });

  it("keeps the stored title when there is no number to derive from", () => {
    expect(seasonLabel({ title: "Season 3" }, t)).toBe("Season 3");
    expect(seasonLabel({}, t)).toBe("");
  });
});

describe("itemLabel", () => {
  it("only rewrites seasons", () => {
    expect(itemLabel({ title: "Some Movie", type: "MEDIA_ITEM_TYPE_MOVIE" }, t)).toBe("Some Movie");
    expect(itemLabel({ title: "Some Episode", indexNumber: 2, type: "MEDIA_ITEM_TYPE_EPISODE" }, t)).toBe("Some Episode");
    expect(itemLabel({ title: "Season 3", indexNumber: 3, type: SEASON_TYPE }, t)).toBe("Saison 3");
  });

  it("leaves an untyped item alone", () => {
    expect(itemLabel({ title: "Season of the Witch", indexNumber: 3 }, t)).toBe("Season of the Witch");
  });
});

describe("displayTitle", () => {
  it("prefixes the series title the server stamped on episodes and seasons", () => {
    expect(displayTitle({ title: "Some Episode", seriesTitle: "Some Series" }, t)).toBe("Some Series - Some Episode");
    expect(displayTitle({ title: "Season 3", indexNumber: 3, type: SEASON_TYPE, seriesTitle: "Some Series" }, t)).toBe("Some Series - Saison 3");
  });

  it("keeps the bare title when no series is stamped - never a broken prefix", () => {
    expect(displayTitle({ title: "Some Movie" }, t)).toBe("Some Movie");
    expect(displayTitle({ title: "Some Movie", seriesTitle: "" }, t)).toBe("Some Movie");
    expect(displayTitle({}, t)).toBe("");
  });
});
