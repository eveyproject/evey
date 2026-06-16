import { useState } from "react";
import { Search, Folder, Users } from "lucide-react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceScanLibrary,
} from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";

export function QuickActions() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data: libData } = useLibraryServiceListLibraries();
  const libraries = libData?.libraries ?? [];
  const scanLib = useLibraryServiceScanLibrary({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/media"] });
        toast.success(t("admin.dashboardStats.scanStarted"));
      },
      onError: (err: Error) => toast.error(err.message || t("admin.dashboardStats.scanFailed")),
    },
  });
  const [scanningAll, setScanningAll] = useState(false);

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

  return (
    <div>
      <h3 className="text-sm font-semibold mb-3 text-[var(--text)]">
        {t("admin.dashboardStats.quickActions")}
      </h3>
      <div className="flex gap-3 flex-wrap">
        <Button
          variant="secondary"
          loading={scanningAll}
          onClick={scanAll}
          disabled={libraries.length === 0}
        >
          <Search size={16} />
          {t("admin.dashboardStats.scanAllLibraries")}
        </Button>
        <Link to="/admin/libraries">
          <Button variant="secondary">
            <Folder size={16} />
            {t("admin.dashboardStats.manageLibraries")}
          </Button>
        </Link>
        <Link to="/admin/users">
          <Button variant="secondary">
            <Users size={16} />
            {t("admin.dashboardStats.manageUsers")}
          </Button>
        </Link>
      </div>
    </div>
  );
}
