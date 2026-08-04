import { describe, it, expect } from "vitest";
import { moveItem } from "./collectionReorder";

describe("moveItem", () => {
  const ids = ["a", "b", "c"];

  it("swaps with the previous member", () => {
    expect(moveItem(ids, "c", "up")).toEqual(["a", "c", "b"]);
  });

  it("swaps with the next member", () => {
    expect(moveItem(ids, "a", "down")).toEqual(["b", "a", "c"]);
  });

  it("leaves the order untouched at either edge", () => {
    expect(moveItem(ids, "a", "up")).toEqual(ids);
    expect(moveItem(ids, "c", "down")).toEqual(ids);
  });

  it("leaves the order untouched for a member it does not hold", () => {
    expect(moveItem(ids, "zz", "up")).toEqual(ids);
  });

  it("never mutates the input", () => {
    const original = [...ids];
    moveItem(ids, "a", "down");
    expect(ids).toEqual(original);
  });
});
