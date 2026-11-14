import { useEffect, useRef } from "react";
import { isDemoMode, useThemeStore } from "@evey/store";
import { useActiveProfile } from "./useActiveProfile";

/**
 * Keeps this client on the settings of the profile it runs as, wherever they
 * were last changed. Mounted once for the signed-in session: the theme used to
 * be adopted only by the settings screen, so a theme picked on the phone did
 * not reach the television until someone opened its settings.
 *
 * The language is the platform's to apply (each loads its catalogue its own
 * way): onLocaleChange fires when the running profile's language changes
 * under the session, never for the value it started with - the boot already
 * settled that one.
 */
export function useProfileSettingsFollower(
  onLocaleChange?: (profile: { id?: string; isDefault?: boolean }, locale: string) => void,
): void {
  const { profile } = useActiveProfile();
  const applyFromPreferences = useThemeStore((s) => s.applyFromPreferences);

  useEffect(() => {
    applyFromPreferences(profile?.preferences);
  }, [profile?.preferences, applyFromPreferences]);

  const seen = useRef<{ id: string; locale: string } | null>(null);
  const onLocaleRef = useRef(onLocaleChange);
  onLocaleRef.current = onLocaleChange;
  const id = profile?.id;
  const locale = profile?.locale ?? "";
  useEffect(() => {
    if (!id) return;
    const previous = seen.current;
    seen.current = { id, locale };
    // Demo: the shared profile's language is not this visitor's pick.
    if (isDemoMode()) return;
    if (previous && previous.id === id && previous.locale !== locale) {
      onLocaleRef.current?.({ id, isDefault: profile?.isDefault }, locale);
    }
  }, [id, locale, profile?.isDefault]);
}
