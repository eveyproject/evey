import { useEffect } from "react";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { saveProfileCache } from "@evey/store";

/**
 * Keeps the on-device profile cache in step with the server's list.
 *
 * Mounted once by the authenticated layout, so it covers the whole session and
 * costs no request: React Query dedupes it against every screen that already
 * asks for the same list. The cache is what the "who is watching" redirect
 * reads on a fresh document load, before any component has rendered.
 */
export function ProfileCacheSync() {
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(
    user?.id ?? "",
    { query: { enabled: !!user?.id } },
  );
  const profiles = profilesResp?.profiles;
  useEffect(() => {
    void saveProfileCache(profiles);
  }, [profiles]);
  return null;
}
