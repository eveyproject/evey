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
    // forceRefresh's contract: null is a refusal, a throw an absent answer.
    await expect(flow.forceRefresh()).resolves.toBeNull();
    vi.unstubAllGlobals();
    expect(clear).toHaveBeenCalledTimes(1);
    expect(logout).toHaveBeenCalledTimes(1);
  });
});

// The refresh route has its own limiter: a 429 there is the server asking
// for a pause, not a dead session. The natives' forceRefresh is what the
// fetcher's 401 retry calls; it used to throw a bare Error, so the original
// request failed as an authorization error with no delay to honour.
describe("createAuthRefresh - a limited refresh", () => {
  beforeEach(() => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
  });

  it("rejects forceRefresh with the limiter's status and delay, and keeps the session", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 429, headers: { "Retry-After": "7" } })));
    useAuthStore.setState({ accessToken: jwt(-60) });
    const clearRefreshToken = vi.fn(async () => {});
    const flow = createAuthRefresh(storage({ clearRefreshToken }));
    await expect(flow.forceRefresh()).rejects.toMatchObject({ httpStatus: 429, retryAfterSeconds: 7 });
    expect(clearRefreshToken).not.toHaveBeenCalled();
    expect(useAuthStore.getState().accessToken).not.toBeNull();
    vi.unstubAllGlobals();
  });

  it("still rejects any other unavailable answer without a status", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response("", { status: 502 })));
    const flow = createAuthRefresh(storage({}));
    const err = await flow.forceRefresh().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(Error);
    expect((err as { httpStatus?: number }).httpStatus).toBeUndefined();
    vi.unstubAllGlobals();
  });
});

// The sign-in screen reached with a session the server may still honour: a
// boot whose renewal got no answer sent the viewer to a password form while
// signed in. It renews there - but never on a process that held its session
// already, where the screen follows a sign-out whose revocation may still be
// in flight: renewing would undo it.
describe("createAuthRefresh - the sign-in screen", () => {
  beforeEach(() => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    useAuthStore.setState({ accessToken: null });
  });

  it("renews on a process that never held a live token", async () => {
    const fresh = jwt(900, "fresh");
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({ accessToken: fresh }), { status: 200 }));
    vi.stubGlobal("fetch", fetchMock);
    const flow = createAuthRefresh(storage({}));
    await expect(flow.renewOnSignInScreen()).resolves.toBe(true);
    vi.unstubAllGlobals();
    expect(fetchMock).toHaveBeenCalledOnce();
    expect(useAuthStore.getState().accessToken).toBe(fresh);
  });

  it("asks nothing on a process that held a session, even with a token still stored", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const flow = createAuthRefresh(storage({}));
    useAuthStore.setState({ accessToken: jwt(900, "signed-in") });
    useAuthStore.getState().logout();
    await expect(flow.renewOnSignInScreen()).resolves.toBe(false);
    vi.unstubAllGlobals();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("is false without a request once the store holds no refresh token", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const flow = createAuthRefresh(storage({ getRefreshToken: async () => null }));
    await expect(flow.renewOnSignInScreen()).resolves.toBe(false);
    vi.unstubAllGlobals();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
