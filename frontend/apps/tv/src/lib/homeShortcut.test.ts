import { describe, it, expect } from "vitest";
import { isDeepScreenPath } from "./homeShortcut";

describe("isDeepScreenPath", () => {
  it.each(["/", "", "/favorites", "/library", "/music", "/requests", "/search", "/settings"])(
    "is false for the tab root %s - the sidebar already offers Home there",
    (path) => {
      expect(isDeepScreenPath(path)).toBe(false);
    },
  );

  it.each([
    "/media/123",
    "/person/45",
    "/library/9",
    "/collections",
    "/collections/3",
    "/music/album/1",
    "/music/artist/2",
    "/playlists",
    "/playlists/9",
    "/history",
    "/notifications",
    "/manage-servers",
    "/settings/playback",
  ])("is true for the deep screen %s - it renders with no sidebar", (path) => {
    expect(isDeepScreenPath(path)).toBe(true);
  });

  it("is false inside the player - it owns its own exit contract (playerExit.ts), no shortcut should compete with it", () => {
    expect(isDeepScreenPath("/player")).toBe(false);
    expect(isDeepScreenPath("/player/123")).toBe(false);
  });
});
