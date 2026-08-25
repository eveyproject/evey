import { describe, it, expect, vi, beforeEach } from "vitest";

const secure: Record<string, string> = {};
vi.mock("expo-secure-store", () => ({
  getItemAsync: vi.fn((k: string) => Promise.resolve(secure[k] ?? null)),
  setItemAsync: vi.fn((k: string, v: string) => {
    secure[k] = v;
    return Promise.resolve();
  }),
  deleteItemAsync: vi.fn((k: string) => {
    delete secure[k];
    return Promise.resolve();
  }),
}));

const authState = { accessToken: null as string | null, setTokens: vi.fn(), logout: vi.fn() };
vi.mock("@evey/store", () => ({
  useAuthStore: { getState: () => authState },
}));

vi.mock("./server-config", () => ({
  getServerUrl: () => Promise.resolve("http://srv.test"),
}));

import { isAccessExpired, refreshAccessTokenIfNeeded, forceRefresh } from "./auth-refresh";
import { setRefreshToken, getRefreshToken } from "./refresh-token";

function jwt(exp: number): string {
  const body = Buffer.from(JSON.stringify({ exp })).toString("base64url");
  return `h.${body}.s`;
}

const future = () => jwt(Math.floor(Date.now() / 1000) + 3600);
const past = () => jwt(Math.floor(Date.now() / 1000) - 10);

beforeEach(() => {
  Object.keys(secure).forEach((k) => delete secure[k]);
  authState.accessToken = null;
  authState.setTokens = vi.fn();
  authState.logout = vi.fn();
  vi.unstubAllGlobals();
});

describe("isAccessExpired", () => {
  it("treats a missing or unparsable token as expired", () => {
    expect(isAccessExpired(null)).toBe(true);
    expect(isAccessExpired("not-a-jwt")).toBe(true);
  });

  it("treats a token expiring within 30s as expired so we do not race the server", () => {
    expect(isAccessExpired(jwt(Math.floor(Date.now() / 1000) + 10))).toBe(true);
    expect(isAccessExpired(future())).toBe(false);
  });
});

