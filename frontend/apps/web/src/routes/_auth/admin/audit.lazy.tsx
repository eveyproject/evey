import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useAdminServiceGetAuditLog } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { AuditLogTable } from "@/features/admin/AuditLogTable";

export const Route = createLazyFileRoute("/_auth/admin/audit")({
  component: AuditPage,
});

const PAGE_SIZE = 50;

function AuditPage() {
  const { t } = useTranslation();
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
        {t("admin.audit.title")}
      </h1>

      <div
        className="flex flex-wrap items-end gap-3 p-4 rounded-[var(--radius-lg)]"
        style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
      >
        <Input
          label={t("admin.audit.action")}
          value={actionFilter}
          onChange={(e) => setActionFilter(e.target.value)}
          placeholder="e.g. login"
          className="w-48"
        />
        <Input
          label={t("admin.audit.resourceType")}
          value={resourceFilter}
          onChange={(e) => setResourceFilter(e.target.value)}
          placeholder="e.g. library"
          className="w-48"
        />
        <div className="flex gap-2">
          <Button onClick={applyFilters}>{t("common.apply")}</Button>
          <Button variant="secondary" onClick={clearFilters}>
            {t("common.clear")}
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
          onPageChange={setPage}
        />
      )}
    </div>
  );
}
