import { describe, it, expect } from "vitest";
import { playerTitle } from "./playerTitle";

describe("playerTitle", () => {
  it("leaves a film with its own title", () => {
    expect(playerTitle({ title: "Ava", type: "MEDIA_ITEM_TYPE_MOVIE" }, "ignored")).toBe("Ava");
  });

  it("leads with the show for an episode", () => {
    expect(
      playerTitle({ title: "The Spider and the Fly", type: "MEDIA_ITEM_TYPE_EPISODE" }, "Lioness"),
    ).toBe("Lioness - The Spider and the Fly");
  });

  // The show's name arrives in a second request; until then the bar must read
  // sensibly rather than flash a dangling separator.
  it("shows the episode alone until the show's name has arrived", () => {
    expect(playerTitle({ title: "The Spider and the Fly", type: "MEDIA_ITEM_TYPE_EPISODE" }, undefined)).toBe(
      "The Spider and the Fly",
    );
    expect(playerTitle({ title: "The Spider and the Fly", type: "MEDIA_ITEM_TYPE_EPISODE" }, "   ")).toBe(
      "The Spider and the Fly",
    );
  });

  it("does not repeat an episode named like its show", () => {
    expect(playerTitle({ title: "Lioness", type: "MEDIA_ITEM_TYPE_EPISODE" }, "Lioness")).toBe("Lioness");
  });

  it("renders nothing while the media is still loading", () => {
    expect(playerTitle(undefined, "Lioness")).toBe("");
  });
});