describe("refreshAccessTokenIfNeeded", () => {
  it("does not hit the network while the access token is still valid", async () => {
    authState.accessToken = future();
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    expect(await refreshAccessTokenIfNeeded()).toBe(true);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("renews an expired token from the stored refresh token and rotates it", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-old");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({
        ok: true,
        json: async () => ({ accessToken: "fresh", refreshToken: "rt-new" }),
      })),
    );

    expect(await refreshAccessTokenIfNeeded()).toBe(true);
    expect(authState.setTokens).toHaveBeenCalledWith("fresh");
    // The server single-uses refresh tokens: keeping the old one would log the
    // viewer out on the next renewal.
    expect(await getRefreshToken()).toBe("rt-new");
    expect(authState.logout).not.toHaveBeenCalled();
  });

  it("logs out and clears the stored token when the server refuses the refresh", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-dead");
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status: 401, json: async () => ({}) })));

    expect(await refreshAccessTokenIfNeeded()).toBe(false);
    expect(authState.logout).toHaveBeenCalled();
    expect(await getRefreshToken()).toBeNull();
  });

  it("keeps the session on a network blip instead of logging out", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-ok");
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("offline"); }));

    expect(await refreshAccessTokenIfNeeded()).toBe(false);
    expect(authState.logout).not.toHaveBeenCalled();
    expect(await getRefreshToken()).toBe("rt-ok");
  });

  it("coalesces concurrent callers onto one request", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-1");
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({ accessToken: "fresh" }),
    }));
    vi.stubGlobal("fetch", fetchMock);

    const [a, b, c] = await Promise.all([
      refreshAccessTokenIfNeeded(),
      refreshAccessTokenIfNeeded(),
      refreshAccessTokenIfNeeded(),
    ]);
    expect([a, b, c]).toEqual([true, true, true]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe("refreshAccessTokenIfNeeded - long-running TV session edge cases", () => {
  // The implementation does not special-case the HTTP status: any !res.ok
  // response is treated as a refusal and logs the viewer out, a 500 included.
  // A television left on for hours would lose its session on a transient
  // server error rather than keeping it, unlike a real auth refusal (401).
  it("keeps the session on a 500: a server hiccup is not a refusal", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-500");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({ ok: false, status: 500, json: async () => ({}) })),
    );

    expect(await refreshAccessTokenIfNeeded()).toBe(false);
    // The viewer stays logged in: a television has no keyboard to log back in
    // with, and the server will most likely answer again in a moment.
    expect(authState.logout).not.toHaveBeenCalled();
    expect(await getRefreshToken()).toBe("rt-500");
  });

  // Overwriting a live token with undefined logs the viewer out just as surely
  // as a refusal, only without saying so.
  it("keeps the session when a 200 carries no token", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-invalid-payload");
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, json: async () => ({}) })));

    await expect(refreshAccessTokenIfNeeded()).resolves.toBe(false);
    expect(authState.setTokens).not.toHaveBeenCalled();
    expect(authState.logout).not.toHaveBeenCalled();
    expect(await getRefreshToken()).toBe("rt-invalid-payload");
  });

  it("performs a new network call for a later renewal after an earlier one already completed", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-a");
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({ accessToken: "fresh-1" }),
    }));
    vi.stubGlobal("fetch", fetchMock);

    expect(await refreshAccessTokenIfNeeded()).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    authState.accessToken = past();
    expect(await refreshAccessTokenIfNeeded()).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("concludes there is no session without hitting the network when no refresh token is stored", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);

    expect(await refreshAccessTokenIfNeeded()).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(authState.logout).not.toHaveBeenCalled();
  });

  it("succeeds on a later call after an earlier one failed on a network error", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-retry");
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(async () => {
        throw new Error("offline");
      })
      .mockImplementationOnce(async () => ({
        ok: true,
        json: async () => ({ accessToken: "fresh-2" }),
      }));
    vi.stubGlobal("fetch", fetchMock);

    expect(await refreshAccessTokenIfNeeded()).toBe(false);
    expect(authState.logout).not.toHaveBeenCalled();

    authState.accessToken = past();
    expect(await refreshAccessTokenIfNeeded()).toBe(true);
    expect(authState.setTokens).toHaveBeenCalledWith("fresh-2");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe("forceRefresh", () => {
  it("refreshes even when the cached token still looks valid", async () => {
    authState.accessToken = future();
    await setRefreshToken("rt-1");
    const fetchMock = vi.fn(async () => ({
      ok: true,
      json: async () => ({ accessToken: "fresh" }),
    }));
    vi.stubGlobal("fetch", fetchMock);
    authState.setTokens = vi.fn((t: string) => { authState.accessToken = t; });

    expect(await forceRefresh()).toEqual({ accessToken: "fresh" });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("returns null with no stored refresh token", async () => {
    authState.accessToken = past();
    expect(await forceRefresh()).toBeNull();
  });
});

describe("forceRefresh - contract with the fetcher's refresh handler", () => {
  // `null` is reserved for a REFUSAL: the fetcher latches its refresh-failure
  // state and fires unauthorized on it. Returning null for a mere network
  // blip used to latch that state forever on a TV that stays on for days.
  it("THROWS on transient unavailability so the fetcher does not latch", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-blip");
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("offline"); }));

    await expect(forceRefresh()).rejects.toThrow();
    expect(authState.logout).not.toHaveBeenCalled();
    expect(await getRefreshToken()).toBe("rt-blip");
  });

  it("throws on a 5xx too - the infrastructure is not the credential", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-502");
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status: 502, json: async () => ({}) })));

    await expect(forceRefresh()).rejects.toThrow();
    expect(authState.logout).not.toHaveBeenCalled();
  });

  it("returns null only when the server REFUSES the credential", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-dead");
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status: 401, json: async () => ({}) })));

    expect(await forceRefresh()).toBeNull();
    expect(authState.logout).toHaveBeenCalled();
    expect(await getRefreshToken()).toBeNull();
  });
});
