import { useRef, useState } from "react";
import { Folder } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useAuthStore, toast } from "@evey/store";
import {
  eveyFetchRaw,
  useBackupServiceListBackups,
  backupServiceCreateBackup,
  backupServiceDeleteBackup,
  getBackupServiceListBackupsQueryKey,
} from "@evey/api";
import type { V1BackupManifest } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";
import { PageHeader } from "@/components/ui/PageHeader";
import { Modal } from "@/components/ui/Modal";

export const Route = createLazyFileRoute("/_auth/admin/backup")({
  component: BackupPage,
});

type Backup = V1BackupManifest;

const BACKUPS_QUERY_KEY = getBackupServiceListBackupsQueryKey();

function formatSize(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
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
    onError: (err: Error) => toast.error(err.message || t("admin.backup.failedCreate")),
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
    onError: (err: Error) => toast.error(err.message || t("admin.backup.failedDeleteBackup")),
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
      const blob = await fileRes.blob();
      const formData = new FormData();
      formData.append("backup", blob, `${backup.id}.tar.gz`);
      await eveyFetchRaw({ url: "/api/v1/admin/backups/restore", method: "POST", body: formData });
      toast.success(t("admin.backup.restoreCompleted"));
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("admin.backup.restoreFailed"));
    } finally {
      setRestoring(false);
    }
  }

  return (
    <tr className="border-b border-[var(--border)]">
      <td className="px-4 py-3 font-mono text-xs text-[var(--text)]">
        {backup.id}
      </td>
      <td className="px-4 py-3 text-sm whitespace-nowrap text-[var(--text-muted)]">
        {backup.createdAt ? new Date(backup.createdAt).toLocaleString() : ""}
      </td>
      <td className="px-4 py-3 text-sm text-[var(--text-muted)]">
        {backup.version}
      </td>
      <td className="px-4 py-3 text-sm whitespace-nowrap text-[var(--text-muted)]">
        {formatSize(Number(backup.size ?? 0))}
      </td>
      <td className="px-4 py-3">
        <Badge variant="success">{t("admin.backup.ready")}</Badge>
      </td>
      <td className="px-4 py-3">
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
      await eveyFetchRaw({ url: "/api/v1/admin/backups/restore", method: "POST", body: formData });
      toast.success(t("admin.backup.restoreCompleted"));
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("admin.backup.restoreFailed"));
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
  const { data: backups, isLoading } = useBackups();
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

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
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
          <table className="w-full text-sm">
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
