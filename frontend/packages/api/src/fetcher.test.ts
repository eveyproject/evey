import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { eveyFetch, setTokenProvider, setTokensUpdater, setUnauthorizedHandler, resetRefreshFailure, _resetFetcherState } from "./fetcher";

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
  const res = {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? "OK" : status === 204 ? "No Content" : status === 401 ? "Unauthorized" : "Error",
    headers: { get: (k: string) => hdrs.get(k) ?? null },
    json: body !== undefined ? () => Promise.resolve(body) : () => Promise.reject(new Error("no body")),
    text: () => Promise.resolve(bodyStr),
    // The error path reads the body through response.clone() so it never
    // consumes the stream the caller will read. Without clone() here the mock
    // silently loses every body-carried field (message, errorCode,
    // retryAfterSeconds) and the tests that check them pass for the wrong
    // reason - or fail while the real code is fine.
    clone: () => res,
  };
  return res;
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

// The error branch of fetchWithRefresh is what every screen in the product
// reads when a call fails - and it was never tested on a real non-2xx
// Response. The two fields it lifts are the two incidents its own comments
// name: a 429 whose wait was invisible so the user retried straight into the
// wall (2026-07-29), and a remote-access block that read as a bad password
// for two days of diagnosis (also 2026-07-29). Both are carried on the thrown
// Error, and `apiErrorMessage` now depends on `errorCode` being there.
describe("eveyFetch: what a failed call tells the UI", () => {
  async function catchError(promise: Promise<unknown>) {
    try {
      await promise;
    } catch (e) {
      return e as Error & { httpStatus?: number; retryAfterSeconds?: number; errorCode?: string };
    }
    throw new Error("expected the call to reject");
  }

  it("prefers the Retry-After HEADER over the body (a proxy may rewrite the body, not the header)", async () => {
    fetchMock.mockResolvedValueOnce(mockResponse(429, { message: "slow down", retryAfterSeconds: 5 }, { "Retry-After": "42" }));
    const err = await catchError(eveyFetch({ url: "/api/v1/x", method: "GET" }));
    expect(err.retryAfterSeconds).toBe(42);
    expect(err.httpStatus).toBe(429);
  });

  it("falls back to the body delay when a proxy stripped the header", async () => {
    fetchMock.mockResolvedValueOnce(mockResponse(429, { message: "slow down", retryAfterSeconds: 7 }));
    const err = await catchError(eveyFetch({ url: "/api/v1/x", method: "GET" }));
    expect(err.retryAfterSeconds).toBe(7);
  });

  it("carries a STRING error code, and ignores the numeric `code` a grpc-gateway error puts there", async () => {
    fetchMock.mockResolvedValueOnce(mockResponse(403, { message: "nope", code: "errors.auth.invalid_pin" }));
    const withCode = await catchError(eveyFetch({ url: "/api/v1/x", method: "GET" }));
    expect(withCode.errorCode).toBe("errors.auth.invalid_pin");

    fetchMock.mockResolvedValueOnce(mockResponse(403, { message: "nope", code: 7 }));
    const grpcShaped = await catchError(eveyFetch({ url: "/api/v1/x", method: "GET" }));
    expect(grpcShaped.errorCode).toBeUndefined();
  });

  it("the remote-access header WINS over any body code - it survives a rewritten body", async () => {
    fetchMock.mockResolvedValueOnce(
      mockResponse(403, { message: "forbidden", code: "errors.auth.invalid_pin" }, { "X-Evey-Remote-Access": "disabled" }),
    );
    const err = await catchError(eveyFetch({ url: "/api/v1/x", method: "GET" }));
    expect(err.errorCode).toBe("remote_access_disabled");
  });

  it("uses the server's message, and degrades to status text on an unparseable body", async () => {
    fetchMock.mockResolvedValueOnce(mockResponse(500, { message: "disk full" }));
    expect((await catchError(eveyFetch({ url: "/api/v1/x", method: "GET" }))).message).toBe("disk full");

    fetchMock.mockResolvedValueOnce(mockResponse(500));
    expect((await catchError(eveyFetch({ url: "/api/v1/x", method: "GET" }))).message).toBe("500 Error");
  });
});

