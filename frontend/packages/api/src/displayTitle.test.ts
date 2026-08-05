import { describe, it, expect } from "vitest";
import { displayTitle } from "./displayTitle";

describe("displayTitle", () => {
  it("prefixes the series title the server stamped on episodes and seasons", () => {
    expect(displayTitle({ title: "Some Episode", seriesTitle: "Some Series" })).toBe("Some Series - Some Episode");
    expect(displayTitle({ title: "Season 3", seriesTitle: "Some Series" })).toBe("Some Series - Season 3");
  });

  it("keeps the bare title when no series is stamped - never a broken prefix", () => {
    expect(displayTitle({ title: "Some Movie" })).toBe("Some Movie");
    expect(displayTitle({ title: "Some Movie", seriesTitle: "" })).toBe("Some Movie");
    expect(displayTitle({})).toBe("");
  });
});
