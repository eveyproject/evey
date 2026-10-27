import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { persistQueryCache, queryCacheScope, restoreQueryCache, type PersistStorage } from "./queryPersist";

function memoryStorage(): PersistStorage & { value: string | null } {
  const s = {
    value: null as string | null,
    read: async () => s.value,
    write: async (v: string) => {
      s.value = v;
    },
    remove: async () => {
      s.value = null;
    },
  };
  return s;
}

const resumeKey = ["/api/v1/playback/resume", { limit: 20 }];

describe("queryPersist", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("writes the first screens' queries once a batch has landed, and the next start reads them back", async () => {
    const storage = memoryStorage();
    const a = new QueryClient();
    persistQueryCache(a, storage, () => "srv|prf");
    a.setQueryData(resumeKey, { items: [{ id: "m1" }] });
    a.setQueryData(["/api/v1/media", { libraryId: "l", "pagination.cursor": "c2" }], { items: [] });
    a.setQueryData(["/api/v1/admin/system"], { version: "x" });
    expect(storage.value).toBeNull();
    await vi.advanceTimersByTimeAsync(2_000);
    const stored = JSON.parse(storage.value!);
    // Only the kept prefixes, and a list's first page only.
    expect(stored.state.queries.map((q: { queryKey: unknown[] }) => q.queryKey[0])).toEqual(["/api/v1/playback/resume"]);

    const b = new QueryClient();
    expect(await restoreQueryCache(b, storage, "srv|prf")).toBe(true);
    expect(b.getQueryData(resumeKey)).toEqual({ items: [{ id: "m1" }] });
  });

  it("another profile or server starts empty, and the stale cache is dropped", async () => {
    const storage = memoryStorage();
    const a = new QueryClient();
    persistQueryCache(a, storage, () => queryCacheScope("srv", "adult"));
    a.setQueryData(resumeKey, { items: [{ id: "m1" }] });
    await vi.advanceTimersByTimeAsync(2_000);

    const kids = new QueryClient();
    expect(await restoreQueryCache(kids, storage, queryCacheScope("srv", "kids"))).toBe(false);
    expect(kids.getQueryData(resumeKey)).toBeUndefined();
    expect(storage.value).toBeNull();
  });

  it("signed out writes nothing, and a corrupt cache is a miss", async () => {
    const storage = memoryStorage();
    const a = new QueryClient();
    persistQueryCache(a, storage, () => queryCacheScope("srv", null));
    a.setQueryData(resumeKey, { items: [] });
    await vi.advanceTimersByTimeAsync(2_000);
    expect(storage.value).toBeNull();

    storage.value = "{not json";
    expect(await restoreQueryCache(new QueryClient(), storage, "srv|prf")).toBe(false);
  });
});
