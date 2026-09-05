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

import { refreshAccessTokenIfNeeded, forceRefresh, ensureAccessToken } from "./auth-refresh";
import { setRefreshToken, getRefreshToken } from "./refresh-token";

function jwt(exp: number): string {
  const body = Buffer.from(JSON.stringify({ exp })).toString("base64url");
  return `h.${body}.s`;
}

const past = () => jwt(Math.floor(Date.now() / 1000) - 10);

beforeEach(() => {
  Object.keys(secure).forEach((k) => delete secure[k]);
  authState.accessToken = null;
  authState.setTokens = vi.fn();
  authState.logout = vi.fn();
  vi.unstubAllGlobals();
});

describe("refreshAccessTokenIfNeeded - what ends a session and what does not", () => {
  it("renews an expired token and rotates the stored refresh token", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-old");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({ ok: true, json: async () => ({ accessToken: "fresh", refreshToken: "rt-new" }) })),
    );

    expect(await refreshAccessTokenIfNeeded()).toBe(true);
    expect(authState.setTokens).toHaveBeenCalledWith("fresh");
    expect(await getRefreshToken()).toBe("rt-new");
  });

  // The old code treated ANY !res.ok as a refusal: a proxy 502 during a token
  // expiry logged the user out AND burned their still-valid refresh token.
  it("keeps the session on a 5xx: a server hiccup is not a refusal", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-502");
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status: 502, json: async () => ({}) })));

    expect(await refreshAccessTokenIfNeeded()).toBe(false);
    expect(authState.logout).not.toHaveBeenCalled();
    expect(await getRefreshToken()).toBe("rt-502");
  });

  it("logs out and clears the stored token only when the server refuses (401)", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-dead");
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status: 401, json: async () => ({}) })));

    expect(await refreshAccessTokenIfNeeded()).toBe(false);
    expect(authState.logout).toHaveBeenCalled();
    expect(await getRefreshToken()).toBeNull();
  });

  it("keeps the session on a network blip", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-ok");
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("offline"); }));

    expect(await refreshAccessTokenIfNeeded()).toBe(false);
    expect(authState.logout).not.toHaveBeenCalled();
    expect(await getRefreshToken()).toBe("rt-ok");
  });

  // Overwriting a live token with undefined logs the user out just as surely
  // as a refusal, only without saying so.
  it("keeps the session when a 200 carries no token", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-empty");
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: true, json: async () => ({}) })));

    expect(await refreshAccessTokenIfNeeded()).toBe(false);
    expect(authState.setTokens).not.toHaveBeenCalled();
    expect(authState.logout).not.toHaveBeenCalled();
  });
});

// The boolean above collapses "the server said no" and "nobody answered" into
// one false. Callers that ROUTE on the answer need them apart: bouncing to
// /login on an unanswered refresh locks a user out of their own downloads
// while offline, and /login cannot be completed offline either.
describe("ensureAccessToken - a dead link is not a dead session", () => {
  it("reports 'valid' without a round-trip when the token is still fresh", async () => {
    authState.accessToken = jwt(Math.floor(Date.now() / 1000) + 3600);
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);

    expect(await ensureAccessToken()).toBe("valid");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("reports 'refreshed' when a new token lands", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-old");
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({ ok: true, json: async () => ({ accessToken: "fresh" }) })),
    );

    expect(await ensureAccessToken()).toBe("refreshed");
  });

  it("reports 'unavailable' - NOT 'refused' - when the network is down", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-ok");
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("offline"); }));

    expect(await ensureAccessToken()).toBe("unavailable");
    expect(authState.logout).not.toHaveBeenCalled();
    expect(await getRefreshToken()).toBe("rt-ok");
  });

  it("reports 'unavailable' on a proxy 5xx, keeping the refresh token", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-502");
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status: 502, json: async () => ({}) })));

    expect(await ensureAccessToken()).toBe("unavailable");
    expect(await getRefreshToken()).toBe("rt-502");
  });

  it("reports 'refused' only when the server rejects the credential", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-dead");
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status: 401, json: async () => ({}) })));

    expect(await ensureAccessToken()).toBe("refused");
    expect(authState.logout).toHaveBeenCalled();
  });

  // An unreachable but ROUTABLE host (captive portal, sleeping NAS) leaves
  // fetch pending for the OS connect timeout. The (auth) layout renders
  // nothing while a refresh is in flight, so an unbounded one is a blank app.
  it("gives up on a refresh that never answers instead of hanging", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-hang");
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn((_url: string, init?: { signal?: AbortSignal }) =>
        new Promise((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () => reject(new Error("aborted")));
        }),
      ),
    );

    const pending = ensureAccessToken();
    await vi.advanceTimersByTimeAsync(10_000);
    expect(await pending).toBe("unavailable");
    expect(authState.logout).not.toHaveBeenCalled();
    vi.useRealTimers();
  });
});

describe("forceRefresh - contract with the fetcher's refresh handler", () => {
  // `null` is reserved for a REFUSAL: the fetcher latches its refresh-failure
  // state and fires unauthorized on it. Returning null for a transient error
  // used to latch that state until the app restarted.
  it("THROWS on transient unavailability so the fetcher does not latch", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-blip");
    vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("offline"); }));

    await expect(forceRefresh()).rejects.toThrow();
    expect(authState.logout).not.toHaveBeenCalled();
  });

  it("returns null only when the server REFUSES the credential", async () => {
    authState.accessToken = past();
    await setRefreshToken("rt-dead");
    vi.stubGlobal("fetch", vi.fn(async () => ({ ok: false, status: 403, json: async () => ({}) })));

    expect(await forceRefresh()).toBeNull();
    expect(authState.logout).toHaveBeenCalled();
  });
});
