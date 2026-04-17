import { describe, it, expect } from "vitest";
import { typeLabel } from "./typeLabel";

describe("typeLabel", () => {
  it("returns 'Film' for movie type", () => {
    expect(typeLabel("MEDIA_ITEM_TYPE_MOVIE")).toBe("Film");
  });

  it("returns 'Serie' for series type", () => {
    expect(typeLabel("MEDIA_ITEM_TYPE_SERIES")).toBe("Serie");
  });

  it("returns empty string for unknown type", () => {
    expect(typeLabel("MEDIA_ITEM_TYPE_EPISODE")).toBe("");
  });

  it("returns empty string for undefined", () => {
    expect(typeLabel()).toBe("");
  });
});
