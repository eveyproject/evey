import { useUserServiceGetCurrentUser, useUserServiceListProfiles, type V1Profile } from "@evey/api";
import { useAuthStore } from "@evey/store";

/**
 * The profile the session is actually RUNNING AS.
 *
 * Switching profiles moves the client-side profileId (and the JWT claim behind
 * it); it does NOT move `isDefault` server-side. So
 * `profiles.find(p => p.isDefault)` names the household's first profile, never
 * "the one in front of the user".
 *
 * Getting that wrong is silent by construction: the write SUCCEEDS, the screen
 * shows the new value, and whatever consumes the setting keeps using the old
 * profile's. It reads as a setting that does nothing. Three components had
 * already open-coded the correct resolution; this is that same chain, named
 * once, so the next screen cannot get it wrong by copying the shorter version.
 *
 * Falls back to the default profile when the active id matches nothing (a
 * single-profile account, or a list that has not landed yet).
 */
export function useActiveProfile(): {
  profile: V1Profile | undefined;
  profiles: V1Profile[] | undefined;
  userId: string | undefined;
  isLoading: boolean;
} {
  const activeProfileId = useAuthStore((s) => s.profileId);
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp, isLoading } = useUserServiceListProfiles(
    user?.id ?? "",
    { query: { enabled: !!user?.id } },
  );
  const profiles = profilesResp?.profiles;
  const profile =
    profiles?.find((p) => p.id === activeProfileId) ??
    profiles?.find((p) => p.isDefault) ??
    profiles?.[0];
  return { profile, profiles, userId: user?.id, isLoading };
}
