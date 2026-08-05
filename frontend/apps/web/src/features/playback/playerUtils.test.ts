import { describe, it, expect } from "vitest";
import { nominalQualityTier, playerCloseTarget, playerExit } from "./playerUtils";

describe("nominalQualityTier", () => {
  it("rounds a widescreen height to the nearest standard tier", () => {
    expect(nominalQualityTier(798)).toBe(720); // 2.40:1 "1080p" rung → 720p (closer)
    expect(nominalQualityTier(532)).toBe(480);
    expect(nominalQualityTier(1040)).toBe(1080);
  });

  it("keeps exact standard heights", () => {
    expect(nominalQualityTier(720)).toBe(720);
    expect(nominalQualityTier(1080)).toBe(1080);
    expect(nominalQualityTier(2160)).toBe(2160);
  });

  it("returns 0 for non-positive heights", () => {
    expect(nominalQualityTier(0)).toBe(0);
    expect(nominalQualityTier(-5)).toBe(0);
  });
});

// Regression: closing the player NAVIGATED FORWARD to the media page, which
// left the player's own entry sitting in the back stack. The media page's own
// "Retour" button then walked straight back into it and autoplayed the episode
// again (reported on a season page after watching episode 2).
describe("playerExit", () => {
  const episode = { type: "MEDIA_ITEM_TYPE_EPISODE", parentId: "season-2" };

  it("pops the player's own entry when there is one, instead of pushing a page on top of it", () => {
    expect(playerExit(true, "ep-9", episode)).toEqual({ action: "back" });
  });

  it("navigates to the season - REPLACING the player entry - on a deep link with nothing to pop", () => {
    expect(playerExit(false, "ep-9", episode)).toEqual({ action: "replace", mediaId: "season-2" });
  });

  it("sends a film with no history back to its own page", () => {
    expect(playerExit(false, "movie-1", { type: "MEDIA_ITEM_TYPE_MOVIE" })).toEqual({
      action: "replace",
      mediaId: "movie-1",
    });
  });

  it("never answers with a plain forward navigation - that is the bug it exists to prevent", () => {
    for (const canGoBack of [true, false]) {
      const exit = playerExit(canGoBack, "ep-9", episode);
      expect(exit.action === "back" || exit.action === "replace").toBe(true);
    }
  });
});

describe("playerCloseTarget", () => {
  it("sends an episode back to its season, not to the episode itself", () => {
    expect(
      playerCloseTarget("ep-9", { type: "MEDIA_ITEM_TYPE_EPISODE", parentId: "season-2" }),
    ).toBe("season-2");
  });

  it("sends a film back to its own page", () => {
    expect(playerCloseTarget("movie-1", { type: "MEDIA_ITEM_TYPE_MOVIE" })).toBe("movie-1");
  });

  it("falls back to the played id when the item is unknown or has no parent", () => {
    expect(playerCloseTarget("ep-9", undefined)).toBe("ep-9");
    expect(playerCloseTarget("ep-9", { type: "MEDIA_ITEM_TYPE_EPISODE" })).toBe("ep-9");
  });
});
