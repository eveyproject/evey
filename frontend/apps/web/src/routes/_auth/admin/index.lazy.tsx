import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useAdminSystemServiceGetDashboard } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { DashboardStats } from "@/features/admin/DashboardStats";
import { PageHeader } from "@/components/ui/PageHeader";

export const Route = createLazyFileRoute("/_auth/admin/")({
  component: AdminDashboard,
});

// The dashboard is the live view; version, OS, ffmpeg and uptime belong to
// System Health, where they sit next to the hardware and capacity facts.
function AdminDashboard() {
  const { t } = useTranslation();
  const { data: dashData, isLoading: dashLoading, dataUpdatedAt } = useAdminSystemServiceGetDashboard({
    query: { refetchInterval: 3_000, refetchIntervalInBackground: false },
  });

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.dashboard.title")} actions={dataUpdatedAt > 0 ? (
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.dashboard.autoRefresh", { time: new Date(dataUpdatedAt).toLocaleTimeString(), interval: "3s" })}
        </p>
      ) : undefined} />
      {dashLoading ? (
        <div className="flex justify-center py-8">
          <Spinner size="lg" />
        </div>
      ) : dashData ? (
        <DashboardStats info={dashData} />
      ) : null}
    </div>
  );
}
