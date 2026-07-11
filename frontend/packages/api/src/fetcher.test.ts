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
  const bodyStr = body !== undefined ? JSON.stringify(body) : "";
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? "OK" : status === 204 ? "No Content" : status === 401 ? "Unauthorized" : "Error",
    headers: { get: (k: string) => hdrs.get(k) ?? null },
    json: body !== undefined ? () => Promise.resolve(body) : () => Promise.reject(new Error("no body")),
    text: () => Promise.resolve(bodyStr),
  };
}

function mockNDJSONResponse(lines: unknown[]) {
  const body = lines.map((l) => JSON.stringify(l)).join("\n") + "\n";
  return {
    ok: true,
    status: 200,
    statusText: "OK",
    headers: { get: () => null },
    json: () => Promise.reject(new Error("use text() for NDJSON")),
    text: () => Promise.resolve(body),
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
    // Only called once - no retry (URL contains /auth/refresh so skip refresh attempt)
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

  // --- Pre-flight JWT expiry refresh (proactive) ----------------------------
  // mkJwt builds a structurally-valid JWT with a custom exp claim so the
  // pre-flight expiry check (`_ensureFreshToken` → `isJwtExpired`) actually
  // engages. Signature is bogus - the test only exercises decode logic.
  function mkJwt(expSecondsFromNow: number): string {
    const header = btoa(JSON.stringify({ alg: "RS256", typ: "JWT" })).replace(/=/g, "");
    const exp    = Math.floor(Date.now() / 1000) + expSecondsFromNow;
    const payload = btoa(JSON.stringify({ exp, sub: "test" })).replace(/=/g, "");
    return `${header}.${payload}.signature`;
  }

  it("refreshes upfront when the cached token is expired (no 401 round-trip)", async () => {
    setTokenProvider(() => mkJwt(-30)); // already 30s past expiry
    const updater = vi.fn();
    setTokensUpdater(updater);

    fetchMock
      .mockResolvedValueOnce(mockResponse(200, { accessToken: "fresh" })) // /auth/refresh
      .mockResolvedValueOnce(mockResponse(200, { data: "ok" }));          // actual call

    const result = await eveyFetch<{ data: string }>({ url: "/api/test", method: "GET" });

    expect(result).toEqual({ data: "ok" });
    expect(updater).toHaveBeenCalledWith("fresh");
    // Exactly 2 fetches - no 401 detour, refresh happens upfront.
    expect(fetchMock).toHaveBeenCalledTimes(2);
    // First call must be the refresh, second the actual API call.
    expect(fetchMock.mock.calls[0][0]).toContain("/auth/refresh");
    // The actual call uses the new token, not the expired one.
    expect(fetchMock.mock.calls[1][1].headers.Authorization).toBe("Bearer fresh");
  });

  it("does NOT refresh when the cached token is still valid", async () => {
    setTokenProvider(() => mkJwt(300)); // 5min in the future
    fetchMock.mockResolvedValueOnce(mockResponse(200, { data: "ok" }));

    const result = await eveyFetch<{ data: string }>({ url: "/api/test", method: "GET" });

    expect(result).toEqual({ data: "ok" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).not.toContain("/auth/refresh");
  });

  it("does NOT pre-flight refresh before hitting /auth/refresh itself (no double rotation)", async () => {
    setTokenProvider(() => mkJwt(-30)); // expired - would normally pre-refresh
    fetchMock.mockResolvedValueOnce(mockResponse(200, { accessToken: "rotated" }));

    await eveyFetch<{ accessToken: string }>({ url: "/api/v1/auth/refresh", method: "POST" });

    // Exactly ONE fetch: the refresh POST. Without the guard, _ensureFreshToken
    // would rotate the cookie once here, then this POST rotates it again.
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toContain("/auth/refresh");
  });

  it("concurrent calls with an expired token share a single refresh", async () => {
    setTokenProvider(() => mkJwt(-1));
    const updater = vi.fn();
    setTokensUpdater(updater);

    fetchMock
      .mockResolvedValueOnce(mockResponse(200, { accessToken: "fresh" })) // single refresh
      .mockResolvedValue(mockResponse(200, { data: "ok" }));

    const [a, b, c] = await Promise.all([
      eveyFetch<{ data: string }>({ url: "/api/a", method: "GET" }),
      eveyFetch<{ data: string }>({ url: "/api/b", method: "GET" }),
      eveyFetch<{ data: string }>({ url: "/api/c", method: "GET" }),
    ]);

    expect(a).toEqual({ data: "ok" });
    expect(b).toEqual({ data: "ok" });
    expect(c).toEqual({ data: "ok" });

    const refreshCalls = fetchMock.mock.calls.filter(([url]) => (url as string).includes("/auth/refresh"));
    expect(refreshCalls).toHaveLength(1);
    expect(updater).toHaveBeenCalledTimes(1);
  });

  it("parses NDJSON streaming response (last line)", async () => {
    fetchMock.mockResolvedValueOnce(
      mockNDJSONResponse([
        { result: { phase: "SCANNING", percent: 50 } },
        { result: { phase: "COMPLETE", percent: 100, totalItems: 42 } },
      ]),
    );

    const result = await eveyFetch<{ result: { phase: string; percent: number; totalItems?: number } }>({
      url: "/api/v1/scan",
      method: "POST",
    });

    expect(result?.result.phase).toBe("COMPLETE");
    expect(result?.result.totalItems).toBe(42);
  });

  it("returns undefined for empty NDJSON response", async () => {
    fetchMock.mockResolvedValueOnce({
      ok: true,
      status: 200,
      statusText: "OK",
      headers: { get: () => null },
      text: () => Promise.resolve("\n\n"),
    });

    const result = await eveyFetch({ url: "/api/v1/empty", method: "GET" });
    expect(result).toBeUndefined();
  });
});
