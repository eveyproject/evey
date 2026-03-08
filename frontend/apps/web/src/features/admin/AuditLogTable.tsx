import { Button } from "@/components/ui/Button";
import type { V1AuditLogEntry } from "@evey/api/src/generated/admin.schemas";

interface AuditLogTableProps {
  entries: V1AuditLogEntry[];
  total?: number;
  onNextPage: () => void;
  onPrevPage: () => void;
  page: number;
  pageSize: number;
}

export function AuditLogTable({
  entries,
  total = 0,
  onNextPage,
  onPrevPage,
  page,
  pageSize,
}: AuditLogTableProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const hasPrev = page > 0;
  const hasNext = (page + 1) * pageSize < total;

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
                <td
                  colSpan={6}
                  className="px-4 py-8 text-center"
                  style={{ color: "var(--text-muted)" }}
                >
                  No entries found.
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
          Page {page + 1} of {totalPages} ({total} entries)
        </span>
        <div className="flex gap-2">
          <Button variant="secondary" onClick={onPrevPage} disabled={!hasPrev}>
            Previous
          </Button>
          <Button variant="secondary" onClick={onNextPage} disabled={!hasNext}>
            Next
          </Button>
        </div>
      </div>
    </div>
  );
}
