import { useTranslation } from "react-i18next";
import { Clock } from "lucide-react";
import type { V1AuditLogEntry } from "@evey/api";
import { Pagination } from "@/components/ui/Pagination";
import { EmptyState } from "@/components/ui/EmptyState";

interface AuditLogTableProps {
  entries: V1AuditLogEntry[];
  total?: number;
  onPageChange: (page: number) => void;
  page: number;
  pageSize: number;
}

export function AuditLogTable({
  entries,
  total = 0,
  onPageChange,
  page,
  pageSize,
}: AuditLogTableProps) {
  const { t } = useTranslation();
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className="flex flex-col gap-4">
      <div
        className="overflow-x-auto rounded-[var(--radius-lg)]"
        style={{ border: "1px solid var(--border)" }}
      >
        <table className="w-full text-sm">
          <thead>
            <tr style={{ backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
              {[t("admin.audit.tableHeaders.time"), t("admin.audit.tableHeaders.user"), t("admin.audit.tableHeaders.action"), t("admin.audit.tableHeaders.resource"), t("admin.audit.tableHeaders.details"), t("admin.audit.tableHeaders.ip")].map((h) => (
                <th
                  key={h}
                  className="px-4 py-2 text-left font-medium"
                  style={{ color: "var(--text-muted)" }}
                >
                  {h}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {entries.length === 0 ? (
              <tr>
                <td colSpan={6}>
                  <EmptyState icon={<Clock size={48} strokeWidth={1.5} />} title={t("admin.audit.noEntries")} />
                </td>
              </tr>
            ) : (
              entries.map((entry) => (
                <tr
                  key={entry.id}
                  style={{ borderBottom: "1px solid var(--border)" }}
                  className="hover:bg-[var(--surface-raised)] transition-colors-fast"
                >
                  <td className="px-4 py-2 whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
                    {entry.createdAt ? new Date(entry.createdAt).toLocaleString() : "\u2014"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap" style={{ color: "var(--text)" }}>
                    {entry.username ?? entry.userId ?? "\u2014"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap" style={{ color: "var(--text)" }}>
                    {entry.action ?? "\u2014"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
                    {entry.resourceType ? `${entry.resourceType}/${entry.resourceId ?? ""}` : "\u2014"}
                  </td>
                  <td className="px-4 py-2 max-w-xs truncate" style={{ color: "var(--text-muted)" }}>
                    {entry.details ?? "\u2014"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
                    {entry.ipAddress ?? "\u2014"}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between">
        <span className="text-sm" style={{ color: "var(--text-muted)" }}>
          {t("admin.audit.entries", { count: total })}
        </span>
        <Pagination
          page={page + 1}
          totalPages={totalPages}
          onPageChange={(p) => onPageChange(p - 1)}
        />
      </div>
    </div>
  );
}
