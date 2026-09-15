import { describe, it, expect, vi } from "vitest";
import { renderHook } from "@testing-library/react";

// The profile the session RUNS AS is the one whose id the auth store holds -
// not the household's default. Getting this wrong writes settings to the
// wrong profile silently.

let activeProfileId: string | null = "p-kid";
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { profileId: string | null }) => unknown) => selector({ profileId: activeProfileId }),
}));

const profiles = [
  { id: "p-default", name: "Main", isDefault: true },
  { id: "p-kid", name: "Kid", isDefault: false },
];
// Mirrors react-query: a disabled query has no data.
const mockListProfiles = vi.fn((_userId: string, opts: { query: { enabled: boolean } }) =>
  opts.query.enabled ? { data: { profiles } } : { data: undefined },
);
const mockCurrentUser = vi.fn(() => ({ data: { id: "u1" } }));
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: () => mockCurrentUser(),
  useUserServiceListProfiles: (userId: string, opts: { query: { enabled: boolean } }) => mockListProfiles(userId, opts),
}));

import { useActiveProfile } from "./useActiveProfile";

describe("useActiveProfile", () => {
  it("resolves the profile the session runs as, not the default one", () => {
    activeProfileId = "p-kid";
    const { result } = renderHook(() => useActiveProfile());
    expect(result.current.profile?.id).toBe("p-kid");
    expect(result.current.userId).toBe("u1");
    expect(result.current.profiles).toHaveLength(2);
  });

  it("falls back to the default profile, then the first, when the active id matches nothing", () => {
    activeProfileId = "p-gone";
    expect(renderHook(() => useActiveProfile()).result.current.profile?.id).toBe("p-default");

    mockListProfiles.mockReturnValueOnce({ data: { profiles: [{ id: "p-only", name: "Only", isDefault: false }] } } as never);
    expect(renderHook(() => useActiveProfile()).result.current.profile?.id).toBe("p-only");
  });

  it("asks for the profile list only once the user id is known", () => {
    activeProfileId = "p-kid";
    mockListProfiles.mockClear();
    mockCurrentUser.mockReturnValueOnce({ data: undefined } as never);
    const { result } = renderHook(() => useActiveProfile());
    expect(result.current.profile).toBeUndefined();
    expect(mockListProfiles).toHaveBeenCalledWith("", { query: { enabled: false } });
  });
});
