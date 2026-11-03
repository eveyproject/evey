import { describe, it, expect } from "vitest";
import { feedItemToCard, feedItemsToCards } from "./feedItemToCard";

describe("feedItemToCard", () => {
  it("carries id, title, type, year and the primary blurhash onto the card", () => {
    expect(feedItemToCard({ mediaItemId: "m1", title: "T", type: "MEDIA_ITEM_TYPE_MOVIE", year: 2001, primaryBlurhash: "LKO2" })).toEqual({
      id: "m1",
      title: "T",
      type: "MEDIA_ITEM_TYPE_MOVIE",
      year: 2001,
      artwork: [{ type: "ARTWORK_TYPE_PRIMARY", blurhash: "LKO2" }],
    });
  });

  it("drops the proto zero year and an empty blurhash", () => {
    const card = feedItemToCard({ mediaItemId: "m1", title: "T", year: 0, primaryBlurhash: "" });
    expect(card.year).toBeUndefined();
    expect(card.artwork).toBeUndefined();
  });

  it("keeps only items that name a title", () => {
    expect(feedItemsToCards([{ mediaItemId: "a" }, { title: "no id" }, { mediaItemId: "b" }]).map((c) => c.id)).toEqual(["a", "b"]);
    expect(feedItemsToCards(undefined)).toEqual([]);
  });
});
