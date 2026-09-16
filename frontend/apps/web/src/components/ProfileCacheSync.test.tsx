import { describe, it, expect, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import { ProfileCacheSync } from "./ProfileCacheSync";

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

// The list is only requested once the user is known, and whatever the server
// answers (including "nothing yet") is written to the on-device cache the
// who-is-watching redirect reads before any component renders.
describe("ProfileCacheSync", () => {
  it("does not ask for profiles before the user is known", () => {
    render(<ProfileCacheSync />);
    expect(api.listCalls.at(-1)).toEqual({ userId: "", enabled: false });
    expect(saveProfileCache).toHaveBeenCalledWith(undefined);
  });

  it("asks for the user's profiles and saves the list", () => {
    api.user = { id: "u1" };
    const list = [{ id: "p1", name: "Kid" }];
    api.profiles = { profiles: list };
    render(<ProfileCacheSync />);
    expect(api.listCalls.at(-1)).toEqual({ userId: "u1", enabled: true });
    expect(saveProfileCache).toHaveBeenLastCalledWith(list);
  });
});
