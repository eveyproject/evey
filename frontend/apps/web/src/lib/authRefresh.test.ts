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

import { refreshTokenUnderLock, isTokenExpired } from "./authRefresh";

function jwt(expOffsetSec: number): string {
  const payload = btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + expOffsetSec }));
  return `h.${payload}.s`;
}

beforeEach(() => {
  refreshMock.mockReset();
  setTokensMock.mockReset();
  currentToken = null;
});

describe("isTokenExpired", () => {
  it("is true for a past exp and for garbage, false for a future exp", () => {
    expect(isTokenExpired(jwt(-10))).toBe(true);
    expect(isTokenExpired("not-a-jwt")).toBe(true);
    expect(isTokenExpired(jwt(300))).toBe(false);
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
});
