import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api/src/fetcher";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";
import { Modal } from "@/components/ui/Modal";

export const Route = createLazyFileRoute("/_auth/admin/transcode-cache")({
  component: TranscodeCachePage,
});

interface CacheFile {
  name: string;
  size: number;
  lastModified: string;
}

interface CacheInfo {
  totalSize: number;
  fileCount: number;
  files: CacheFile[];
}

const CACHE_QUERY_KEY = ["/api/v1/admin/transcode-cache"];

function formatSize(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  const val = bytes / Math.pow(1024, i);
  return `${val.toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

function formatDate(d: string): string {
  return new Date(d).toLocaleString();
}

function useCacheInfo() {
  return useQuery<CacheInfo>({
    queryKey: CACHE_QUERY_KEY,
    queryFn: () => eveyFetch<CacheInfo>({ url: "/api/v1/admin/transcode-cache", method: "GET" }),
  });
}

function TranscodeCachePage() {
  const { t } = useTranslation();
  const { data, isLoading } = useCacheInfo();
  const queryClient = useQueryClient();
  const [showClearConfirm, setShowClearConfirm] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [deletingFile, setDeletingFile] = useState<string | null>(null);

  async function handleClearAll() {
    setClearing(true);
    try {
      await eveyFetch<void>({ url: "/api/v1/admin/transcode-cache", method: "DELETE" });
      toast.success(t("admin.transcodeCache.cacheCleared"));
      queryClient.invalidateQueries({ queryKey: CACHE_QUERY_KEY });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("admin.transcodeCache.failedClear"));
    } finally {
      setClearing(false);
      setShowClearConfirm(false);
    }
  }

  async function handleDeleteFile(filename: string) {
    setDeletingFile(filename);
    try {
      await eveyFetch<void>({ url: `/api/v1/admin/transcode-cache/${encodeURIComponent(filename)}`, method: "DELETE" });
      toast.success(t("admin.transcodeCache.deleted", { name: filename }));
      queryClient.invalidateQueries({ queryKey: CACHE_QUERY_KEY });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t("admin.transcodeCache.failedDeleteFile"));
    } finally {
      setDeletingFile(null);
    }
  }

  const files = data?.files ?? [];

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          {t("admin.transcodeCache.title")}
        </h1>
        {files.length > 0 && (
          <Button variant="danger" onClick={() => setShowClearConfirm(true)}>
            {t("admin.transcodeCache.clearAll")}
          </Button>
        )}
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
            <Card>
              <dt className="text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>
                {t("admin.transcodeCache.totalSize")}
              </dt>
              <dd className="text-xl font-bold" style={{ color: "var(--text)" }}>
                {formatSize(data?.totalSize ?? 0)}
              </dd>
            </Card>
            <Card>
              <dt className="text-xs font-medium mb-1" style={{ color: "var(--text-muted)" }}>
                {t("admin.transcodeCache.files")}
              </dt>
              <dd className="text-xl font-bold" style={{ color: "var(--text)" }}>
                {data?.fileCount ?? 0}
              </dd>
            </Card>
          </div>

          {files.length === 0 ? (
            <div
              className="rounded-[var(--radius-lg)]"
              style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
            >
              <EmptyState icon="media" title={t("admin.transcodeCache.cacheEmpty")} />
            </div>
          ) : (
            <div
              className="rounded-[var(--radius-lg)] overflow-hidden"
              style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
            >
              <table className="w-full text-sm">
                <thead>
                  <tr style={{ borderBottom: "1px solid var(--border)" }}>
                    <th className="text-left px-4 py-3 font-medium" style={{ color: "var(--text-muted)" }}>
                      {t("admin.transcodeCache.tableHeaders.file")}
                    </th>
                    <th className="text-left px-4 py-3 font-medium" style={{ color: "var(--text-muted)" }}>
                      {t("admin.transcodeCache.tableHeaders.size")}
                    </th>
                    <th className="text-left px-4 py-3 font-medium hidden sm:table-cell" style={{ color: "var(--text-muted)" }}>
                      {t("admin.transcodeCache.tableHeaders.modified")}
                    </th>
                    <th className="px-4 py-3 w-20" />
                  </tr>
                </thead>
                <tbody>
                  {files.map((file) => (
                    <tr key={file.name} style={{ borderBottom: "1px solid var(--border)" }}>
                      <td className="px-4 py-3 font-mono text-xs truncate max-w-[200px] sm:max-w-[400px]" style={{ color: "var(--text)" }} title={file.name}>
                        {file.name}
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
                        {formatSize(file.size)}
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap hidden sm:table-cell" style={{ color: "var(--text-muted)" }}>
                        {formatDate(file.lastModified)}
                      </td>
                      <td className="px-4 py-3">
                        <Button
                          variant="danger"
                          loading={deletingFile === file.name}
                          onClick={() => handleDeleteFile(file.name)}
                          className="text-xs"
                        >
                          {t("common.delete")}
                        </Button>
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
        open={showClearConfirm}
        onClose={() => setShowClearConfirm(false)}
        title={t("admin.transcodeCache.clearConfirmTitle")}
      >
        <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
          {t("admin.transcodeCache.clearConfirm", { count: data?.fileCount ?? 0, size: formatSize(data?.totalSize ?? 0) })}
        </p>
        <div className="flex justify-end gap-3">
          <Button variant="secondary" onClick={() => setShowClearConfirm(false)}>
            {t("common.cancel")}
          </Button>
          <Button variant="danger" loading={clearing} onClick={handleClearAll}>
            {t("admin.transcodeCache.clearAll")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
