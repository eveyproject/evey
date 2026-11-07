import { describe, it, expect, vi, beforeEach } from "vitest";

const refreshMock = vi.fn();
const setTokensMock = vi.fn();
let currentToken: string | null = null;

vi.mock("@evey/api", () => ({
  userServiceRefreshToken: (...args: unknown[]) => refreshMock(...args),
}));
vi.mock("@evey/store", () => ({
  useAuthStore: { getState: () => ({ accessToken: currentToken, setTokens: setTokensMock }) },
}));

import { refreshTokenUnderLock, refreshTokenWithOutcome, isTokenExpired } from "./authRefresh";

function jwt(expOffsetSec: number): string {
  const payload = btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + expOffsetSec }));
  return `h.${payload}.s`;
}

beforeEach(() => {
  refreshMock.mockReset();
  setTokensMock.mockReset();
  currentToken = null;
  localStorage.clear();
});

describe("isTokenExpired", () => {
  it("is true for a past exp and for garbage, false for a future exp", () => {
    expect(isTokenExpired(jwt(-10))).toBe(true);
    expect(isTokenExpired("not-a-jwt")).toBe(true);
    expect(isTokenExpired(jwt(300))).toBe(false);
  });
});

describe("refreshTokenWithOutcome", () => {
  it("is denied when the server answers 401 or 403 - the session is dead", async () => {
    currentToken = jwt(-10);
    refreshMock.mockRejectedValue(Object.assign(new Error("refresh failed"), { httpStatus: 401 }));
    expect(await refreshTokenWithOutcome()).toBe("denied");
    refreshMock.mockRejectedValue(Object.assign(new Error("refresh failed"), { httpStatus: 403 }));
    expect(await refreshTokenWithOutcome()).toBe("denied");
  });

  it("is transient on a 5xx or a network error - the session is not known to be dead", async () => {
    currentToken = jwt(-10);
    refreshMock.mockRejectedValue(Object.assign(new Error("refresh failed"), { httpStatus: 503 }));
    expect(await refreshTokenWithOutcome()).toBe("transient");
    refreshMock.mockRejectedValue(new TypeError("Failed to fetch"));
    expect(await refreshTokenWithOutcome()).toBe("transient");
    expect(setTokensMock).not.toHaveBeenCalled();
  });

  it("is denied when the server answers without a token", async () => {
    currentToken = jwt(-10);
    refreshMock.mockResolvedValue({});
    expect(await refreshTokenWithOutcome()).toBe("denied");
  });
});

describe("refreshTokenUnderLock", () => {
  it("POSTs refresh and stores the new token when the current one is expired", async () => {
    currentToken = jwt(-10);
    refreshMock.mockResolvedValue({ accessToken: "new-token" });
    const ok = await refreshTokenUnderLock();
    expect(ok).toBe(true);
    expect(refreshMock).toHaveBeenCalledOnce();
    expect(setTokensMock).toHaveBeenCalledWith("new-token");
  });

  it("skips the POST when another tab already left a fresh token (lock dedup)", async () => {
    currentToken = jwt(300);
    const ok = await refreshTokenUnderLock();
    expect(ok).toBe(true);
    expect(refreshMock).not.toHaveBeenCalled();
    expect(setTokensMock).not.toHaveBeenCalled();
  });

  it("returns false when the refresh call throws", async () => {
    currentToken = jwt(-10);
    refreshMock.mockRejectedValue(new Error("boom"));
    const ok = await refreshTokenUnderLock();
    expect(ok).toBe(false);
  });

  it("routes the refresh through the evey-refresh-token exclusive Web Lock", async () => {
    // jsdom has no navigator.locks, so inject a minimal one to prove the refresh
    // is serialized under the named exclusive lock (the whole reason this helper
    // exists: concurrent /auth/refresh trips reuse-detection and purges sessions).
    currentToken = jwt(-10);
    refreshMock.mockResolvedValue({ accessToken: "new-token" });
    const requestMock = vi.fn((_name: string, _opts: unknown, fn: () => Promise<unknown>) => fn());
    Object.defineProperty(navigator, "locks", { value: { request: requestMock }, configurable: true });
    try {
      const ok = await refreshTokenUnderLock();
      expect(ok).toBe(true);
      expect(requestMock).toHaveBeenCalledWith(
        "evey-refresh-token",
        { mode: "exclusive" },
        expect.any(Function),
      );
      expect(refreshMock).toHaveBeenCalledOnce();
    } finally {
      // @ts-expect-error remove the injected API so the other tests keep exercising the fallback
      delete navigator.locks;
    }
  });
});

