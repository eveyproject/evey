import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { isHomeListQuery, refreshHomeLists } from "./homeFreshness";

describe("refreshHomeLists", () => {
  it("targets the home's lists, not a single title", () => {
    expect(isHomeListQuery(["/api/v1/media", { libraryId: "lib-1" }])).toBe(true);
    expect(isHomeListQuery(["/api/v1/feeds/recommended", { limit: 20 }])).toBe(true);
    expect(isHomeListQuery(["/api/v1/media/abc"])).toBe(false);
    expect(isHomeListQuery(["/api/v1/feeds/config"])).toBe(false);
  });

  it("marks only the lists older than the given age as stale", async () => {
    const qc = new QueryClient();
    const now = 1_000_000;
    qc.setQueryData(["/api/v1/media", { libraryId: "old" }], { items: [] }, { updatedAt: now - 120_000 });
    qc.setQueryData(["/api/v1/media", { libraryId: "fresh" }], { items: [] }, { updatedAt: now - 10_000 });
    qc.setQueryData(["/api/v1/media/abc"], { id: "abc" }, { updatedAt: now - 120_000 });
    await refreshHomeLists(qc, 60_000, now);
    expect(qc.getQueryState(["/api/v1/media", { libraryId: "old" }])?.isInvalidated).toBe(true);
    expect(qc.getQueryState(["/api/v1/media", { libraryId: "fresh" }])?.isInvalidated).toBe(false);
    expect(qc.getQueryState(["/api/v1/media/abc"])?.isInvalidated).toBe(false);
  });
});
