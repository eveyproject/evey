import { useRef, useState } from "react";
import { Folder } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useAuthStore, toast } from "@evey/store";
import {
  eveyFetchRaw,
  useBackupServiceListBackups,
  backupServiceCreateBackup,
  backupServiceDeleteBackup,
  getBackupServiceListBackupsQueryKey,
  backupScheduleServiceGetBackupSchedule,
  getBackupScheduleServiceGetBackupScheduleQueryKey,
  backupScheduleServiceUpdateBackupSchedule,
  backupScheduleServiceRunBackupNow,
  apiErrorMessage,
} from "@evey/api";
import type { V1BackupManifest, V1BackupSchedule } from "@evey/api";
import { formatBytes } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Checkbox } from "@/components/ui/Checkbox";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { PageHeader } from "@/components/ui/PageHeader";
import { Modal } from "@/components/ui/Modal";

export const Route = createLazyFileRoute("/_auth/admin/backup")({
  component: BackupPage,
});

type Backup = V1BackupManifest;

const BACKUPS_QUERY_KEY = getBackupServiceListBackupsQueryKey();
const SCHEDULE_QUERY_KEY = getBackupScheduleServiceGetBackupScheduleQueryKey();

function isValidCron(expr: string): boolean {
  const parts = expr.trim().split(/\s+/);
  return parts.length === 5;
}

function formatRunTime(iso: string | undefined, t: (k: string, opts?: Record<string, unknown>) => string, neverKey: string, timeKey: string): string {
  if (!iso) return t(neverKey);
  return t(timeKey, { time: new Date(iso).toLocaleString() });
}

