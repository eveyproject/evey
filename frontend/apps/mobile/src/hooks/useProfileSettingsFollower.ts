import { useProfileSettingsFollower as useSharedFollower } from "@evey/client";
import { applyEditedProfileLocale } from "@evey/client/native/i18n";

/**
 * The theme and the language of the running profile, wherever they were last
 * changed: another device's change lands here without opening Settings.
 */
export function useProfileSettingsFollower(): void {
  useSharedFollower((profile, locale) => void applyEditedProfileLocale(profile, locale));
}
