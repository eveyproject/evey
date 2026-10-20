import { describe, it, expect } from "vitest";
import { nominalQualityTier, playerCloseTarget, playerExit, resolveStartPositionMs, toEpisodeNavItem } from "./playerUtils";

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

// Regression: a film watched to 100% overnight saved a resume position AT its
// runtime; the sole Play button (no "start over" for a played item) seeked
// <video> to EOF, hls.js buffer-stalled on a black screen, and playback never
// started. A finished title must restart from 0.
describe("resolveStartPositionMs", () => {
  it("restarts a title watched to the very end (played) instead of resuming at EOF", () => {
    expect(
      resolveStartPositionMs(false, { positionMs: "6953154", durationMs: "6953072", played: true }),
    ).toBe(0);
  });

  it("restarts when the position sits within the end epsilon, even if played was not set", () => {
    expect(resolveStartPositionMs(false, { positionMs: "6950000", durationMs: "6953072" })).toBe(0);
  });

  it("resumes genuine mid-title progress", () => {
    expect(resolveStartPositionMs(false, { positionMs: "600000", durationMs: "6953072" })).toBe(600000);
  });

  it("starts at 0 when restart is asked, whatever the saved progress", () => {
    expect(resolveStartPositionMs(true, { positionMs: "600000", durationMs: "6953072" })).toBe(0);
  });

  it("starts at 0 with no progress, a zero position, or an unparseable one", () => {
    expect(resolveStartPositionMs(false, null)).toBe(0);
    expect(resolveStartPositionMs(false, undefined)).toBe(0);
    expect(resolveStartPositionMs(false, { positionMs: "0" })).toBe(0);
    expect(resolveStartPositionMs(false, { positionMs: "abc" })).toBe(0);
  });

  it("resumes when the runtime is unknown and the item is not marked played", () => {
    // No duration to compare against and not played: trust the saved position.
    expect(resolveStartPositionMs(false, { positionMs: "600000" })).toBe(600000);
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

describe("toEpisodeNavItem", () => {
  it("keeps the season number the server stamped on the episode", () => {
    expect(toEpisodeNavItem({ id: "e2", title: "Two", indexNumber: 2, seasonIndexNumber: 3 }, 1)).toEqual({
      id: "e2", title: "Two", indexNumber: 2, seasonIndexNumber: 3,
    });
  });

  it("falls back to the fetched season's number", () => {
    expect(toEpisodeNavItem({ id: "e2", title: "Two", indexNumber: 2 }, 4).seasonIndexNumber).toBe(4);
  });

  it("names an untitled episode by its number", () => {
    expect(toEpisodeNavItem({ id: "e2", indexNumber: 2 }).title).toBe("Episode 2");
  });
});
