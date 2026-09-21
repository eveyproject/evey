import { describe, it, expect, vi, beforeEach } from "vitest";

const api = vi.hoisted(() => ({ eveyFetch: vi.fn() }));
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => api.eveyFetch(...a) }));
const store = vi.hoisted(() => ({ accessToken: "acc" as string | null }));
vi.mock("@evey/store", () => ({ useAuthStore: { getState: () => ({ accessToken: store.accessToken }) } }));
const tokens = vi.hoisted(() => ({ get: vi.fn(), clear: vi.fn() }));
vi.mock("./refresh-token", () => ({
  getRefreshToken: () => tokens.get(),
  clearRefreshToken: () => tokens.clear(),
}));

import { signOutDevice } from "./sign-out";

beforeEach(() => {
  vi.clearAllMocks();
  store.accessToken = "acc";
  tokens.get.mockResolvedValue("rt-device");
  tokens.clear.mockResolvedValue(undefined);
  api.eveyFetch.mockResolvedValue({});
});

describe("signOutDevice", () => {
  it("names this device's refresh token and carries the access token itself, then clears the disk", async () => {
    await signOutDevice();
    expect(api.eveyFetch).toHaveBeenCalledWith({
      url: "/api/v1/auth/logout",
      method: "POST",
      data: { refreshToken: "rt-device" },
      headers: { Authorization: "Bearer acc" },
    });
    expect(tokens.clear).toHaveBeenCalled();
  });

  it("clears the disk even when the server cannot be reached", async () => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    api.eveyFetch.mockRejectedValue(new Error("network down"));
    await expect(signOutDevice()).resolves.toBeUndefined();
    expect(tokens.clear).toHaveBeenCalled();
    await new Promise((r) => setTimeout(r, 0));
  });

  it("skips the server call without an access token and still clears the disk", async () => {
    store.accessToken = null;
    await signOutDevice();
    expect(api.eveyFetch).not.toHaveBeenCalled();
    expect(tokens.clear).toHaveBeenCalled();
  });

  it("surfaces a failed local clear to the caller", async () => {
    tokens.clear.mockRejectedValue(new Error("keychain locked"));
    await expect(signOutDevice()).rejects.toThrow("keychain locked");
  });
});
