import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { QueryClient, QueryObserver } from "@tanstack/react-query";
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

  it("a restored screen shows at once and is fetched again, however recent the stored copy", async () => {
    // A reload a minute after the last visit restored the home's rows as
    // fresh: nothing was asked again until the stale time ran out, so a
    // plugin installed in between, or a title added, stayed invisible.
    const storage = memoryStorage();
    const a = new QueryClient();
    persistQueryCache(a, storage, () => "srv|prf");
    a.setQueryData(resumeKey, { items: [{ id: "m1" }] });
    await vi.advanceTimersByTimeAsync(2_000);

    const b = new QueryClient({ defaultOptions: { queries: { staleTime: 5 * 60 * 1000 } } });
    expect(await restoreQueryCache(b, storage, "srv|prf")).toBe(true);
    const fetchAgain = vi.fn().mockResolvedValue({ items: [{ id: "m2" }] });
    const observer = new QueryObserver(b, { queryKey: resumeKey, queryFn: fetchAgain });
    expect(observer.getOptimisticResult(observer.options).data).toEqual({ items: [{ id: "m1" }] });
    const stop = observer.subscribe(() => {});
    await vi.advanceTimersByTimeAsync(0);
    expect(fetchAgain).toHaveBeenCalledTimes(1);
    expect(b.getQueryData(resumeKey)).toEqual({ items: [{ id: "m2" }] });
    stop();
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
