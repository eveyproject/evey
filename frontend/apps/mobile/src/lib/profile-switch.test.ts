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

const authState = { setTokens: vi.fn() };
vi.mock("@evey/store", () => ({
  useAuthStore: { getState: () => authState },
}));

const refreshCall = vi.fn();
vi.mock("@evey/api", () => ({
  userServiceRefreshToken: (req: unknown) => refreshCall(req),
}));

import { switchProfileToken } from "./profile-switch";
import { setRefreshToken, getRefreshToken } from "./refresh-token";

beforeEach(() => {
  Object.keys(secure).forEach((k) => delete secure[k]);
  authState.setTokens = vi.fn();
  refreshCall.mockReset();
});

describe("switchProfileToken", () => {
  it("sends the stored refresh token and persists the rotated one", async () => {
    await setRefreshToken("old-rt");
    refreshCall.mockResolvedValue({ accessToken: "new-access", refreshToken: "new-rt" });

    const resp = await switchProfileToken("p-2", "pin-proof");

    expect(refreshCall).toHaveBeenCalledWith({ refreshToken: "old-rt", profileId: "p-2", pinToken: "pin-proof" });
    expect(await getRefreshToken()).toBe("new-rt");
    expect(authState.setTokens).toHaveBeenCalledWith("new-access");
    expect(resp.accessToken).toBe("new-access");
  });

  it("keeps the stored token when the server rotates nothing", async () => {
    await setRefreshToken("old-rt");
    refreshCall.mockResolvedValue({ accessToken: "new-access" });

    await switchProfileToken("p-2");

    expect(refreshCall).toHaveBeenCalledWith({ refreshToken: "old-rt", profileId: "p-2" });
    expect(await getRefreshToken()).toBe("old-rt");
  });

  it("omits the refresh token field when nothing is stored", async () => {
    refreshCall.mockResolvedValue({});

    await switchProfileToken("p-2");

    expect(refreshCall).toHaveBeenCalledWith({ profileId: "p-2" });
    expect(authState.setTokens).not.toHaveBeenCalled();
  });

  it("propagates a refusal without touching the stored token", async () => {
    await setRefreshToken("old-rt");
    refreshCall.mockRejectedValue(Object.assign(new Error("pin"), { httpStatus: 403 }));

    await expect(switchProfileToken("p-2")).rejects.toMatchObject({ httpStatus: 403 });
    expect(await getRefreshToken()).toBe("old-rt");
  });
});
