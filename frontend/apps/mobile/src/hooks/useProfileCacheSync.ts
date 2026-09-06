import { useEffect } from "react";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { saveProfileCache } from "@evey/store";

/**
 * Keeps the on-device profile cache in step with the server's list.
 *
 * Called once, from the (auth) layout, so it covers the whole authenticated
 * session and costs no extra request: React Query dedupes this against the
 * dozen screens that already ask for the same list under the same key. The
 * cache is what the cold-start picker draws itself from, so it has to be
 * written from somewhere that runs whether or not the user visits Settings.
 */
export function useProfileCacheSync(): void {
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(
    user?.id ?? "",
    { query: { enabled: !!user?.id } } as never,
  );
  const profiles = profilesResp?.profiles;
  useEffect(() => {
    void saveProfileCache(profiles);
  }, [profiles]);
}
