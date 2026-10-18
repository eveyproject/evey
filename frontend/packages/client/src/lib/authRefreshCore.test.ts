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
