import { useUserServiceGetCurrentUser, useUserServiceListProfiles, type V1Profile } from "@evey/api";
import { useAuthStore } from "@evey/store";

/**
 * The profile the app is actually RUNNING AS.
 *
 * Switching profiles from /profile-select changes the client-side profileId
 * (and the JWT claim behind it); it does NOT move `isDefault` server-side. So
 * `profiles.find(p => p.isDefault)` names the household's first profile, never
 * "the one in front of the user" - and every screen that edited that instead
 * was writing to a profile nobody was looking at.
 *
 * The damage was never loud, because the write SUCCEEDS: the settings screen
 * shows the new value, and the feature that reads it (the player reads audio
 * and subtitle languages off the ACTIVE profile, useParentalGate enforces the
 * ACTIVE profile's PIN and age cap) keeps using the old one. It reads as a
 * setting that does nothing.
 *
 * Falls back to the default profile when the active id cannot be matched -
 * a single-profile account, or a list that has not landed yet.
 */
export function useActiveProfile(): {
  profile: V1Profile | undefined;
  profiles: V1Profile[] | undefined;
  isLoading: boolean;
} {
  const activeProfileId = useAuthStore((s) => s.profileId);
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp, isLoading } = useUserServiceListProfiles(
    user?.id ?? "",
    { query: { enabled: !!user?.id } } as never,
  );
  const profiles = profilesResp?.profiles;
  const profile =
    profiles?.find((p) => p.id === activeProfileId) ??
    profiles?.find((p) => p.isDefault) ??
    profiles?.[0];
  return { profile, profiles, isLoading };
}
