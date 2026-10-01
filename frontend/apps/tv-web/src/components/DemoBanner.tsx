import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useAdminSystemServiceGetSetupStatus } from "@evey/api";
import { useDemoStore } from "@evey/store";

// The public demo instance, told apart like on the Android television: the
// server says so on its public setup-status answer. The flag also keeps this
// television's theme and language local instead of writing them to the shared
// profile (the server refuses that write there). Off the demo: nothing.
//
// A pill fixed at the top rather than a strip in the flow: the shells are
// full-height, and a line pushed into them would shift every screen.
export function DemoBanner({ visible }: { visible: boolean }) {
  const { t } = useTranslation();
  const setupStatus = useAdminSystemServiceGetSetupStatus({ query: { staleTime: 60_000 } });
  const demo = setupStatus.data?.demoMode === true;
  useEffect(() => {
    if (setupStatus.data) useDemoStore.getState().setDemoMode(demo);
  }, [demo, setupStatus.data]);
  // Mounted on every signed-in screen so the flag is always set; drawn only on
  // the deep screens, which have no header (the tab screens carry a tag in it).
  if (!demo || !visible) return null;
  return (
    <div className="demo-banner" role="status" data-testid="demo-banner">
      {t("common.demo.banner")}
    </div>
  );
}
