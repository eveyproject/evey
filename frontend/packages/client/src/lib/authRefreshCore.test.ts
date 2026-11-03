import { describe, it, expect, vi, beforeEach } from "vitest";
import { useAuthStore } from "@evey/store";
import { createAuthRefresh, type RefreshTokenStorage } from "./authRefreshCore";

function storage(over: Partial<RefreshTokenStorage>): RefreshTokenStorage {
  return {
    getRefreshToken: async () => "rt",
    setRefreshToken: async () => {},
    clearRefreshToken: async () => {},
    getServerUrl: async () => "http://evey",
    ...over,
  };
}

// A keystore read that rejects (Android SecureStore's DecryptException) used to
// escape the outcome: the mobile gate sat on "pending" (a blank shell) and the
// TV spinner never stopped. It is an absent answer, never a refusal.
describe("createAuthRefresh - unreadable secure storage", () => {
  beforeEach(() => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    useAuthStore.setState({ accessToken: null });
  });

  it("reads a rejected refresh-token read as unavailable, not as a throw", async () => {
    const logout = vi.spyOn(useAuthStore.getState(), "logout");
    const flow = createAuthRefresh(storage({ getRefreshToken: () => Promise.reject(new Error("DecryptException")) }));
    await expect(flow.ensureAccessToken()).resolves.toBe("unavailable");
    expect(logout).not.toHaveBeenCalled();
  });

  it("reads a rejected server-url read as unavailable", async () => {
    const flow = createAuthRefresh(storage({ getServerUrl: () => Promise.reject(new Error("DecryptException")) }));
    await expect(flow.refreshAccessTokenIfNeeded()).resolves.toBe(false);
    await expect(flow.forceRefresh()).rejects.toThrow(/unavailable/);
  });
});

// A JWT whose exp is offsetSec from now.
function jwt(offsetSec: number, tag = ""): string {
  const payload = btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + offsetSec, tag }));
  return `h.${payload}.s`;
}

// A television that goes to sleep while a refresh is in flight freezes it
// there: the server answers, the app only reads the answer on wake, hours
// later, and that answer carries an access token that expired long ago. The
// screen guard renewing that token from the store's change listener joined the
// same refresh and was told "refreshed" with the expired token still held: it
// sent the viewer to sign in on a live session.
describe("createAuthRefresh - an answer read long after it was sent", () => {
  beforeEach(() => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
  });

  it("never reports success while the store holds an expired token", async () => {
    const fresh = jwt(900, "fresh");
    const answers = [
      { accessToken: jwt(-36_000, "stale"), refreshToken: "rt2" },
      { accessToken: fresh, refreshToken: "rt3" },
    ];
    const fetchMock = vi.fn(async () => new Response(JSON.stringify(answers.shift()), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    useAuthStore.setState({ accessToken: jwt(-40_000, "before-sleep") });
    const flow = createAuthRefresh(storage({}));

    // The screen guard: every expired token the store takes on gets renewed,
    // from inside the store update, as the layout's effect does.
    const verdicts: Array<{ state: string; heldExpired: boolean }> = [];
    const unsubscribe = useAuthStore.subscribe((s, prev) => {
      if (s.accessToken === prev.accessToken || !s.accessToken) return;
      void flow.ensureAccessToken().then((state) => {
        const held = useAuthStore.getState().accessToken;
        const exp = JSON.parse(atob(held!.split(".")[1]!)).exp as number;
        verdicts.push({ state, heldExpired: exp < Date.now() / 1000 });
      });
    });

    const state = await flow.ensureAccessToken();
    await vi.waitFor(() => expect(verdicts.length).toBeGreaterThan(0));
    unsubscribe();
    vi.unstubAllGlobals();

    expect(state).toBe("refreshed");
    expect(useAuthStore.getState().accessToken).toBe(fresh);
    // Whatever the guard is told, it never holds an expired token with it.
    for (const v of verdicts) expect(v.heldExpired).toBe(false);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

// Only the server's own refusal of the credential (401) ends a session. A 403
// on this call comes from what stands in front of the server - remote access
// switched off while the phone is away, a bouncer banning the household's
// address, the CSRF guard - and reading it as a refusal burned the refresh
// token of every device of the house for a ban that lifted an hour later.
describe("createAuthRefresh - a 403 is the gate, not the credential", () => {
  beforeEach(() => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    useAuthStore.setState({ accessToken: jwt(-60, "expired") });
  });

  it("keeps the session and the stored token on a 403", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 403, headers: { "X-Evey-Remote-Access": "disabled" } })));
    const logout = vi.spyOn(useAuthStore.getState(), "logout");
    const clear = vi.fn(async () => {});
    const flow = createAuthRefresh(storage({ clearRefreshToken: clear }));
    await expect(flow.forceRefresh()).rejects.toThrow(/unavailable/);
    vi.unstubAllGlobals();
    expect(clear).not.toHaveBeenCalled();
    expect(logout).not.toHaveBeenCalled();
  });

  it("still ends the session on a 401", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 401 })));
    const logout = vi.spyOn(useAuthStore.getState(), "logout");
    const clear = vi.fn(async () => {});
    const flow = createAuthRefresh(storage({ clearRefreshToken: clear }));
    await expect(flow.forceRefresh()).rejects.toThrow();
    vi.unstubAllGlobals();
    expect(clear).toHaveBeenCalledTimes(1);
    expect(logout).toHaveBeenCalledTimes(1);
  });
});
