import { useState } from "react";
import { ScanSearch } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import {
  useLibraryServiceListLibraries,
  useAdminMediaServiceScanSeekIndex,
  useAdminMediaServiceReindexMediaSeekIndex,
  useAdminMediaServiceRollbackMediaSeekIndex,
  useAdminMediaServiceReindexLibrarySeekIndex,
  useAdminMediaServiceRollbackLibrarySeekIndex,
} from "@evey/api";
import type { V1SeekIndexItem } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { PageHeader } from "@/components/ui/PageHeader";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Card } from "@/components/ui/Card";
import { Modal } from "@/components/ui/Modal";
import { Select } from "@/components/ui/Select";
import { Checkbox } from "@/components/ui/Checkbox";
import { EmptyState } from "@/components/ui/EmptyState";

export const Route = createLazyFileRoute("/_auth/admin/seek-index")({
  component: SeekIndexPage,
});

function formatSize(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

function errText(err: { message?: string } | null | undefined, fallback: string): string {
  return err?.message || fallback;
}

function SeekIndexPage() {
  const { t } = useTranslation();
  const [libraryId, setLibraryId] = useState("");
  const [onlyAttention, setOnlyAttention] = useState(true);
  const [confirmBatch, setConfirmBatch] = useState<"repair" | "rollback" | null>(null);

  const { data: libData } = useLibraryServiceListLibraries();
  const libraries = libData?.libraries ?? [];

  // Scanning walks every MKV/WebM of the scope on disk — manual trigger only.
  const scan = useAdminMediaServiceScanSeekIndex(libraryId ? { libraryId } : undefined, {
    query: { enabled: false, retry: false },
  });
  const result = scan.data;
  const items = result?.items ?? [];
  const missing = result?.missingIndex ?? 0;
  const withBackup = result?.withBackup ?? 0;

  const refreshScan = () => void scan.refetch();

  const repairOne = useAdminMediaServiceReindexMediaSeekIndex({
    mutation: {
      onSuccess: (res) => {
        if (res.skipped) toast.info(t("admin.seekIndex.skippedAlreadyIndexed"));
        else toast.success(t("admin.seekIndex.repaired"));
        refreshScan();
      },
      onError: (err) => toast.error(errText(err, t("admin.seekIndex.repairFailed"))),
    },
  });

  const rollbackOne = useAdminMediaServiceRollbackMediaSeekIndex({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.seekIndex.rolledBack"));
        refreshScan();
      },
      onError: (err) => toast.error(errText(err, t("admin.seekIndex.rollbackFailed"))),
    },
  });

  const repairLib = useAdminMediaServiceReindexLibrarySeekIndex({
    mutation: {
      onSuccess: (res) => {
        const queued = res.queued ?? 0;
        if (queued === 0) toast.info(t("admin.seekIndex.nothingQueued"));
        else toast.success(t("admin.seekIndex.queuedRepair", { count: queued }));
      },
      onError: (err) => toast.error(errText(err, t("admin.seekIndex.repairFailed"))),
    },
  });

  const rollbackLib = useAdminMediaServiceRollbackLibrarySeekIndex({
    mutation: {
      onSuccess: (res) => {
        const queued = res.queued ?? 0;
        if (queued === 0) toast.info(t("admin.seekIndex.nothingQueued"));
        else toast.success(t("admin.seekIndex.queuedRollback", { count: queued }));
      },
      onError: (err) => toast.error(errText(err, t("admin.seekIndex.rollbackFailed"))),
    },
  });

  const visible = onlyAttention ? items.filter((it) => !it.hasIndex || it.hasBackup) : items;

  function rowRepairPending(it: V1SeekIndexItem): boolean {
    return repairOne.isPending && repairOne.variables?.mediaId === it.mediaId;
  }
  function rowRollbackPending(it: V1SeekIndexItem): boolean {
    return rollbackOne.isPending && rollbackOne.variables?.mediaId === it.mediaId;
  }

  return (
    <div className="flex flex-col gap-6" data-testid="seek-index-page">
      <PageHeader title={t("admin.seekIndex.title")} />

      <Card>
        <div className="flex flex-col gap-2 text-sm text-[var(--text-muted)]">
          <p>{t("admin.seekIndex.description")}</p>
          <p data-testid="seek-index-ro-note">{t("admin.seekIndex.readOnlyNote")}</p>
        </div>
      </Card>

      <div className="flex flex-wrap items-end gap-3">
        <div className="w-64">
          <Select
            label={t("admin.seekIndex.library")}
            value={libraryId}
            onChange={setLibraryId}
            options={[
              { value: "", label: t("admin.seekIndex.allLibraries") },
              ...libraries.map((l) => ({ value: l.id ?? "", label: l.name ?? l.id ?? "" })),
            ]}
          />
        </div>
        <Button onClick={refreshScan} loading={scan.isFetching} data-testid="seek-index-scan-btn">
          {result ? t("admin.seekIndex.scanAgain") : t("admin.seekIndex.scan")}
        </Button>
        {result && (
          <>
            <Button
              variant="secondary"
              disabled={missing === 0}
              onClick={() => setConfirmBatch("repair")}
              data-testid="seek-index-repair-all-btn"
            >
              {t("admin.seekIndex.repairAll", { count: missing })}
            </Button>
            <Button
              variant="secondary"
              disabled={withBackup === 0}
              onClick={() => setConfirmBatch("rollback")}
              data-testid="seek-index-rollback-all-btn"
            >
              {t("admin.seekIndex.rollbackAll", { count: withBackup })}
            </Button>
          </>
        )}
      </div>

      {scan.isError && (
        <p className="text-sm text-[var(--color-danger)]" role="alert">
          {errText(scan.error, t("admin.seekIndex.scanFailed"))}
        </p>
      )}

      {scan.isFetching ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : !result ? (
        <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
          <EmptyState
            icon={<ScanSearch size={48} strokeWidth={1.5} />}
            title={t("admin.seekIndex.emptyTitle")}
            description={t("admin.seekIndex.emptyDescription")}
          />
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
            <Card>
              <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                {t("admin.seekIndex.statsInspected")}
              </dt>
              <dd className="text-xl font-bold text-[var(--text)]">{result.total ?? 0}</dd>
            </Card>
            <Card>
              <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                {t("admin.seekIndex.statsMissing")}
              </dt>
              <dd className="text-xl font-bold text-[var(--text)]" data-testid="seek-index-missing-count">
                {missing}
              </dd>
            </Card>
            <Card>
              <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                {t("admin.seekIndex.statsBackup")}
              </dt>
              <dd className="text-xl font-bold text-[var(--text)]">{withBackup}</dd>
            </Card>
          </div>

          <Checkbox
            label={t("admin.seekIndex.onlyAttention")}
            checked={onlyAttention}
            onChange={setOnlyAttention}
          />

          {items.length === 0 ? (
            <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
              <EmptyState title={t("admin.seekIndex.noEbml")} />
            </div>
          ) : visible.length === 0 ? (
            <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
              <EmptyState title={t("admin.seekIndex.allGood")} />
            </div>
          ) : (
            <div className="overflow-x-auto rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-[var(--border)]">
                    <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
                      {t("admin.seekIndex.tableHeaders.file")}
                    </th>
                    <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
                      {t("admin.seekIndex.tableHeaders.container")}
                    </th>
                    <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
                      {t("admin.seekIndex.tableHeaders.size")}
                    </th>
                    <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
                      {t("admin.seekIndex.tableHeaders.status")}
                    </th>
                    <th className="px-4 py-3 w-44" />
                  </tr>
                </thead>
                <tbody>
                  {visible.map((it) => (
                    <tr key={it.mediaId} className="border-b border-[var(--border)]" data-testid="seek-index-row">
                      <td className="px-4 py-3">
                        <div className="text-[var(--text)]">{it.title || it.path}</div>
                        <div
                          className="font-mono text-xs text-[var(--text-muted)] truncate max-w-[280px] sm:max-w-[480px]"
                          title={it.path}
                        >
                          {it.path}
                        </div>
                      </td>
                      <td className="px-4 py-3">
                        <Badge>{(it.container ?? "").toUpperCase()}</Badge>
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap text-[var(--text-muted)]">
                        {formatSize(Number(it.sizeBytes ?? 0))}
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap">
                        {it.hasIndex ? (
                          <Badge variant="success">{t("admin.seekIndex.statusIndexed")}</Badge>
                        ) : (
                          <Badge variant="warning">{t("admin.seekIndex.statusMissing")}</Badge>
                        )}
                        {it.hasBackup && (
                          <Badge variant="info" className="ml-1.5">
                            {t("admin.seekIndex.backupBadge")}
                          </Badge>
                        )}
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap">
                        <div className="flex items-center justify-end gap-2">
                          {!it.hasIndex && (
                            <Button
                              variant="secondary"
                              className="text-xs"
                              loading={rowRepairPending(it)}
                              onClick={() => it.mediaId && repairOne.mutate({ mediaId: it.mediaId })}
                              data-testid="seek-index-repair-btn"
                            >
                              {t("admin.seekIndex.repair")}
                            </Button>
                          )}
                          {it.hasBackup && (
                            <Button
                              variant="ghost"
                              className="text-xs"
                              loading={rowRollbackPending(it)}
                              onClick={() => it.mediaId && rollbackOne.mutate({ mediaId: it.mediaId })}
                              data-testid="seek-index-rollback-btn"
                            >
                              {t("admin.seekIndex.rollback")}
                            </Button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      )}

      <Modal
        open={confirmBatch !== null}
        onClose={() => setConfirmBatch(null)}
        title={
          confirmBatch === "rollback"
            ? t("admin.seekIndex.rollbackAllConfirmTitle")
            : t("admin.seekIndex.repairAllConfirmTitle")
        }
      >
        <p className="text-sm text-[var(--text-muted)] mb-4">
          {confirmBatch === "rollback"
            ? t("admin.seekIndex.rollbackAllConfirm", { count: withBackup })
            : t("admin.seekIndex.repairAllConfirm", { count: missing })}
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setConfirmBatch(null)}>
            {t("common.cancel")}
          </Button>
          <Button
            loading={repairLib.isPending || rollbackLib.isPending}
            onClick={() => {
              const req = { data: libraryId ? { libraryId } : {} };
              if (confirmBatch === "rollback") rollbackLib.mutate(req);
              else repairLib.mutate(req);
              setConfirmBatch(null);
            }}
            data-testid="seek-index-confirm-batch-btn"
          >
            {t("common.confirm")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
