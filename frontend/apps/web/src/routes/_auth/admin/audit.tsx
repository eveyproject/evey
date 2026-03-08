import { useState } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useAdminServiceGetAuditLog } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { AuditLogTable } from "@/features/admin/AuditLogTable";

export const Route = createFileRoute("/_auth/admin/audit")({
  component: AuditPage,
});

const PAGE_SIZE = 50;

function AuditPage() {
  const [page, setPage] = useState(0);
  const [actionFilter, setActionFilter] = useState("");
  const [resourceFilter, setResourceFilter] = useState("");
  const [activeFilters, setActiveFilters] = useState<{ action?: string; resourceType?: string }>({});

  const { data, isLoading } = useAdminServiceGetAuditLog({
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
    ...(activeFilters.action ? { action: activeFilters.action } : {}),
    ...(activeFilters.resourceType ? { resourceType: activeFilters.resourceType } : {}),
  });

  const entries = data?.entries ?? [];
  const total = data?.total ?? 0;

  function applyFilters() {
    setPage(0);
    setActiveFilters({
      action: actionFilter.trim() || undefined,
      resourceType: resourceFilter.trim() || undefined,
    });
  }

  function clearFilters() {
    setActionFilter("");
    setResourceFilter("");
    setActiveFilters({});
    setPage(0);
  }

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
        Audit Log
      </h1>

      <div
        className="flex flex-wrap items-end gap-3 p-4 rounded-[var(--radius-lg)]"
        style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
      >
        <Input
          label="Action"
          value={actionFilter}
          onChange={(e) => setActionFilter(e.target.value)}
          placeholder="e.g. login"
          className="w-48"
        />
        <Input
          label="Resource Type"
          value={resourceFilter}
          onChange={(e) => setResourceFilter(e.target.value)}
          placeholder="e.g. library"
          className="w-48"
        />
        <div className="flex gap-2">
          <Button onClick={applyFilters}>Apply</Button>
          <Button variant="secondary" onClick={clearFilters}>
            Clear
          </Button>
        </div>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <AuditLogTable
          entries={entries}
          total={total}
          page={page}
          pageSize={PAGE_SIZE}
          onNextPage={() => setPage((p) => p + 1)}
          onPrevPage={() => setPage((p) => Math.max(0, p - 1))}
        />
      )}
    </div>
  );
}
