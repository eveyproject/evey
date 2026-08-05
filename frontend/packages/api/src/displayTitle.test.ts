import { describe, it, expect } from "vitest";
import { displayTitle } from "./displayTitle";

describe("displayTitle", () => {
  it("prefixes the series title the server stamped on episodes and seasons", () => {
    expect(displayTitle({ title: "The Spider and the Fly", seriesTitle: "Lioness" })).toBe("Lioness - The Spider and the Fly");
    expect(displayTitle({ title: "Season 3", seriesTitle: "Lioness" })).toBe("Lioness - Season 3");
  });

  it("keeps the bare title when no series is stamped - never a broken prefix", () => {
    expect(displayTitle({ title: "Heat" })).toBe("Heat");
    expect(displayTitle({ title: "Heat", seriesTitle: "" })).toBe("Heat");
    expect(displayTitle({})).toBe("");
  });
});