// The login route on a document load: the refresh cookie may still be live,
// so ask the server before asking for a password. Not on a document that held
// a session (a sign-out, whose revocation may be in flight) nor after a
// refusal (the server already said no): there the form stands.
describe("renewOnLoginArrival", () => {
  // The verdict lives at module scope, as the document's does: a fresh module
  // per test is a fresh document.
  async function freshDocument() {
    vi.resetModules();
    return import("./authRefresh");
  }

  it("renews on a document that never held a live token", async () => {
    const doc = await freshDocument();
    refreshMock.mockResolvedValue({ accessToken: "fresh" });
    expect(await doc.renewOnLoginArrival()).toBe(true);
    expect(refreshMock).toHaveBeenCalledOnce();
    expect(setTokensMock).toHaveBeenCalledWith("fresh");
  });

  it("does not ask again once a refresh was refused, until a sign-in lands", async () => {
    const doc = await freshDocument();
    refreshMock.mockRejectedValue(Object.assign(new Error("refresh failed"), { httpStatus: 401 }));
    expect(await doc.refreshTokenWithOutcome()).toBe("denied");
    expect(await doc.renewOnLoginArrival()).toBe(false);
    expect(refreshMock).toHaveBeenCalledOnce();
  });

  it("stands aside on a document that held a session: a sign-out is not undone", async () => {
    const doc = await freshDocument();
    doc.noteLiveTokenHeld();
    currentToken = null;
    refreshMock.mockResolvedValue({ accessToken: "fresh" });
    expect(await doc.renewOnLoginArrival()).toBe(false);
    expect(refreshMock).not.toHaveBeenCalled();
  });

  // The page cannot read the HttpOnly cookie, so the browser keeps what it
  // last knew of it: a sign-out or a refused refresh ends it, and later
  // loads of /login show the form without a request. A sign-in starts over.
  it("asks nothing on a later load once this browser's session ended", async () => {
    (await freshDocument()).noteSessionEnded();
    const later = await freshDocument();
    refreshMock.mockResolvedValue({ accessToken: "fresh" });
    expect(await later.renewOnLoginArrival()).toBe(false);
    expect(refreshMock).not.toHaveBeenCalled();
  });

  it("asks again on a later load once a sign-in landed after the end", async () => {
    (await freshDocument()).noteSessionEnded();
    (await freshDocument()).noteLiveTokenHeld();
    const later = await freshDocument();
    refreshMock.mockResolvedValue({ accessToken: "fresh" });
    expect(await later.renewOnLoginArrival()).toBe(true);
    expect(refreshMock).toHaveBeenCalledOnce();
  });

  it("remembers the server's own refusal (401) across loads, not the gate's (403)", async () => {
    const refused = await freshDocument();
    refreshMock.mockRejectedValue(Object.assign(new Error("refresh failed"), { httpStatus: 401 }));
    expect(await refused.refreshTokenWithOutcome()).toBe("denied");
    refreshMock.mockResolvedValue({ accessToken: "fresh" });
    expect(await (await freshDocument()).renewOnLoginArrival()).toBe(false);
    expect(refreshMock).toHaveBeenCalledOnce();

    localStorage.clear();
    const gated = await freshDocument();
    refreshMock.mockRejectedValue(Object.assign(new Error("refresh failed"), { httpStatus: 403 }));
    expect(await gated.refreshTokenWithOutcome()).toBe("denied");
    refreshMock.mockResolvedValue({ accessToken: "fresh" });
    expect(await (await freshDocument()).renewOnLoginArrival()).toBe(true);
  });

  it("still asks when the hint cannot be read", async () => {
    const doc = await freshDocument();
    const getItem = vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => { throw new Error("SecurityError"); });
    try {
      refreshMock.mockResolvedValue({ accessToken: "fresh" });
      expect(await doc.renewOnLoginArrival()).toBe(true);
    } finally {
      getItem.mockRestore();
    }
  });
});
