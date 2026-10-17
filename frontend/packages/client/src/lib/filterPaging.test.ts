import { describe, it, expect } from "vitest";
import { nextFilterOffset } from "./filterPaging";

describe("nextFilterOffset", () => {
  it("follows the server past a page the parental pass shortened", () => {
    expect(nextFilterOffset({ items: [1], total: 3, nextOffset: 2, hasMore: true }, 0, 2)).toBe(2);
  });

  it("stops when the server says there is nothing more", () => {
    expect(nextFilterOffset({ items: [1], total: 3, nextOffset: 3, hasMore: false }, 2, 2)).toBeUndefined();
  });

  // proto3 JSON omits a false bool: an absent has_more next to a next_offset
  // is a server that knows the field and answered false.
  it("reads an omitted has_more as false when next_offset is present", () => {
    expect(nextFilterOffset({ items: [1, 2], total: 4, nextOffset: 4 }, 2, 2)).toBeUndefined();
  });

  describe("server without next_offset", () => {
    it("stops on a short page", () => {
      expect(nextFilterOffset({ items: [1, 2, 3, 4, 5], total: 100 }, 5, 10)).toBeUndefined();
    });
    it("stops when every row is fetched", () => {
      expect(nextFilterOffset({ items: Array(10).fill(0), total: 100 }, 90, 10)).toBeUndefined();
    });
    it("continues after a full page", () => {
      expect(nextFilterOffset({ items: Array(30).fill(0), total: 100 }, 0, 30)).toBe(30);
    });
  });
});
