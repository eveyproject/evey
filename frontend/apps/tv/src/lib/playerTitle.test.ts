import { describe, it, expect } from "vitest";
import { playerTitle } from "./playerTitle";

describe("playerTitle", () => {
  it("leaves a film with its own title", () => {
    expect(playerTitle({ title: "Ava", type: "MEDIA_ITEM_TYPE_MOVIE" }, "ignored")).toBe("Ava");
  });

  it("leads with the show for an episode", () => {
    expect(
      playerTitle({ title: "Some Episode", type: "MEDIA_ITEM_TYPE_EPISODE" }, "Some Series"),
    ).toBe("Some Series - Some Episode");
  });

  // The show's name arrives in a second request; until then the bar must read
  // sensibly rather than flash a dangling separator.
  it("shows the episode alone until the show's name has arrived", () => {
    expect(playerTitle({ title: "Some Episode", type: "MEDIA_ITEM_TYPE_EPISODE" }, undefined)).toBe(
      "Some Episode",
    );
    expect(playerTitle({ title: "Some Episode", type: "MEDIA_ITEM_TYPE_EPISODE" }, "   ")).toBe(
      "Some Episode",
    );
  });

  it("does not repeat an episode named like its show", () => {
    expect(playerTitle({ title: "Some Series", type: "MEDIA_ITEM_TYPE_EPISODE" }, "Some Series")).toBe("Some Series");
  });

  it("renders nothing while the media is still loading", () => {
    expect(playerTitle(undefined, "Some Series")).toBe("");
  });
});
