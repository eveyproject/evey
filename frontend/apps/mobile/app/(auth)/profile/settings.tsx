import { Redirect } from "expo-router";

/**
 * Web parity placeholder: web exposes profile management under
 * /_auth/profile/settings (the "Profiles" tab of the unified settings page).
 * On mobile, account-level settings live under /(tabs)/settings, while
 * profile management has its own dedicated screen at /(auth)/profile/manage.
 *
 * This route exists so any link/deep-link mirroring the web URL still lands
 * the user in the right mobile screen.
 */
export default function ProfileSettingsRedirect() {
  return <Redirect href="/(auth)/profile/manage" />;
}
