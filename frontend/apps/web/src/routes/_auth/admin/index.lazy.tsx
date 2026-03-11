import { createLazyFileRoute } from "@tanstack/react-router";
import { useAdminServiceGetDashboard, useAdminServiceGetSystemInfo } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { DashboardStats } from "@/features/admin/DashboardStats";

export const Route = createLazyFileRoute("/_auth/admin/")({
  component: AdminDashboard,
});

function SystemInfoCard() {
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
    ["Version", sysData.version],
    ["OS", sysData.os],
    ["Arch", sysData.arch],
    ["Go Version", sysData.goVersion],
    ["FFmpeg", sysData.ffmpegVersion],
    ["Uptime", sysData.uptimeSeconds ? `${Math.floor(parseInt(sysData.uptimeSeconds) / 60)}m` : undefined],
  ];

  return (
    <div
      className="p-4 rounded-[var(--radius-lg)]"
      style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
    >
      <h3 className="text-sm font-semibold mb-3" style={{ color: "var(--text)" }}>
        System Info
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
  const { data: dashData, isLoading: dashLoading } = useAdminServiceGetDashboard();

  return (
    <div className="flex flex-col gap-8">
      <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
        Dashboard
      </h1>
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
