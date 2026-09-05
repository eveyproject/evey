// Three settings screens (region, playback, parental) resolved the profile to
// edit as `profiles.find(p => p.isDefault)`. Switching profiles never moves
// `isDefault` server-side, so on any non-default profile those screens showed
// and wrote somebody else's settings - while the features that read them (the
// player's audio/subtitle languages, useParentalGate's PIN and age cap) go on
// using the ACTIVE profile. The write succeeded, so nothing looked broken; the
// setting simply had no effect. This hook is the single answer to "which
// profile is in front of the user", and these tests pin its precedence.
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook } from "@testing-library/react";

const authState = { profileId: null as string | null };
vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: typeof authState) => unknown) => selector(authState),
}));

let profiles: Array<{ id: string; isDefault?: boolean }> | undefined;
vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: () => ({ data: { id: "u1" } }),
  useUserServiceListProfiles: () => ({ data: profiles ? { profiles } : undefined, isLoading: false }),
}));

import { useActiveProfile } from "./useActiveProfile";

beforeEach(() => {
  authState.profileId = null;
  profiles = [
    { id: "p-default", isDefault: true },
    { id: "p-kid" },
  ];
});

describe("useActiveProfile", () => {
  it("returns the profile the session is running as, not the household default", () => {
    authState.profileId = "p-kid";
    const { result } = renderHook(() => useActiveProfile());
    expect(result.current.profile?.id).toBe("p-kid");
  });

  it("falls back to the default profile when the active id matches nothing", () => {
    authState.profileId = "p-deleted";
    const { result } = renderHook(() => useActiveProfile());
    expect(result.current.profile?.id).toBe("p-default");
  });

  it("falls back to the first profile when none is flagged default", () => {
    authState.profileId = null;
    profiles = [{ id: "p-only" }];
    const { result } = renderHook(() => useActiveProfile());
    expect(result.current.profile?.id).toBe("p-only");
  });

  it("returns nothing rather than a wrong guess before the list lands", () => {
    authState.profileId = "p-kid";
    profiles = undefined;
    const { result } = renderHook(() => useActiveProfile());
    expect(result.current.profile).toBeUndefined();
  });
});
