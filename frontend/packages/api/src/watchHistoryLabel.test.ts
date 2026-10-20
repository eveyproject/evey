import { describe, expect, it } from "vitest";
import { watchHistoryLabel } from "./watchHistoryLabel";

describe("watchHistoryLabel", () => {
  it("returns the plain title for a movie", () => {
    expect(watchHistoryLabel({ title: "Blade Runner", mediaType: "movie" })).toBe("Blade Runner");
  });

  it("builds series - SxxExx - title for an episode", () => {
    expect(
      watchHistoryLabel({
        mediaType: "episode",
        seriesTitle: "GIGN",
        parentIndexNumber: 1,
        indexNumber: 5,
        title: "Prise d'otages",
      }),
    ).toBe("GIGN · S01E05 · Prise d'otages");
  });

  it("falls back to E<n> when the season number is missing", () => {
    expect(watchHistoryLabel({ mediaType: "episode", seriesTitle: "GIGN", indexNumber: 5 })).toBe("GIGN · E05");
  });

  // A resolved-but-partial episode must still beat the raw id: the whole point
  // of the helper is that no screen ever prints a UUID at the user.
  it("uses whatever episode part resolved, even without a series title", () => {
    expect(watchHistoryLabel({ mediaType: "episode", parentIndexNumber: 2, indexNumber: 3, title: "Pilot" })).toBe(
      "S02E03 · Pilot",
    );
  });

  it("returns null when the server could not resolve the item, so the caller localizes the fallback", () => {
    expect(watchHistoryLabel({ mediaItemId: "0f2c1a3e-dead-beef", mediaType: "episode" })).toBeNull();
    expect(watchHistoryLabel({ mediaItemId: "0f2c1a3e-dead-beef" })).toBeNull();
    expect(watchHistoryLabel({ title: "   " })).toBeNull();
  });
});