// The refresh-failure latch (2026-08-25). Once a refresh attempt failed, the
// old code latched `refreshFailed` FOREVER - a single NAS reboot or proxy 502
// during a token expiry left the app unable to ever refresh again (every call:
// stale token → 401 → give up), until a full app restart. A television that
// stays on for days WILL meet a transient failure. The latch must only engage
// when the server REFUSES the refresh credential (the session is truly dead),
// and it must be resettable when a new session begins.
describe("refresh failure latch", () => {
  const onUnauthorized = vi.fn();

  beforeEach(() => {
    onUnauthorized.mockReset();
    setTokenProvider(() => "opaque-token");
    // The outer beforeEach nulls the updater; a SUCCESSFUL refresh calls it,
    // so these tests need a real (no-op) one or the success path itself throws.
    setTokensUpdater(() => {});
    setUnauthorizedHandler(onUnauthorized);
  });

  afterEach(() => {
    setUnauthorizedHandler(null as unknown as () => void);
  });

  it("a TRANSIENT refresh failure does not latch: the next call refreshes again", async () => {
    // call 1: request 401 → refresh POST dies on the network
    fetchMock.mockResolvedValueOnce(mockResponse(401));
    fetchMock.mockRejectedValueOnce(new TypeError("network request failed"));
    await expect(eveyFetch({ url: "/api/v1/x", method: "GET" })).rejects.toMatchObject({ httpStatus: 401 });

    // a transient failure is not a dead session - nobody gets logged out for it
    expect(onUnauthorized).not.toHaveBeenCalled();

    // call 2: request 401 → refresh succeeds → retry succeeds
    fetchMock.mockResolvedValueOnce(mockResponse(401));
    fetchMock.mockResolvedValueOnce(mockResponse(200, { accessToken: "fresh" }));
    fetchMock.mockResolvedValueOnce(mockResponse(200, { ok: true }));
    await expect(eveyFetch({ url: "/api/v1/x", method: "GET" })).resolves.toEqual({ ok: true });
  });

  it("a 5xx from the refresh endpoint is transient too - no latch, no logout", async () => {
    fetchMock.mockResolvedValueOnce(mockResponse(401));
    fetchMock.mockResolvedValueOnce(mockResponse(502, { message: "bad gateway" }));
    await expect(eveyFetch({ url: "/api/v1/x", method: "GET" })).rejects.toMatchObject({ httpStatus: 401 });
    expect(onUnauthorized).not.toHaveBeenCalled();

    fetchMock.mockResolvedValueOnce(mockResponse(401));
    fetchMock.mockResolvedValueOnce(mockResponse(200, { accessToken: "fresh" }));
    fetchMock.mockResolvedValueOnce(mockResponse(200, { ok: true }));
    await expect(eveyFetch({ url: "/api/v1/x", method: "GET" })).resolves.toEqual({ ok: true });
  });

  it("a REFUSAL (401 from the refresh endpoint) latches and fires unauthorized", async () => {
    fetchMock.mockResolvedValueOnce(mockResponse(401));
    fetchMock.mockResolvedValueOnce(mockResponse(401, { message: "refresh token revoked" }));
    await expect(eveyFetch({ url: "/api/v1/x", method: "GET" })).rejects.toMatchObject({ httpStatus: 401 });
    expect(onUnauthorized).toHaveBeenCalledTimes(1);

    // latched: the next 401 gives up without touching the refresh endpoint
    fetchMock.mockResolvedValueOnce(mockResponse(401));
    await expect(eveyFetch({ url: "/api/v1/x", method: "GET" })).rejects.toMatchObject({ httpStatus: 401 });
    const refreshCalls = fetchMock.mock.calls.filter(([url]) => String(url).includes("/auth/refresh"));
    expect(refreshCalls).toHaveLength(1);
  });

  it("resetRefreshFailure lifts the latch - a new login must not inherit a dead session's latch", async () => {
    fetchMock.mockResolvedValueOnce(mockResponse(401));
    fetchMock.mockResolvedValueOnce(mockResponse(401, { message: "refresh token revoked" }));
    await expect(eveyFetch({ url: "/api/v1/x", method: "GET" })).rejects.toMatchObject({ httpStatus: 401 });

    resetRefreshFailure();

    fetchMock.mockResolvedValueOnce(mockResponse(401));
    fetchMock.mockResolvedValueOnce(mockResponse(200, { accessToken: "fresh" }));
    fetchMock.mockResolvedValueOnce(mockResponse(200, { ok: true }));
    await expect(eveyFetch({ url: "/api/v1/x", method: "GET" })).resolves.toEqual({ ok: true });
  });
});
