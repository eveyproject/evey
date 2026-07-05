import { Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useAdminSessionServiceGetAuditLog } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { Card } from "@/components/ui/Card";
import { Spinner } from "@/components/ui/Spinner";

export function RecentActivity() {
  const { t } = useTranslation();
  const { data, isLoading } = useAdminSessionServiceGetAuditLog({ limit: 8, offset: 0 });
  const entries = data?.entries ?? [];
  const total = data?.total ?? 0;

  return (
    <Card>
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-baseline gap-2">
          <h3 className="text-sm font-semibold text-[var(--text)]">
            {t("admin.dashboardStats.recentActivity")}
          </h3>
          {total > 0 && <span className="text-xs text-[var(--text-muted)]">({total})</span>}
        </div>
        <Link
          to="/admin/activity"
          className="text-xs font-medium hover:underline text-[var(--accent)]"
        >
          {t("admin.dashboardStats.viewAll")}
        </Link>
      </div>
      {isLoading ? (
        <div className="flex justify-center py-8">
          <Spinner size="md" />
        </div>
      ) : entries.length === 0 ? (
        <p className="text-sm text-[var(--text-muted)] py-4 text-center">{t("admin.audit.noEntries")}</p>
      ) : (
      <div
        className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
      >
        <table className="w-full text-sm">
          <thead>
            <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
              {[t("admin.audit.tableHeaders.time"), t("admin.audit.tableHeaders.user"), t("admin.audit.tableHeaders.action"), t("admin.audit.tableHeaders.resource"), t("admin.audit.tableHeaders.details")].map((h) => (
                <th key={h} className="px-3 py-2 text-left font-medium text-[var(--text-muted)]">
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {entries.map((entry) => {
              const isFailed = entry.details?.includes('"failed"') ?? false;
              return (
                <tr
                  key={entry.id}
                  className={`border-b border-[var(--border)] hover:bg-[var(--surface-raised)] transition-colors-fast ${isFailed ? "bg-[color-mix(in_srgb,var(--danger-bg)_10%,transparent)]" : ""}`}
                >
                  <td className="px-3 py-2 whitespace-nowrap text-xs text-[var(--text-muted)]">
                    {entry.createdAt ? new Date(entry.createdAt).toLocaleString() : "-"}
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap text-[var(--text)]">
                    {entry.username || (isFailed ? entry.details?.match(/"username":"([^"]+)"/)?.[1] : null) || "-"}
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap">
                    <Badge variant={isFailed ? "error" : "info"}>{entry.action ?? "-"}</Badge>
                  </td>
                  <td className="px-3 py-2 whitespace-nowrap text-xs text-[var(--text-muted)]">
                    {entry.resourceType ?? "-"}
                  </td>
                  <td className="px-3 py-2 text-xs text-[var(--text-muted)]">
                    {isFailed ? t("admin.dashboardStats.loginFailed") : "-"}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
      )}
    </Card>
  );
}
