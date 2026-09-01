import { useTranslation } from "react-i18next";
import { Clock } from "lucide-react";
import type { V1AuditLogEntry } from "@evey/api";
import { Pagination } from "@/components/ui/Pagination";
import { EmptyState } from "@/components/ui/EmptyState";

interface AuditLogTableProps {
  entries: V1AuditLogEntry[];
  total?: number;
  // Numbered-pager mode. Omit all three for infinite-scroll mode (the caller
  // renders its own sentinel below the table); only the total count shows.
  onPageChange?: (page: number) => void;
  page?: number;
  pageSize?: number;
}

const HIDDEN_DETAIL_KEYS = /(_id|_ids|^title|^method)$/;

function formatDetails(raw?: string): string {
  if (!raw) return "";
  try {
    const obj = JSON.parse(raw);
    return Object.entries(obj)
      .filter(([k]) => !HIDDEN_DETAIL_KEYS.test(k))
      .map(([k, v]) => `${k.replace(/_/g, " ")}: ${v}`)
      .join(", ");
  } catch {
    return raw;
  }
}

export function AuditLogTable({
  entries,
  total = 0,
  onPageChange,
  page,
  pageSize,
}: AuditLogTableProps) {
  const { t } = useTranslation();
  const paged = onPageChange != null && page != null && pageSize != null;
  const totalPages = Math.max(1, Math.ceil(total / (pageSize || 1)));

  return (
    <div className="flex flex-col gap-4">
      <div
        className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
      >
        <table className="w-full text-sm">
          <thead>
            <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
              <th className="px-4 py-2 text-left font-medium text-[var(--text-muted)]">{t("admin.audit.tableHeaders.time")}</th>
              <th className="px-4 py-2 text-left font-medium text-[var(--text-muted)] hidden sm:table-cell">{t("admin.audit.tableHeaders.user")}</th>
              <th className="px-4 py-2 text-left font-medium text-[var(--text-muted)]">{t("admin.audit.tableHeaders.action")}</th>
              <th className="px-4 py-2 text-left font-medium text-[var(--text-muted)] hidden md:table-cell">{t("admin.audit.tableHeaders.resource")}</th>
              <th className="px-4 py-2 text-left font-medium text-[var(--text-muted)] hidden lg:table-cell">{t("admin.audit.tableHeaders.details")}</th>
              <th className="px-4 py-2 text-left font-medium text-[var(--text-muted)] hidden lg:table-cell">{t("admin.audit.tableHeaders.device", "Device")}</th>
              <th className="px-4 py-2 text-left font-medium text-[var(--text-muted)] hidden md:table-cell">{t("admin.audit.tableHeaders.ip")}</th>
            </tr>
          </thead>
          <tbody>
            {entries.length === 0 ? (
              <tr>
                <td colSpan={7}>
                  <EmptyState icon={<Clock size={48} strokeWidth={1.5} />} title={t("admin.audit.noEntries")} />
                </td>
              </tr>
            ) : (
              entries.map((entry) => (
                <tr
                  key={entry.id}
                  className="border-b border-[var(--border)] hover:bg-[var(--surface-raised)] transition-colors-fast"
                >
                  <td className="px-4 py-2 whitespace-nowrap text-[var(--text-muted)]">
                    {entry.createdAt ? new Date(entry.createdAt).toLocaleString() : "-"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap text-[var(--text)] hidden sm:table-cell">
                    {entry.username ?? entry.userId ?? "-"}
                    {/* The profile is the half that tells two people on one
                        household account apart; server-side actions have none. */}
                    {entry.profileName && (
                      <span className="block text-xs text-[var(--text-muted)]">{entry.profileName}</span>
                    )}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap text-[var(--text)]">
                    {entry.action?.replace(/_/g, " ") ?? "-"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap text-[var(--text-muted)] hidden md:table-cell">
                    {entry.resourceType
                      ? `${entry.resourceType.replace(/_/g, " ")}${entry.resourceName ? `: ${entry.resourceName}` : ""}`
                      : "-"}
                  </td>
                  <td className="px-4 py-2 text-[var(--text-muted)] hidden lg:table-cell">
                    {formatDetails(entry.details) || "-"}
                  </td>
                  <td className="px-4 py-2 text-[var(--text-muted)] hidden lg:table-cell max-w-[200px] truncate" title={entry.userAgent ?? undefined}>
                    {entry.deviceName || entry.userAgent || "-"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap text-[var(--text-muted)] hidden md:table-cell">
                    {entry.ipAddress ?? "-"}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between">
        <span className="text-sm text-[var(--text-muted)]">
          {t("admin.audit.entries", { count: total })}
        </span>
        {paged && (
          <Pagination
            page={page + 1}
            totalPages={totalPages}
            onPageChange={(p) => onPageChange(p - 1)}
          />
        )}
      </div>
    </div>
  );
}
