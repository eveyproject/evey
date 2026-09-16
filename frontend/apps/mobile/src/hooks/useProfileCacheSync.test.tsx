import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";
import { useProfileCacheSync } from "./useProfileCacheSync";

const api = vi.hoisted(() => ({
  user: undefined as { id: string } | undefined,
  profiles: undefined as unknown,
  listCalls: [] as { userId: string; enabled: boolean }[],
}));
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: () => ({ data: api.user }),
  useUserServiceListProfiles: (userId: string, o: { query: { enabled: boolean } }) => {
    api.listCalls.push({ userId, enabled: o.query.enabled });
    return { data: api.profiles };
  },
}));
const saveProfileCache = vi.hoisted(() => vi.fn());
vi.mock("@evey/store", () => ({ saveProfileCache: (...a: unknown[]) => saveProfileCache(...a) }));

beforeEach(() => {
  api.user = undefined;
  api.profiles = undefined;
  api.listCalls.length = 0;
  saveProfileCache.mockReset().mockResolvedValue(undefined);
});

// The list is requested only once the user is known, and the server's
// answer is written to the on-device cache the cold-start picker draws from,
// whether or not the user ever opens Settings.
describe("useProfileCacheSync", () => {
  it("holds the list query until the user is known", () => {
    renderHook(() => useProfileCacheSync());
    expect(api.listCalls.at(-1)).toEqual({ userId: "", enabled: false });
    expect(saveProfileCache).toHaveBeenCalledWith(undefined);
  });

  it("asks for the user's profiles and saves them", () => {
    api.user = { id: "u1" };
    const list = [{ id: "p1", name: "Kid" }];
    api.profiles = { profiles: list };
    renderHook(() => useProfileCacheSync());
    expect(api.listCalls.at(-1)).toEqual({ userId: "u1", enabled: true });
    expect(saveProfileCache).toHaveBeenLastCalledWith(list);
  });
});
