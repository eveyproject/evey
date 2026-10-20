import { describe, it, expect } from "vitest";
import { downloadLabel } from "./downloadLabel";

describe("downloadLabel", () => {
  it("names the series first and puts the code with the episode title", () => {
    expect(downloadLabel({ title: "Some Episode", seriesTitle: "Some Series", seasonNumber: 1, episodeNumber: 5 })).toEqual({
      primary: "Some Series",
      secondary: "S01E05 · Some Episode",
    });
  });

  it("keeps a film on one line", () => {
    expect(downloadLabel({ title: "Some Movie" })).toEqual({ primary: "Some Movie" });
  });

  // Entries downloaded before the display capture carry no series/numbers:
  // they must render exactly as they used to, never a dangling separator.
  it("falls back to the bare title for a pre-capture entry", () => {
    expect(downloadLabel({ title: "Some Episode" })).toEqual({ primary: "Some Episode" });
  });

  it("handles a partial code without inventing the missing half", () => {
    expect(downloadLabel({ title: "Some Episode", seriesTitle: "Some Series", episodeNumber: 5 })).toEqual({
      primary: "Some Series",
      secondary: "E05 · Some Episode",
    });
    expect(downloadLabel({ title: "Some Episode", seriesTitle: "Some Series" })).toEqual({
      primary: "Some Series",
      secondary: "Some Episode",
    });
  });

  it("ignores zero numbers rather than showing S0E0", () => {
    expect(downloadLabel({ title: "Some Movie", seasonNumber: 0, episodeNumber: 0 })).toEqual({ primary: "Some Movie" });
  });
});
