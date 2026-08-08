import { describe, it, expect } from "vitest";
import { buildTrackQueryUrl, TRACKS_PAGE_SIZE } from "./musicTrackQuery";

function params(url: string): URLSearchParams {
  return new URLSearchParams(url.split("?")[1]);
}

describe("buildTrackQueryUrl", () => {
  // The defect this exists for: a flat limit=200 made track 201 of a library
  // unreachable, silently.
  it("asks for a page size the server will honour", () => {
    const p = params(buildTrackQueryUrl({ libraryID: "lib-1" }));
    const limit = Number(p.get("limit"));
    expect(limit).toBe(TRACKS_PAGE_SIZE);
    expect(limit).toBeGreaterThan(0);
    expect(limit).toBeLessThanOrEqual(500);
  });

  it("takes the offset path on page 1 - the only one that computes an exact total", () => {
    const p = params(buildTrackQueryUrl({ libraryID: "lib-1" }));
    expect(p.get("offset")).toBe("0");
    expect(p.get("cursor")).toBeNull();
  });

  it("carries the cursor - and no offset - on the pages after it", () => {
    const p = params(buildTrackQueryUrl({ libraryID: "lib-1", cursor: "cur-1" }));
    expect(p.get("cursor")).toBe("cur-1");
    // Sending both hides which path the server is actually taking.
    expect(p.get("offset")).toBeNull();
  });

  it("escapes a cursor that contains URL-significant characters", () => {
    const raw = "a+b/c=d&e";
    const url = buildTrackQueryUrl({ libraryID: "lib-1", cursor: raw });
    expect(params(url).get("cursor")).toBe(raw);
  });

  // Album mode: the server returns the album's full track set and ignores
  // limit/search, so there is nothing to page through.
  it("does not page in album mode", () => {
    const p = params(buildTrackQueryUrl({ libraryID: "lib-1", albumID: "alb 1", cursor: "cur-1" }));
    expect(p.get("albumId")).toBe("alb 1");
    expect(p.get("cursor")).toBeNull();
    expect(p.get("offset")).toBeNull();
  });

  it("ignores a search term in album mode, the way the server does", () => {
    const p = params(buildTrackQueryUrl({ libraryID: "lib-1", albumID: "alb-1", search: "blue" }));
    expect(p.get("search")).toBeNull();
    expect(p.get("albumId")).toBe("alb-1");
  });

  // Search mode: answered by the full-text index, which has no keyset. Sending
  // a cursor would serve page 1 from the search engine and page 2 from SQL.
  it("does not page in search mode", () => {
    const p = params(buildTrackQueryUrl({ libraryID: "lib-1", search: "blue", cursor: "cur-1" }));
    expect(p.get("search")).toBe("blue");
    expect(p.get("cursor")).toBeNull();
    expect(p.get("limit")).toBeNull();
  });

  it("treats a whitespace-only search as no search at all", () => {
    const p = params(buildTrackQueryUrl({ libraryID: "lib-1", search: "   " }));
    expect(p.get("search")).toBeNull();
    expect(p.get("offset")).toBe("0");
  });
});
