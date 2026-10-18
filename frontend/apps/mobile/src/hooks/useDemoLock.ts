import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useDemoStore } from "@evey/store";

// The phone's twin of the web's useDemoLock (apps/web/src/hooks/useDemoMode.ts).
// On the demo the favourites, ratings, watched marks, playlists, collections
// and requests are the shared account's and the server refuses them, so their
// controls are greyed instead of letting the press fail. Spread `lock` LAST on
// the Pressable and dim it with DEMO_LOCKED_OPACITY; off the demo `lock` is an
// empty object and the control is exactly what it was.
export interface DemoLockProps {
  disabled?: true;
  accessibilityState?: { disabled: true };
  accessibilityHint?: string;
}

// The dim the bulk bars already give a busy button.
export const DEMO_LOCKED_OPACITY = 0.4;

export function useDemoLock(): { demo: boolean; lock: DemoLockProps } {
  const demo = useDemoStore((s) => s.demoMode);
  const { t } = useTranslation();
  // Memoised: the lock lands in renderItem callbacks' dependencies.
  return useMemo<{ demo: boolean; lock: DemoLockProps }>(() => ({
    demo,
    lock: demo ? { disabled: true, accessibilityState: { disabled: true }, accessibilityHint: t("common.demo.locked") } : {},
  }), [demo, t]);
}
