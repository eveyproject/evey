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

import { refreshAccessTokenIfNeeded, forceRefresh } from "./auth-refresh";
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
