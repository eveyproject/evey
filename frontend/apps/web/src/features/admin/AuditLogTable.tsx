import type { V1AuditLogEntry } from "@evey/api/src/generated/admin.schemas";
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
              {["Time", "User", "Action", "Resource", "Details", "IP"].map((h) => (
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
                  <EmptyState icon="history" title="No entries found" />
                </td>
              </tr>
            ) : (
              entries.map((entry) => (
                <tr
                  key={entry.id}
                  style={{ borderBottom: "1px solid var(--border)" }}
                  className="hover:bg-[var(--surface-raised)] transition-colors"
                >
                  <td className="px-4 py-2 whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
                    {entry.createdAt ? new Date(entry.createdAt).toLocaleString() : "—"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap" style={{ color: "var(--text)" }}>
                    {entry.username ?? entry.userId ?? "—"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap" style={{ color: "var(--text)" }}>
                    {entry.action ?? "—"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
                    {entry.resourceType ? `${entry.resourceType}/${entry.resourceId ?? ""}` : "—"}
                  </td>
                  <td className="px-4 py-2 max-w-xs truncate" style={{ color: "var(--text-muted)" }}>
                    {entry.details ?? "—"}
                  </td>
                  <td className="px-4 py-2 whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
                    {entry.ipAddress ?? "—"}
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between">
        <span className="text-sm" style={{ color: "var(--text-muted)" }}>
          {total} entries
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
