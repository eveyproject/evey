import { describe, it, expect } from "vitest";
import { feedItemToCard, feedItemsToCards, type FeedItemLike } from "./feedItemToCard";

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

  it("carries the audience score, and drops the proto zero", () => {
    expect(feedItemToCard({ mediaItemId: "m1", communityRating: 7.4 }).communityRating).toBe(7.4);
    expect(feedItemToCard({ mediaItemId: "m1", communityRating: 0 }).communityRating).toBeUndefined();
  });

  // The popular rail answers with its own shape; it goes through the same
  // projection, so its card gets the placeholder and the score too.
  it("projects a popular item like a feed item", () => {
    const card = feedItemToCard({ mediaItemId: "p1", title: "P", type: "movie", year: 2004, primaryBlurhash: "LKO2", communityRating: 8.1, playCount: 9 } as FeedItemLike);
    expect(card).toEqual({
      id: "p1",
      title: "P",
      type: "movie",
      year: 2004,
      communityRating: 8.1,
      artwork: [{ type: "ARTWORK_TYPE_PRIMARY", blurhash: "LKO2" }],
    });
  });

  it("keeps only items that name a title", () => {
    expect(feedItemsToCards([{ mediaItemId: "a" }, { title: "no id" }, { mediaItemId: "b" }]).map((c) => c.id)).toEqual(["a", "b"]);
    expect(feedItemsToCards(undefined)).toEqual([]);
  });
});
