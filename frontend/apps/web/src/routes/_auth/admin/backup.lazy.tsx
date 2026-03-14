import { useRef, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuthStore, toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Modal } from "@/components/ui/Modal";

export const Route = createLazyFileRoute("/_auth/admin/backup")({
  component: BackupPage,
});

interface Backup {
  id: string;
  createdAt: string;
  version: string;
  size: number;
}

const BACKUPS_QUERY_KEY = ["/api/v1/admin/backups"];

function formatSize(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

function useBackups() {
  return useQuery<Backup[]>({
    queryKey: BACKUPS_QUERY_KEY,
    queryFn: async () => {
      const data = await eveyFetch<{ backups: Backup[] }>({ url: "/api/v1/admin/backups", method: "GET" });
      return data.backups ?? [];
    },
  });
}

function useCreateBackup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => eveyFetch<Backup>({ url: "/api/v1/admin/backups", method: "POST" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: BACKUPS_QUERY_KEY });
      toast.success("Backup created");
    },
    onError: (err: Error) => toast.error(err.message || "Failed to create backup"),
  });
}

function useDeleteBackup() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => eveyFetch<void>({ url: `/api/v1/admin/backups/${id}`, method: "DELETE" }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: BACKUPS_QUERY_KEY });
      toast.success("Backup deleted");
    },
    onError: (err: Error) => toast.error(err.message || "Failed to delete backup"),
  });
}

function BackupRow({ backup }: { backup: Backup }) {
  const token = useAuthStore((s) => s.accessToken);
  const deleteBackup = useDeleteBackup();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const [restoring, setRestoring] = useState(false);

  function handleDownload() {
    const a = document.createElement("a");
    a.href = `/api/v1/admin/backups/${backup.id}`;
    if (token) {
      // Append token as query param for direct download
      a.href += `?token=${encodeURIComponent(token)}`;
    }
    a.download = `${backup.id}.gz`;
    a.click();
  }

  async function handleRestore() {
    setRestoring(true);
    try {
      // Raw fetch required: blob download then multipart upload (eveyFetch is JSON-only)
      const fileRes = await fetch(`/api/v1/admin/backups/${backup.id}`, {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (fileRes.status === 401) window.dispatchEvent(new Event("evey:unauthorized"));
      if (!fileRes.ok) throw new Error("Failed to fetch backup file");
      const blob = await fileRes.blob();
      const formData = new FormData();
      formData.append("backup", blob, `${backup.id}.gz`);
      const res = await fetch("/api/v1/admin/backups/restore", {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: formData,
      });
      if (res.status === 401) window.dispatchEvent(new Event("evey:unauthorized"));
      if (!res.ok) throw new Error("Restore failed");
      toast.success("Restore completed");
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Restore failed");
    } finally {
      setRestoring(false);
    }
  }

  return (
    <tr style={{ borderBottom: "1px solid var(--border)" }}>
      <td className="px-4 py-3 font-mono text-xs" style={{ color: "var(--text)" }}>
        {backup.id}
      </td>
      <td className="px-4 py-3 text-sm whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
        {new Date(backup.createdAt).toLocaleString()}
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text-muted)" }}>
        {backup.version}
      </td>
      <td className="px-4 py-3 text-sm whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
        {formatSize(backup.size)}
      </td>
      <td className="px-4 py-3">
        <div className="flex gap-2">
          <Button variant="secondary" onClick={handleDownload} className="text-xs">
            Download
          </Button>
          <Button
            variant="secondary"
            loading={restoring}
            onClick={handleRestore}
            className="text-xs"
          >
            Restore
          </Button>
          <Button
            variant="danger"
            loading={deleteBackup.isPending}
            onClick={() => setConfirmDelete(true)}
            className="text-xs"
          >
            Delete
          </Button>
        </div>
        <Modal
          open={confirmDelete}
          onClose={() => setConfirmDelete(false)}
          title="Delete Backup"
        >
          <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
            Delete backup from {new Date(backup.createdAt).toLocaleString()}? This cannot be undone.
          </p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setConfirmDelete(false)}>
              Cancel
            </Button>
            <Button
              variant="danger"
              loading={deleteBackup.isPending}
              onClick={() => {
                deleteBackup.mutate(backup.id);
                setConfirmDelete(false);
              }}
            >
              Delete
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
  const token = useAuthStore((s) => s.accessToken);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [restoring, setRestoring] = useState(false);

  async function handleUpload() {
    const file = fileInputRef.current?.files?.[0];
    if (!file) {
      toast.error("Select a backup file first");
      return;
    }
    setRestoring(true);
    try {
      const formData = new FormData();
      formData.append("backup", file);
      // Raw fetch required: multipart upload (eveyFetch is JSON-only)
      const res = await fetch("/api/v1/admin/backups/restore", {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: formData,
      });
      if (res.status === 401) window.dispatchEvent(new Event("evey:unauthorized"));
      if (!res.ok) throw new Error("Restore failed");
      toast.success("Restore completed");
      onClose();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Restore failed");
    } finally {
      setRestoring(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Restore from File">
      <div className="flex flex-col gap-4">
        <p className="text-sm" style={{ color: "var(--text-muted)" }}>
          Upload a backup .gz file to restore. This will overwrite existing data.
        </p>
        <input
          ref={fileInputRef}
          type="file"
          accept=".gz"
          className="text-sm"
          style={{ color: "var(--text)" }}
        />
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button loading={restoring} onClick={handleUpload}>
            Restore
          </Button>
        </div>
      </div>
    </Modal>
  );
}

function BackupPage() {
  const { data: backups, isLoading } = useBackups();
  const createBackup = useCreateBackup();
  const [showRestoreUpload, setShowRestoreUpload] = useState(false);

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          Backup & Restore
        </h1>
        <div className="flex gap-2">
          <Button
            variant="secondary"
            onClick={() => setShowRestoreUpload(true)}
          >
            Restore from File
          </Button>
          <Button
            loading={createBackup.isPending}
            onClick={() => createBackup.mutate()}
          >
            Create Backup
          </Button>
        </div>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : !backups || backups.length === 0 ? (
        <EmptyState
          icon="library"
          title="No backups"
          description="Create a backup to save your database state."
        />
      ) : (
        <div
          className="overflow-x-auto rounded-[var(--radius-lg)]"
          style={{ border: "1px solid var(--border)" }}
        >
          <table className="w-full text-sm">
            <thead>
              <tr
                style={{
                  backgroundColor: "var(--surface)",
                  borderBottom: "1px solid var(--border)",
                }}
              >
                {["ID", "Date", "Version", "Size", "Actions"].map((h) => (
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
