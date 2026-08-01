import { describe, it, expect } from "vitest";
import {
  SERVER_ALLOWED_PHOTO_WIDTHS,
  PHOTO_GRID_THUMB_WIDTH,
  photoServeUrl,
  buildPhotoListParams,
  hasMorePhotos,
} from "./photoGrid";

describe("PHOTO_GRID_THUMB_WIDTH", () => {
  it("is one of the server's whitelisted photo widths", () => {
    // Guards against ever requesting a grid thumbnail width the server would
    // silently snap to something else (photo_handler.go's photoAllowedWidths).
    expect(SERVER_ALLOWED_PHOTO_WIDTHS).toContain(PHOTO_GRID_THUMB_WIDTH);
  });
});

describe("photoServeUrl", () => {
  it("builds a plain serve URL with no options", () => {
    expect(photoServeUrl("http://srv", "p1")).toBe("http://srv/api/v1/photos/p1/serve");
  });

  it("adds the width query param for grid thumbnails", () => {
    expect(photoServeUrl("http://srv", "p1", { width: PHOTO_GRID_THUMB_WIDTH })).toBe(
      "http://srv/api/v1/photos/p1/serve?w=400",
    );
  });

  it("adds both width and format", () => {
    expect(photoServeUrl("http://srv", "p1", { width: 400, format: "webp" })).toBe(
      "http://srv/api/v1/photos/p1/serve?w=400&fmt=webp",
    );
  });

  it("omits the width entirely for the fullscreen (original-resolution) request", () => {
    expect(photoServeUrl("http://srv", "p1", {})).toBe("http://srv/api/v1/photos/p1/serve");
  });
});

describe("buildPhotoListParams", () => {
  it("omits album when absent, page 1 takes the offset(0) path", () => {
    expect(buildPhotoListParams({ libraryId: "lib1", limit: 60 })).toEqual({
      libraryId: "lib1",
      limit: 60,
      offset: 0,
    });
  });

  it("includes album when set", () => {
    expect(buildPhotoListParams({ libraryId: "lib1", album: "Vacation", limit: 60 })).toEqual({
      libraryId: "lib1",
      limit: 60,
      offset: 0,
      album: "Vacation",
    });
  });

  it("sends the keyset cursor instead of offset once a page carries one", () => {
    expect(buildPhotoListParams({ libraryId: "lib1", limit: 60, cursor: "opaque-cursor-1" })).toEqual({
      libraryId: "lib1",
      limit: 60,
      cursor: "opaque-cursor-1",
    });
  });
});

describe("hasMorePhotos", () => {
  // RED PROOF: the signal this replaced (`total > 0 && loadedCount < total`)
  // truncates every keyset page. A keyset page always reports `total: 0` by
  // server design (V1ListPhotosResponse.total's doc), so the old check
  // "total > 0" fails immediately even though nextCursor says there is more.
  it("documents the bug the old total-based signal had on a keyset page", () => {
    const oldHasMorePhotos = (loadedCount: number, total: number): boolean => total > 0 && loadedCount < total;
    const total = 0; // what a keyset (cursor) page actually returns
    const nextCursor = "opaque-cursor-2"; // ...while clearly saying there is more
    const loadedCount = 60;
    expect(oldHasMorePhotos(loadedCount, total)).toBe(false); // BUG: reports "done", grid gets truncated
    expect(nextCursor).toBeTruthy(); // yet the server says otherwise
  });

  it("is true while the last page carries a next cursor", () => {
    expect(hasMorePhotos("opaque-cursor-2", 60, 60)).toBe(true);
  });

  it("is false once nextCursor comes back empty", () => {
    expect(hasMorePhotos(undefined, 60, 60)).toBe(false);
    expect(hasMorePhotos("", 60, 60)).toBe(false);
  });

  it("falls back to a short page as 'done', even if a cursor was somehow still set", () => {
    expect(hasMorePhotos("opaque-cursor-3", 12, 60)).toBe(false);
  });
});
