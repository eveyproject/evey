import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useAdminServiceGetDashboard, useAdminServiceGetSystemInfo } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { DashboardStats } from "@/features/admin/DashboardStats";

export const Route = createLazyFileRoute("/_auth/admin/")({
  component: AdminDashboard,
});

function SystemInfoCard() {
  const { t } = useTranslation("admin");
  const { data: sysData, isLoading } = useAdminServiceGetSystemInfo();

  if (isLoading) {
    return (
      <div className="flex justify-center py-8">
        <Spinner size="md" />
      </div>
    );
  }

  if (!sysData) return null;

  const fields: [string, string | undefined][] = [
    [t("dashboard.version"), sysData.version],
    [t("dashboard.os"), sysData.os],
    [t("dashboard.arch"), sysData.arch],
    [t("dashboard.goVersion"), sysData.goVersion],
    [t("dashboard.ffmpeg"), sysData.ffmpegVersion],
    [t("dashboard.uptime"), sysData.uptimeSeconds ? `${Math.floor(parseInt(sysData.uptimeSeconds) / 60)}m` : undefined],
  ];

  return (
    <div
      className="p-4 rounded-[var(--radius-lg)]"
      style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
    >
      <h3 className="text-sm font-semibold mb-3" style={{ color: "var(--text)" }}>
        {t("dashboard.systemInfo")}
      </h3>
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-x-8 gap-y-3 text-sm">
        {fields.map(([k, v]) =>
          v ? (
            <div key={k}>
              <dt className="font-medium" style={{ color: "var(--text-muted)" }}>
                {k}
              </dt>
              <dd style={{ color: "var(--text)" }}>{v}</dd>
            </div>
          ) : null,
        )}
      </div>
    </div>
  );
}

function AdminDashboard() {
  const { t } = useTranslation("admin");
  const { data: dashData, isLoading: dashLoading, dataUpdatedAt } = useAdminServiceGetDashboard({
    query: { refetchInterval: 10_000 },
  });

  return (
    <div className="flex flex-col gap-8">
      <div className="flex items-center justify-between gap-4 flex-wrap">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          {t("dashboard.title")}
        </h1>
        {dataUpdatedAt > 0 && (
          <p className="text-xs" style={{ color: "var(--text-muted)" }}>
            {t("dashboard.autoRefresh", { time: new Date(dataUpdatedAt).toLocaleTimeString(), interval: "10s" })}
          </p>
        )}
      </div>
      {dashLoading ? (
        <div className="flex justify-center py-8">
          <Spinner size="lg" />
        </div>
      ) : dashData ? (
        <DashboardStats info={dashData} />
      ) : null}

      <SystemInfoCard />
    </div>
  );
}
