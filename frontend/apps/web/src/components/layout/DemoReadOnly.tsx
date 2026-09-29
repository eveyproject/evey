import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useDemoMode } from "@/hooks/useDemoMode";

// Read-only wrapper for the admin pages whose every control is an operator
// write the demo refuses (settings, transcoding knobs, maintenance jobs). A
// disabled <fieldset> is the browser's own way to disable every button, input
// and select beneath it at once, so the page shows its state and offers no
// gesture that the server would refuse. `display: contents` keeps the fieldset
// out of the layout. Off the demo it renders the children untouched.
//
// Pages where reads and writes sit side by side (users, libraries, plugins)
// lock their write controls one by one with useDemoLock instead, so the reads
// stay interactive.
export function DemoReadOnly({ children, silent = false }: { children: ReactNode; silent?: boolean }) {
  const { t } = useTranslation();
  const demo = useDemoMode();
  if (!demo) return <>{children}</>;
  return (
    <>
      {/* silent: a card on a page that already carries the notice. */}
      {!silent && (
        <p role="status" className="text-xs text-[var(--text-muted)]" data-testid="demo-readonly">
          {t("common.demo.readOnly")}
        </p>
      )}
      <fieldset disabled className="contents min-w-0 border-0 p-0 m-0">
        {children}
      </fieldset>
    </>
  );
}