function ScheduleSection() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();

  const { data: schedule, isLoading } = useQuery({
    queryKey: SCHEDULE_QUERY_KEY,
    queryFn: ({ signal }) => backupScheduleServiceGetBackupSchedule(signal),
  });

  const [cron, setCron] = useState<string | undefined>(undefined);
  const [retention, setRetention] = useState<number | undefined>(undefined);
  const [enabled, setEnabled] = useState<boolean | undefined>(undefined);

  // Initialize local state from server data once loaded, only if user hasn't touched them yet
  const effectiveCron = cron ?? schedule?.cronExpression ?? "";
  const effectiveRetention = retention ?? schedule?.retentionDays ?? 7;
  const effectiveEnabled = enabled ?? schedule?.enabled ?? false;

  const dirty =
    cron !== undefined ||
    retention !== undefined ||
    enabled !== undefined;

  const cronError = effectiveCron && !isValidCron(effectiveCron)
    ? t("admin.backup.schedule.cronInvalid")
    : undefined;

  const saveMutation = useMutation({
    mutationFn: () =>
      backupScheduleServiceUpdateBackupSchedule({
        cronExpression: effectiveCron,
        retentionDays: effectiveRetention,
        enabled: effectiveEnabled,
      }),
    onSuccess: (data: V1BackupSchedule) => {
      queryClient.setQueryData(SCHEDULE_QUERY_KEY, data);
      setCron(undefined);
      setRetention(undefined);
      setEnabled(undefined);
      toast.success(t("admin.backup.schedule.saved"));
    },
    onError: (err: Error) =>
      toast.error(apiErrorMessage(err, t, t("admin.backup.schedule.saveFailed"))),
  });

  const runMutation = useMutation({
    mutationFn: () => backupScheduleServiceRunBackupNow({}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: BACKUPS_QUERY_KEY });
      queryClient.invalidateQueries({ queryKey: SCHEDULE_QUERY_KEY });
      toast.success(t("admin.backup.schedule.runStarted"));
    },
    onError: (err: Error) =>
      toast.error(apiErrorMessage(err, t, t("admin.backup.schedule.runFailed"))),
  });

  const lastStatus = schedule?.lastStatus ?? "";
  const statusBadge =
    lastStatus === "success" ? (
      <Badge variant="success">{t("admin.backup.schedule.statusSuccess")}</Badge>
    ) : lastStatus === "failed" ? (
      <span title={schedule?.lastError ? t("admin.backup.schedule.lastError", { message: schedule.lastError }) : undefined}>
        <Badge variant="error">{t("admin.backup.schedule.statusFailed")}</Badge>
      </span>
    ) : (
      <span className="text-xs text-[var(--text-muted)]">{t("admin.backup.schedule.statusUnknown")}</span>
    );

  return (
    <Card>
      <div className="flex flex-col gap-4">
        <div>
          <h2 className="text-base font-semibold text-[var(--text)]">{t("admin.backup.schedule.title")}</h2>
          <p className="text-sm text-[var(--text-muted)] mt-0.5">{t("admin.backup.schedule.description")}</p>
        </div>

        {isLoading ? (
          <div className="flex justify-center py-6">
            <Spinner size="sm" />
          </div>
        ) : (
          <>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="flex flex-col gap-1">
                <Input
                  label={t("admin.backup.schedule.cronLabel")}
                  value={effectiveCron}
                  onChange={(e) => setCron(e.target.value)}
                  placeholder="0 3 * * *"
                  error={cronError}
                />
                <span className="text-xs text-[var(--text-muted)]">{t("admin.backup.schedule.cronHelp")}</span>
              </div>
              <div className="flex flex-col gap-1">
                <Input
                  label={t("admin.backup.schedule.retentionLabel")}
                  type="number"
                  min={1}
                  value={effectiveRetention}
                  onChange={(e) => setRetention(Number(e.target.value))}
                />
                <span className="text-xs text-[var(--text-muted)]">{t("admin.backup.schedule.retentionHelp")}</span>
              </div>
            </div>

            <Checkbox
              label={t("admin.backup.schedule.enabledLabel")}
              checked={effectiveEnabled}
              onChange={(v) => setEnabled(v)}
            />

            <div className="flex flex-wrap items-center gap-4 text-sm text-[var(--text-muted)]">
              <span>{formatRunTime(schedule?.lastRunAt, t, "admin.backup.schedule.lastRunNever", "admin.backup.schedule.lastRun")}</span>
              <span>{formatRunTime(schedule?.nextRunAt, t, "admin.backup.schedule.nextRunNotScheduled", "admin.backup.schedule.nextRun")}</span>
              <span className="flex items-center gap-1.5">{statusBadge}</span>
            </div>

            <div className="flex gap-2">
              <Button
                loading={saveMutation.isPending}
                disabled={!dirty || !!cronError}
                onClick={() => saveMutation.mutate()}
              >
                {t("admin.backup.schedule.save")}
              </Button>
              <Button
                variant="secondary"
                loading={runMutation.isPending}
                onClick={() => runMutation.mutate()}
              >
                {t("admin.backup.schedule.runNow")}
              </Button>
            </div>
          </>
        )}
      </div>
    </Card>
  );
}

function useBackups() {
  return useBackupServiceListBackups({
    query: { select: (d) => d.backups ?? [] },
  });
}

function useCreateBackup() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => backupServiceCreateBackup({}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: BACKUPS_QUERY_KEY });
      toast.success(t("admin.backup.backupCreated"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.backup.failedCreate"))),
  });
}

function useDeleteBackup() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => backupServiceDeleteBackup(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: BACKUPS_QUERY_KEY });
      toast.success(t("admin.backup.backupDeleted"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.backup.failedDeleteBackup"))),
  });
}

