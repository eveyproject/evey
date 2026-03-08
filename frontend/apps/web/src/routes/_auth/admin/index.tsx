import { createFileRoute, Link, useRouterState } from "@tanstack/react-router";
import { useAdminServiceGetDashboard, useAdminServiceGetSystemInfo } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Card } from "@/components/ui/Card";
import { DashboardStats } from "@/features/admin/DashboardStats";

export const Route = createFileRoute("/_auth/admin/")({
  component: AdminDashboard,
});

const adminNavLinks = [
  { href: "/admin/libraries", label: "Libraries" },
  { href: "/admin/users", label: "Users" },
  { href: "/admin/plugins", label: "Plugins" },
  { href: "/admin/webhooks", label: "Webhooks" },
  { href: "/admin/audit", label: "Audit Log" },
  { href: "/admin/metadata", label: "Metadata" },
  { href: "/admin/activity", label: "Activity" },
  { href: "/admin/tasks", label: "Scheduled Tasks" },
  { href: "/admin/logs", label: "Logs" },
];

function AdminNav() {
  const { location } = useRouterState();
  const pathname = location.pathname;

  return (
    <div className="flex flex-wrap gap-2">
      {adminNavLinks.map(({ href, label }) => {
        const isActive = pathname === href || (href !== "/admin" && pathname.startsWith(href));
        return (
          <Link
            key={href}
            to={href}
            className="px-4 py-2 text-sm font-medium rounded-[var(--radius-md)] transition-colors"
            style={{
              backgroundColor: isActive ? "var(--accent)" : "var(--surface)",
              border: `1px solid ${isActive ? "var(--accent)" : "var(--border)"}`,
              color: isActive ? "white" : "var(--text)",
            }}
          >
            {label}
          </Link>
        );
      })}
    </div>
  );
}

function AdminDashboard() {
  const { data: dashData, isLoading: dashLoading } = useAdminServiceGetDashboard();
  const { data: sysData, isLoading: sysLoading } = useAdminServiceGetSystemInfo();

  return (
    <div className="flex flex-col gap-8">
      <div>
        <h1 className="text-2xl font-bold mb-2" style={{ color: "var(--text)" }}>
          Admin
        </h1>
        <AdminNav />
      </div>

      <section>
        <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text)" }}>
          Dashboard
        </h2>
        {dashLoading ? (
          <div className="flex justify-center py-8">
            <Spinner size="lg" />
          </div>
        ) : dashData ? (
          <DashboardStats info={dashData} />
        ) : null}
      </section>

      <section>
        <h2 className="text-lg font-semibold mb-4" style={{ color: "var(--text)" }}>
          System Info
        </h2>
        {sysLoading ? (
          <div className="flex justify-center py-8">
            <Spinner size="md" />
          </div>
        ) : sysData ? (
          <Card>
            <dl className="grid grid-cols-2 sm:grid-cols-3 gap-x-8 gap-y-3 text-sm">
              {(
                [
                  ["Version", sysData.version],
                  ["OS", sysData.os],
                  ["Arch", sysData.arch],
                  ["Go Version", sysData.goVersion],
                  ["FFmpeg", sysData.ffmpegVersion],
                  ["Uptime", sysData.uptimeSeconds ? `${Math.floor(parseInt(sysData.uptimeSeconds) / 60)}m` : undefined],
                ] as [string, string | undefined][]
              ).map(([k, v]) =>
                v ? (
                  <div key={k}>
                    <dt className="font-medium" style={{ color: "var(--text-muted)" }}>
                      {k}
                    </dt>
                    <dd style={{ color: "var(--text)" }}>{v}</dd>
                  </div>
                ) : null,
              )}
            </dl>
          </Card>
        ) : null}
      </section>
    </div>
  );
}
