import { FlaskConical } from "lucide-react";
import { useTranslation } from "react-i18next";
import { ICON } from "@/constants/iconSizes";
import { useDemoMode } from "@/hooks/useDemoMode";

// Persistent strip on the public demo instance: says the changes are reset
// and the administration is read-only, so a greyed button or a refused save
// reads as the demo's boundary and not as a bug. Off the demo it renders
// nothing - the server tells the client which it is.
export function DemoBanner() {
  const { t } = useTranslation();
  const demo = useDemoMode();
  if (!demo) return null;
  return (
    <div
      // relative + z-badge: a media page's hero is pulled up under the top
      // padding, and its backdrop and Back scrim (z-raised) painted over this.
      className="relative z-badge flex items-center justify-center gap-2 px-4 py-2 text-sm bg-[var(--toast-warning-bg)] border-b border-[var(--toast-warning-border)] text-[var(--text)]"
      role="status"
      data-testid="demo-banner"
    >
      <FlaskConical size={ICON.sm} aria-hidden="true" className="text-[var(--toast-warning-icon)]" />
      <span>{t("common.demo.banner")}</span>
    </div>
  );
}
