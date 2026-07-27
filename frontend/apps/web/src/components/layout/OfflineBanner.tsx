import { memo } from "react";
import { WifiOff } from "lucide-react";
import { useTranslation } from "react-i18next";
import { ICON } from "@/constants/iconSizes";
import { useOnlineStatus } from "@/lib/online-manager";

/**
 * Persistent strip shown while the browser has no connectivity - mirrors the
 * mobile OfflineBanner so a lost connection reads the same way on both
 * platforms instead of failing silently (requests just erroring out one by
 * one with no explanation).
 */
function OfflineBannerImpl() {
  const online = useOnlineStatus();
  const { t } = useTranslation();

  if (online) return null;

  return (
    <div
      className="fixed top-0 inset-x-0 z-popover flex items-center justify-center gap-2 py-2 text-sm font-semibold border-b bg-[var(--toast-warning-bg)] border-[var(--toast-warning-border)] text-[var(--text)]"
      role="status"
      aria-live="polite"
      data-testid="offline-banner"
    >
      <WifiOff size={ICON.sm} aria-hidden="true" className="text-[var(--toast-warning-icon)]" />
      {t("common.offline.title", "Offline")}
    </div>
  );
}

// memo: no props; only re-renders on its own hooks (online status, i18n).
export const OfflineBanner = memo(OfflineBannerImpl);
