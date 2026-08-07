import { describe, it, expect } from "vitest";
import { hasMoreFromCursor } from "./pagination";

// Payloads copied verbatim from the running server (GET /api/v1/media,
// 2026-07-28), not invented. That matters: the bug this guards against stayed
// invisible because the mobile screen test mocked `totalPages: 1`, a value the
// server has never once sent, so a green suite sat on top of a list frozen at
// 30 items in every library.
describe("hasMoreFromCursor", () => {
  it("keeps paging on the real Séries payload (8076 items, totalPages 0)", () => {
    expect(
      hasMoreFromCursor({
        page: 1,
        pageSize: 30,
        totalItems: 8076,
        totalPages: 0,
        nextCursor: "eyJmIjoidGl0bGUiLCJ2IjoiMDAwMDEzIDAwMCJ9",
      } as never),
    ).toBe(true);
  });

  it("stops on a library that fits in one page (Documentaires: 11 items, empty cursor)", () => {
    expect(
      hasMoreFromCursor({ page: 1, pageSize: 30, totalItems: 11, totalPages: 0, nextCursor: "" } as never),
    ).toBe(false);
  });

  it("stops on an empty library (Musique: 0 items)", () => {
    expect(
      hasMoreFromCursor({ page: 1, pageSize: 30, totalItems: 0, totalPages: 0, nextCursor: "" } as never),
    ).toBe(false);
  });

  it("stops rather than looping when the server sends no pagination block", () => {
    expect(hasMoreFromCursor(undefined)).toBe(false);
    expect(hasMoreFromCursor({})).toBe(false);
  });

  // RED PROOF: the signal this replaced (`total > 0 && loadedCount < total`)
  // truncates every keyset page, on any cursor-paginated list (ListPhotos
  // included). A keyset page always reports `total: 0` by server design, so
  // the old check "total > 0" fails immediately even though nextCursor says
  // there is more.
  it("documents the bug the old total-based signal had on a keyset ListPhotos page", () => {
    const oldHasMore = (loadedCount: number, total: number): boolean => total > 0 && loadedCount < total;
    const total = 0; // what a keyset (cursor) page actually returns
    const nextCursor = "opaque-cursor-2"; // ...while clearly saying there is more
    const loadedCount = 60;
    expect(oldHasMore(loadedCount, total)).toBe(false); // BUG: reports "done", grid gets truncated
    expect(hasMoreFromCursor({ nextCursor })).toBe(true); // the correct signal disagrees
  });
});
