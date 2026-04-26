import { useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { ProfileSwitcher } from "./ProfileSwitcher";

/**
 * Topbar user-menu wrapper. Fetches the user + profile list and forwards them
 * to ProfileSwitcher (avatar trigger + switch/manage/settings/logout dropdown).
 *
 * Mirrors web's ProfileSwitcher behavior: unified topbar entry-point for both
 * profile switching and account-level actions.
 */
export function UserMenu({ size = 30 }: { size?: number } = {}) {
  const activeProfileId = useAuthStore((s) => s.profileId);
  const { data: currentUser } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(currentUser?.id ?? "", {
    query: { enabled: !!currentUser?.id },
  });
  const profiles = profilesResp?.profiles ?? [];

  if (profiles.length === 0) return null;
  return <ProfileSwitcher profiles={profiles} activeProfileId={activeProfileId} size={size} />;
}
