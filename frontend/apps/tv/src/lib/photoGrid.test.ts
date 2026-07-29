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
  it("omits album when absent", () => {
    expect(buildPhotoListParams({ libraryId: "lib1", limit: 60, offset: 0 })).toEqual({
      libraryId: "lib1",
      limit: 60,
      offset: 0,
    });
  });

  it("includes album when set", () => {
    expect(buildPhotoListParams({ libraryId: "lib1", album: "Vacation", limit: 60, offset: 60 })).toEqual({
      libraryId: "lib1",
      limit: 60,
      offset: 60,
      album: "Vacation",
    });
  });
});

describe("hasMorePhotos", () => {
  it("is true while loaded is below total", () => {
    expect(hasMorePhotos(60, 200)).toBe(true);
  });

  it("is false once everything is loaded", () => {
    expect(hasMorePhotos(200, 200)).toBe(false);
  });

  it("is false for an empty/unknown total", () => {
    expect(hasMorePhotos(0, 0)).toBe(false);
  });
});
