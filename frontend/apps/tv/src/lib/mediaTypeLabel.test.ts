import { describe, it, expect } from "vitest";
import { mediaTypeLabelKey } from "./mediaTypeLabel";

describe("mediaTypeLabelKey", () => {
  it("maps every type the clients render", () => {
    expect(mediaTypeLabelKey("MEDIA_ITEM_TYPE_MOVIE")).toBe("media.itemType.movie");
    expect(mediaTypeLabelKey("MEDIA_ITEM_TYPE_SERIES")).toBe("media.itemType.series");
    expect(mediaTypeLabelKey("MEDIA_ITEM_TYPE_EPISODE")).toBe("media.itemType.episode");
    expect(mediaTypeLabelKey("MEDIA_ITEM_TYPE_TRACK")).toBe("media.itemType.track");
  });

  // Showing nothing beats showing "MEDIA_ITEM_TYPE_WHATEVER" or a raw key.
  it("returns nothing for a type it does not know", () => {
    expect(mediaTypeLabelKey("MEDIA_ITEM_TYPE_FUTURE")).toBeNull();
  });

  it("returns nothing when the server sent no type", () => {
    expect(mediaTypeLabelKey(undefined)).toBeNull();
    expect(mediaTypeLabelKey("")).toBeNull();
  });
});
