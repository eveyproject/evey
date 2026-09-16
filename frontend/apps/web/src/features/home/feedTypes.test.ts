import { describe, it, expect } from "vitest";
import { isKnownFeedType, isPluginFeedType, pluginIdFromFeedType, pluginFeedType, FEED_LABEL_KEYS } from "./feedTypes";

// Only the sections the web renders are "known" (a legacy or experimental
// type the server still lists must not appear in Customize), every known
// type has a label, and plugin sections round-trip through their prefix.
describe("feedTypes", () => {
  it("knows exactly the renderable sections, each with a label", () => {
    for (const ft of ["resume_playback", "next_episode", "collections", "recommended", "most_popular"]) {
      expect(isKnownFeedType(ft)).toBe(true);
      expect(FEED_LABEL_KEYS[ft as keyof typeof FEED_LABEL_KEYS]).toMatch(/^common\.feeds\.labels\./);
    }
    expect(isKnownFeedType("legacy_row")).toBe(false);
    expect(isKnownFeedType("library:lib-1")).toBe(false);
    expect(isKnownFeedType("plugin:weather")).toBe(false);
  });

  it("round-trips plugin sections through their prefix", () => {
    expect(pluginFeedType("weather")).toBe("plugin:weather");
    expect(isPluginFeedType("plugin:weather")).toBe(true);
    expect(pluginIdFromFeedType("plugin:weather")).toBe("weather");
    expect(isPluginFeedType("library:weather")).toBe(false);
    expect(pluginIdFromFeedType("library:weather")).toBe("");
  });
});
