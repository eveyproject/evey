import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { eveyFetch, setTokenProvider, setTokensUpdater, _resetFetcherState } from "./fetcher";

const fetchMock = vi.fn();

beforeEach(() => {
  fetchMock.mockReset();
  vi.stubGlobal("fetch", fetchMock);
  vi.stubGlobal("window", { dispatchEvent: vi.fn() });
  setTokenProvider(null as unknown as () => string | null);
  setTokensUpdater(null as unknown as (a: string) => void);
  _resetFetcherState();
});

afterEach(() => {
  vi.restoreAllMocks();
});

function mockResponse(status: number, body?: unknown, headers?: Record<string, string>) {
  const hdrs = new Map(Object.entries(headers ?? {}));
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? "OK" : status === 204 ? "No Content" : status === 401 ? "Unauthorized" : "Error",
    headers: { get: (k: string) => hdrs.get(k) ?? null },
    json: body !== undefined ? () => Promise.resolve(body) : () => Promise.reject(new Error("no body")),
  };
}

describe("eveyFetch", () => {
  it("attaches Authorization header when token present", async () => {
    setTokenProvider(() => "my-jwt-token");
    fetchMock.mockResolvedValue(mockResponse(200, { ok: true }));

    await eveyFetch({ url: "/api/test", method: "GET" });

    expect(fetchMock).toHaveBeenCalledWith(
      "/api/test",
      expect.objectContaining({
        headers: expect.objectContaining({ Authorization: "Bearer my-jwt-token" }),
      }),
    );
  });

  it("handles 204 No Content response", async () => {
    fetchMock.mockResolvedValue(mockResponse(204, undefined, {}));

    const result = await eveyFetch({ url: "/api/test", method: "DELETE" });
    expect(result).toBeUndefined();
  });

  it("401 → refresh via cookie → retry succeeds", async () => {
    setTokenProvider(() => "expired-token");
    const updater = vi.fn();
    setTokensUpdater(updater);

    fetchMock
      .mockResolvedValueOnce(mockResponse(401, { message: "token expired" }))
      .mockResolvedValueOnce(mockResponse(200, { accessToken: "new-access" }))
      .mockResolvedValueOnce(mockResponse(200, { data: "ok" }));

    const result = await eveyFetch<{ data: string }>({ url: "/api/test", method: "GET" });

    expect(result).toEqual({ data: "ok" });
    expect(updater).toHaveBeenCalledWith("new-access");
    expect(window.dispatchEvent).not.toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it("401 → refresh fails → dispatches evey:unauthorized and throws", async () => {
    setTokenProvider(() => "expired-token");

    fetchMock
      .mockResolvedValueOnce(mockResponse(401, { message: "token expired" }))
      .mockResolvedValueOnce(mockResponse(401, { message: "refresh invalid" }));

    await expect(eveyFetch({ url: "/api/test", method: "GET" })).rejects.toThrow("Unauthorized");
    expect(window.dispatchEvent).toHaveBeenCalledWith(expect.any(Event));
  });

  it("401 on /auth/refresh → dispatches evey:unauthorized (no retry loop)", async () => {
    setTokenProvider(() => "some-token");

    fetchMock.mockResolvedValue(mockResponse(401, { message: "Unauthorized" }));

    await expect(eveyFetch({ url: "/api/v1/auth/refresh", method: "POST" })).rejects.toThrow();
    expect(window.dispatchEvent).toHaveBeenCalledWith(expect.any(Event));
    // Only called once — no retry (URL contains /auth/refresh so skip refresh attempt)
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("concurrent 401s deduplicate refresh call", async () => {
    setTokenProvider(() => "expired-token");
    const updater = vi.fn();
    setTokensUpdater(updater);

    fetchMock
      .mockResolvedValueOnce(mockResponse(401, { message: "expired" }))
      .mockResolvedValueOnce(mockResponse(401, { message: "expired" }))
      .mockResolvedValueOnce(mockResponse(200, { accessToken: "new-access" }))
      .mockResolvedValueOnce(mockResponse(200, { data: "first" }))
      .mockResolvedValueOnce(mockResponse(200, { data: "second" }));

    const [r1, r2] = await Promise.all([
      eveyFetch<{ data: string }>({ url: "/api/first", method: "GET" }),
      eveyFetch<{ data: string }>({ url: "/api/second", method: "GET" }),
    ]);

    expect(r1).toEqual({ data: "first" });
    expect(r2).toEqual({ data: "second" });
    // refresh endpoint called exactly once despite two concurrent 401s
    const refreshCalls = fetchMock.mock.calls.filter(([url]) => (url as string).includes("/auth/refresh"));
    expect(refreshCalls).toHaveLength(1);
    expect(updater).toHaveBeenCalledTimes(1);
  });
});
