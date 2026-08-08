import { describe, it, expect } from "vitest";
import { backPressAction, playerCloseTarget, playerExit } from "./playerExit";

describe("backPressAction - hardware back closes what is open first", () => {
  it("dismisses the settings panel instead of exiting the player", () => {
    expect(backPressAction(true)).toBe("close-settings");
  });
  it("exits the player when nothing is open above it", () => {
    expect(backPressAction(false)).toBe("exit");
  });
});

describe("playerExit - same contract as the web player's back button", () => {
  it("pops the history entry whenever there is one to pop", () => {
    expect(playerExit(true, "m1")).toEqual({ action: "back" });
    // Even for an episode: episode changes replace, so back() lands on the
    // launching page, never on the previous episode.
    expect(playerExit(true, "ep1", { type: "MEDIA_ITEM_TYPE_EPISODE", parentId: "s1" }))
      .toEqual({ action: "back" });
  });

  it("with no history (deep link), replaces toward the media page", () => {
    expect(playerExit(false, "m1")).toEqual({ action: "replace", mediaId: "m1" });
  });

  it("an episode exits to its SEASON, the page that lists it", () => {
    expect(playerExit(false, "ep1", { type: "MEDIA_ITEM_TYPE_EPISODE", parentId: "s1" }))
      .toEqual({ action: "replace", mediaId: "s1" });
  });

  it("an episode without a known parent falls back to its own page", () => {
    expect(playerCloseTarget("ep1", { type: "MEDIA_ITEM_TYPE_EPISODE" })).toBe("ep1");
  });

  it("a film exits to its own page", () => {
    expect(playerCloseTarget("m1", { type: "MEDIA_ITEM_TYPE_MOVIE" })).toBe("m1");
  });
});