function BackupRow({ backup }: { backup: Backup }) {
  const { t } = useTranslation();
  const token = useAuthStore((s) => s.accessToken);
  const deleteBackup = useDeleteBackup();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [confirmRestore, setConfirmRestore] = useState(false);
  const [restoring, setRestoring] = useState(false);

  function handleDownload() {
    if (!backup.id) return;
    const a = document.createElement("a");
    a.href = `/api/v1/admin/backups/${backup.id}`;
    if (token) {
      a.href += `?token=${encodeURIComponent(token)}`;
    }
    a.download = `${backup.id}.tar.gz`;
    a.click();
  }

  async function handleRestore() {
    if (!backup.id) return;
    setRestoring(true);
    try {
      const fileRes = await eveyFetchRaw({ url: `/api/v1/admin/backups/${backup.id}`, method: "GET" });
      if (!fileRes.ok) throw new Error((await fileRes.text()) || t("admin.backup.restoreFailed"));
      const blob = await fileRes.blob();
      const formData = new FormData();
      formData.append("backup", blob, `${backup.id}.tar.gz`);
      const restoreRes = await eveyFetchRaw({ url: "/api/v1/admin/backups/restore", method: "POST", body: formData });
      if (!restoreRes.ok) throw new Error((await restoreRes.text()) || t("admin.backup.restoreFailed"));
      toast.success(t("admin.backup.restoreCompleted"));
    } catch (err) {
      toast.error(apiErrorMessage(err as Error, t, t("admin.backup.restoreFailed")));
    } finally {
      setRestoring(false);
    }
  }

  return (
    <tr className="border-b border-[var(--border)]">
      <td data-label={t("admin.backup.tableHeaders.id")} className="px-4 py-3 font-mono text-xs text-[var(--text)]">
        {backup.id}
      </td>
      <td data-label={t("admin.backup.tableHeaders.date")} className="px-4 py-3 text-sm whitespace-nowrap text-[var(--text-muted)]">
        {backup.createdAt ? new Date(backup.createdAt).toLocaleString() : ""}
      </td>
      <td data-label={t("admin.backup.tableHeaders.version")} className="px-4 py-3 text-sm text-[var(--text-muted)]">
        {backup.version}
      </td>
      <td data-label={t("admin.backup.tableHeaders.size")} className="px-4 py-3 text-sm whitespace-nowrap text-[var(--text-muted)]">
        {formatBytes(Number(backup.size ?? 0))}
      </td>
      <td data-label={t("admin.backup.tableHeaders.status")} className="px-4 py-3">
        <Badge variant="success">{t("admin.backup.ready")}</Badge>
      </td>
      <td data-label={t("admin.backup.tableHeaders.actions")} className="px-4 py-3">
        <div className="flex gap-2">
          <Button variant="secondary" onClick={handleDownload} className="text-xs">
            {t("common.download")}
          </Button>
          <Button
            variant="secondary"
            loading={restoring}
            onClick={() => setConfirmRestore(true)}
            className="text-xs"
          >
            {t("common.restore")}
          </Button>
          <Button
            variant="danger"
            loading={deleteBackup.isPending}
            onClick={() => setConfirmDelete(true)}
            className="text-xs"
          >
            {t("common.delete")}
          </Button>
        </div>
        <Modal
          open={confirmRestore}
          onClose={() => setConfirmRestore(false)}
          title={t("admin.backup.restoreBackupTitle")}
        >
          <p className="text-sm mb-4 text-[var(--text-muted)]">
            {t("admin.backup.restoreConfirm", { date: backup.createdAt ? new Date(backup.createdAt).toLocaleString() : "" })}
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setConfirmRestore(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              loading={restoring}
              onClick={() => {
                setConfirmRestore(false);
                handleRestore();
              }}
            >
              {t("common.restore")}
            </Button>
          </div>
        </Modal>
        <Modal
          open={confirmDelete}
          onClose={() => setConfirmDelete(false)}
          title={t("admin.backup.deleteBackupTitle")}
        >
          <p className="text-sm mb-4 text-[var(--text-muted)]">
            {t("admin.backup.deleteConfirm", { date: backup.createdAt ? new Date(backup.createdAt).toLocaleString() : "" })}
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setConfirmDelete(false)}>
              {t("common.cancel")}
            </Button>
            <Button
              variant="danger"
              loading={deleteBackup.isPending}
              onClick={() => {
                if (backup.id) deleteBackup.mutate(backup.id);
                setConfirmDelete(false);
              }}
            >
              {t("common.delete")}
            </Button>
          </div>
        </Modal>
      </td>
    </tr>
  );
}

