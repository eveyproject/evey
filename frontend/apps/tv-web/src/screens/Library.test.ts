import { describe, expect, it } from "vitest";
import { shouldFetchNextPage, PREFETCH_FROM_END } from "./Library";

describe("shouldFetchNextPage", () => {
  it("fetches only from the last rows, once, and only when a page remains", () => {
    expect(shouldFetchNextPage(0, 30, true, false)).toBe(false);
    expect(shouldFetchNextPage(30 - PREFETCH_FROM_END, 30, true, false)).toBe(true);
    expect(shouldFetchNextPage(29, 30, true, true)).toBe(false);
    expect(shouldFetchNextPage(29, 30, false, false)).toBe(false);
  });
});
