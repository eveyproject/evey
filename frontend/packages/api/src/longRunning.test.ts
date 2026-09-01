import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { clearTranscodeCacheUnbounded, deleteLibraryUnbounded } from "./longRunning";
import { NO_TIMEOUT, _resetFetcherState, setTokenProvider, setTokensUpdater } from "./fetcher";

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("window", { dispatchEvent: vi.fn() });
  setTokenProvider(null as unknown as () => string | null);
  setTokensUpdater(null as unknown as (a: string) => void);
  _resetFetcherState();
  fetchMock.mockResolvedValue({ ok: true, status: 200, statusText: "OK", headers: { get: () => null }, json: () => Promise.resolve({}), text: () => Promise.resolve("{}"), clone() { return this; } });
});
afterEach(() => { vi.restoreAllMocks(); });

// These mirror the generated clients' URL and method exactly, plus the
// never-firing signal: a drift here would silently hit the wrong route.
describe("long-running RPC wrappers", () => {
  it("clears the transcode cache with the generated route and no timeout", async () => {
    await clearTranscodeCacheUnbounded();
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/admin/transcode-cache", expect.objectContaining({ method: "DELETE", signal: NO_TIMEOUT }));
  });

  it("deletes a library with the generated route, an encoded id and no timeout", async () => {
    await deleteLibraryUnbounded("lib-a/b");
    expect(fetchMock).toHaveBeenCalledWith("/api/v1/libraries/lib-a%2Fb", expect.objectContaining({ method: "DELETE", signal: NO_TIMEOUT }));
  });
});
