import { useState, useEffect } from "react";
import { Search, Folder, Users, Power, Cpu } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceScanLibrary,
  useAdminSystemServiceRestartServer,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import { scanFailedPaths } from "@evey/client";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { useDemoLock, useDemoLockFor } from "@/hooks/useDemoMode";

export function QuickActions() {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const { lock: restartLock } = useDemoLockFor("/evey.v1.AdminSystemService/RestartServer");
  const queryClient = useQueryClient();
  const { data: libData } = useLibraryServiceListLibraries();
  const libraries = libData?.libraries ?? [];
  const scanLib = useLibraryServiceScanLibrary({
    mutation: {
      onSuccess: (data) => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/media"] });
        const failedPaths = scanFailedPaths(data);
        if (failedPaths.length > 0) {
          toast.warning(t("admin.libraries.scanPartial", { paths: failedPaths.join(", ") }));
          return;
        }
        toast.success(t("admin.dashboardStats.scanStarted"));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.dashboardStats.scanFailed"))),
    },
  });
  const [scanningAll, setScanningAll] = useState(false);

  // Restart is a confirm-to-act button: one click arms it, a second click within
  // a few seconds triggers the restart. Avoids a modal for a rare-but-heavy op.
  const [restartArmed, setRestartArmed] = useState(false);
  useEffect(() => {
    if (!restartArmed) return;
    const id = setTimeout(() => setRestartArmed(false), 4000);
    return () => clearTimeout(id);
  }, [restartArmed]);
  const restart = useAdminSystemServiceRestartServer({
    mutation: {
      onSuccess: () => toast.success(t("admin.dashboardStats.restartTriggered")),
      onError: (err: { code?: number; message?: string }) => {
        // gRPC UNAVAILABLE (14) => not supervised in this deployment.
        toast.error(
          err?.code === 14
            ? t("admin.dashboardStats.restartUnavailable")
            : err?.message || t("admin.dashboardStats.restartFailed"),
        );
      },
    },
  });
  function onRestartClick() {
    if (!restartArmed) {
      setRestartArmed(true);
      return;
    }
    setRestartArmed(false);
    restart.mutate({ data: {} });
  }

  async function scanAll() {
    setScanningAll(true);
    let failed = 0;
    for (const lib of libraries) {
      if (lib.id) {
        try { await scanLib.mutateAsync({ id: lib.id }); } catch (err) { console.warn("library scan failed", err); failed++; }
      }
    }
    setScanningAll(false);
    if (failed > 0) {
      toast.error(t("admin.dashboardStats.scanFailed"));
    } else {
      toast.success(t("admin.dashboardStats.allScansTriggered"));
    }
  }

  // A card of full-width actions, sized to sit beside the background-tasks
  // panel: the row of loose buttons it replaced sat alone across the page.
  return (
    <Card>
      <h3 className="text-sm font-semibold mb-3 text-[var(--text)]">
        {t("admin.dashboardStats.quickActions")}
      </h3>
      <div className="flex flex-col gap-2">
        <Button
          variant="secondary"
          className="justify-start"
          loading={scanningAll}
          onClick={scanAll}
          disabled={libraries.length === 0}
          {...lock}
        >
          <Search size={16} />
          {t("admin.dashboardStats.scanAllLibraries")}
        </Button>
        <Link to="/admin/libraries" className="flex">
          <Button variant="secondary" className="w-full justify-start">
            <Folder size={16} />
            {t("admin.dashboardStats.manageLibraries")}
          </Button>
        </Link>
        <Link to="/admin/users" className="flex">
          <Button variant="secondary" className="w-full justify-start">
            <Users size={16} />
            {t("admin.dashboardStats.manageUsers")}
          </Button>
        </Link>
        <Link to="/admin/system-health" className="flex">
          <Button variant="secondary" className="w-full justify-start">
            <Cpu size={16} />
            {t("admin.nav.systemHealth")}
          </Button>
        </Link>
        <Button
          variant={restartArmed ? "danger" : "secondary"}
          className="justify-start"
          loading={restart.isPending}
          onClick={onRestartClick}
          title={t("admin.dashboardStats.restartHint")}
          {...restartLock}
        >
          <Power size={16} />
          {restartArmed
            ? t("admin.dashboardStats.restartConfirm")
            : t("admin.dashboardStats.restartServer")}
        </Button>
      </div>
      {restartArmed && (
        <p className="mt-2 text-xs text-[var(--text-muted)]">
          {t("admin.dashboardStats.restartHint")}
        </p>
      )}
    </Card>
  );
}