function RestoreUploadModal({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [restoring, setRestoring] = useState(false);

  async function handleUpload() {
    const file = fileInputRef.current?.files?.[0];
    if (!file) {
      toast.error(t("admin.backup.selectFileFirst"));
      return;
    }
    setRestoring(true);
    try {
      const formData = new FormData();
      formData.append("backup", file);
      const restoreRes = await eveyFetchRaw({ url: "/api/v1/admin/backups/restore", method: "POST", body: formData });
      if (!restoreRes.ok) throw new Error((await restoreRes.text()) || t("admin.backup.restoreFailed"));
      toast.success(t("admin.backup.restoreCompleted"));
      onClose();
    } catch (err) {
      toast.error(apiErrorMessage(err as Error, t, t("admin.backup.restoreFailed")));
    } finally {
      setRestoring(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("admin.backup.restoreFromFileTitle")}>
      <div className="flex flex-col gap-4">
        <p className="text-sm text-[var(--text-muted)]">
          {t("admin.backup.restoreFromFileDesc")}
        </p>
        <input
          ref={fileInputRef}
          type="file"
          accept=".tar.gz,.gz"
          className="text-sm file:mr-3 file:rounded-[var(--radius-lg)] file:border file:border-solid file:border-[var(--border)] file:bg-[var(--surface)] file:px-4 file:py-2 file:text-sm file:font-medium file:text-[var(--text)] file:cursor-pointer hover:file:bg-[var(--accent)] hover:file:text-[var(--filter-active-text)] file:transition-colors-fast text-[var(--text-muted)]"
        />
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>{t("common.cancel")}</Button>
          <Button loading={restoring} onClick={handleUpload}>
            {t("common.restore")}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function BackupPage() {
  const { t } = useTranslation();
  const { data: backups, isLoading, isError, error, refetch } = useBackups();
  const createBackup = useCreateBackup();
  const [showRestoreUpload, setShowRestoreUpload] = useState(false);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.backup.title")} actions={
        <div className="flex gap-2">
          <Button
            variant="secondary"
            onClick={() => setShowRestoreUpload(true)}
          >
            {t("admin.backup.restoreFromFile")}
          </Button>
          <Button
            loading={createBackup.isPending}
            onClick={() => createBackup.mutate()}
          >
            {t("admin.backup.createBackup")}
          </Button>
        </div>
      } />

      <ScheduleSection />

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : isError ? (
        // "No backups" tells an admin there is nothing to restore, which is the
        // last thing to get wrong on this screen.
        <FetchErrorState error={error} onRetry={() => void refetch()} />
      ) : !backups || backups.length === 0 ? (
        <EmptyState
          icon={<Folder size={48} strokeWidth={1.5} />}
          title={t("admin.backup.noBackups")}
          description={t("admin.backup.noBackupsDesc")}
        />
      ) : (
        <div
          className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]"
        >
          <table className="w-full text-sm table-cards">
            <thead>
              <tr
                className="bg-[var(--surface)] border-b border-[var(--border)]"
              >
                {[t("admin.backup.tableHeaders.id"), t("admin.backup.tableHeaders.date"), t("admin.backup.tableHeaders.version"), t("admin.backup.tableHeaders.size"), t("admin.backup.tableHeaders.status"), t("admin.backup.tableHeaders.actions")].map((h) => (
                  <th
                    key={h}
                    className="px-4 py-2 text-left font-medium text-[var(--text-muted)]"
                  >
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {backups.map((b) => (
                <BackupRow key={b.id} backup={b} />
              ))}
            </tbody>
          </table>
        </div>
      )}

      <RestoreUploadModal
        open={showRestoreUpload}
        onClose={() => setShowRestoreUpload(false)}
      />
    </div>
  );
}
