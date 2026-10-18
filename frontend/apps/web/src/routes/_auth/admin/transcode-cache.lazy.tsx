import { useState, useEffect, useCallback } from "react";
import { Film } from "lucide-react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { formatBytes, eveyFetch } from "@evey/api";
import {
  useTranscodeCacheServiceGetTranscodeCache,
  clearTranscodeCacheUnbounded,
  transcodeCacheServiceDeleteTranscodeCacheFile,
  getTranscodeCacheServiceGetTranscodeCacheQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { PageHeader } from "@/components/ui/PageHeader";
import { Modal } from "@/components/ui/Modal";
import { DemoReadOnly } from "@/components/layout/DemoReadOnly";
import { SETTINGS_KEY, useTranscodingServerSettings } from "@/features/admin/transcoding/hooks";

export const Route = createLazyFileRoute("/_auth/admin/transcode-cache")({
  // Every control here is an operator write the demo refuses (DemoReadOnly).
  component: () => (
    <DemoReadOnly>
      <TranscodeCachePage />
    </DemoReadOnly>
  ),
});

const CACHE_QUERY_KEY = getTranscodeCacheServiceGetTranscodeCacheQueryKey();

function formatDate(d: string | undefined): string {
  return d ? new Date(d).toLocaleString() : "";
}

// The cap goes through /admin/server-settings, which applies it to the running
// transcoder; the raw server-config key was only read back at the next start.
function TranscodingSettings() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: settings, isLoading } = useTranscodingServerSettings();

  const [maxConcurrent, setMaxConcurrent] = useState("2");
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (settings) setMaxConcurrent(String(settings.maxConcurrentTranscodes.value));
  }, [settings]);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      // An empty field is an explicit null: the server drops the override.
      await eveyFetch({
        url: "/api/v1/admin/server-settings",
        method: "PUT",
        data: { maxConcurrentTranscodes: maxConcurrent === "" ? null : parseInt(maxConcurrent, 10) },
      });
      await qc.invalidateQueries({ queryKey: SETTINGS_KEY });
      toast.success(t("admin.transcodeCache.settingsSaved") || "Transcoding settings saved");
    } catch {
      toast.error(t("admin.transcodeCache.settingsSaveFailed"));
    }
    setSaving(false);
  }, [maxConcurrent, qc, t]);

  if (isLoading) return <div className="flex justify-center py-8"><Spinner size="md" /></div>;

  return (
    <Card>
      <div className="p-5 flex flex-col gap-5">
        <h3 className="text-sm font-semibold text-[var(--text)]">
          {t("admin.transcodeCache.transcodingSettings")}
        </h3>

        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.transcodeCache.hwAccelNote")}{" "}
          <Link
            to="/admin/transcoding"
            className="underline text-[var(--accent)] hover:opacity-80"
          >
            {t("admin.transcodeCache.hwAccelNoteLink")}
          </Link>
        </p>

        <Input
          label={t("admin.transcodeCache.maxConcurrent")}
          type="number"
          min={1}
          max={16}
          value={maxConcurrent}
          onChange={(e) => setMaxConcurrent(e.target.value)}
        />

        <div className="pt-2">
          <Button onClick={handleSave} loading={saving}>
            {t("common.save")}
          </Button>
        </div>
      </div>
    </Card>
  );
}

function TranscodeCachePage() {
  const { t } = useTranslation();
  const { data, isLoading, isError, error, refetch } = useTranscodeCacheServiceGetTranscodeCache();
  const queryClient = useQueryClient();
  const [showClearConfirm, setShowClearConfirm] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [deletingFile, setDeletingFile] = useState<string | null>(null);

  async function handleClearAll() {
    setClearing(true);
    try {
      await clearTranscodeCacheUnbounded();
      toast.success(t("admin.transcodeCache.cacheCleared"));
      queryClient.invalidateQueries({ queryKey: CACHE_QUERY_KEY });
    } catch (err) {
      toast.error(apiErrorMessage(err as Error, t, t("admin.transcodeCache.failedClear")));
    } finally {
      setClearing(false);
      setShowClearConfirm(false);
    }
  }

  async function handleDeleteFile(filename: string) {
    setDeletingFile(filename);
    try {
      await transcodeCacheServiceDeleteTranscodeCacheFile(filename);
      toast.success(t("admin.transcodeCache.deleted", { name: filename }));
      queryClient.invalidateQueries({ queryKey: CACHE_QUERY_KEY });
    } catch (err) {
      toast.error(apiErrorMessage(err as Error, t, t("admin.transcodeCache.failedDeleteFile")));
    } finally {
      setDeletingFile(null);
    }
  }

  const files = data?.files ?? [];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.transcodeCache.title")} actions={files.length > 0 ? (
        <Button variant="danger" onClick={() => setShowClearConfirm(true)}>
          {t("admin.transcodeCache.clearAll")}
        </Button>
      ) : undefined} />

      <TranscodingSettings />

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : (
        <>
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4">
            <Card>
              <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                {t("admin.transcodeCache.totalSize")}
              </dt>
              <dd className="text-xl font-bold text-[var(--text)]">
                {formatBytes(Number(data?.totalSize ?? 0))}
              </dd>
            </Card>
            <Card>
              <dt className="text-xs font-medium mb-1 text-[var(--text-muted)]">
                {t("admin.transcodeCache.files")}
              </dt>
              <dd className="text-xl font-bold text-[var(--text)]">
                {data?.fileCount ?? 0}
              </dd>
            </Card>
          </div>

          {isError ? (
            // "Cache empty" would have an admin believe it was purged.
            <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
              <FetchErrorState error={error} onRetry={() => void refetch()} />
            </div>
          ) : files.length === 0 ? (
            <div
              className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
            >
              <EmptyState icon={<Film size={48} strokeWidth={1.5} />} title={t("admin.transcodeCache.cacheEmpty")} />
            </div>
          ) : (
            <div
              className="rounded-[var(--radius-lg)] overflow-hidden bg-[var(--surface)] border border-[var(--border)]"
            >
              <table className="w-full text-sm table-cards">
                <thead>
                  <tr className="border-b border-[var(--border)]">
                    <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
                      {t("admin.transcodeCache.tableHeaders.file")}
                    </th>
                    <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
                      {t("admin.transcodeCache.tableHeaders.size")}
                    </th>
                    <th className="text-left px-4 py-3 font-medium hidden sm:table-cell text-[var(--text-muted)]">
                      {t("admin.transcodeCache.tableHeaders.modified")}
                    </th>
                    <th className="px-4 py-3 w-20" />
                  </tr>
                </thead>
                <tbody>
                  {files.map((file) => (
                    <tr key={file.name} className="border-b border-[var(--border)]">
                      <td className="px-4 py-3 font-mono text-xs truncate max-w-[200px] sm:max-w-[400px] text-[var(--text)]" title={file.name} data-label={t("admin.transcodeCache.tableHeaders.file")}>
                        {file.name}
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap text-[var(--text-muted)]" data-label={t("admin.transcodeCache.tableHeaders.size")}>
                        {formatBytes(Number(file.size ?? 0))}
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap hidden sm:table-cell text-[var(--text-muted)]" data-label={t("admin.transcodeCache.tableHeaders.modified")}>
                        {formatDate(file.lastModified)}
                      </td>
                      <td className="px-4 py-3" data-label="">
                        <Button
                          variant="danger"
                          loading={deletingFile === file.name}
                          onClick={() => file.name && handleDeleteFile(file.name)}
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
        <p className="text-sm mb-4 text-[var(--text-muted)]">
          {t("admin.transcodeCache.clearConfirm", { count: data?.fileCount ?? 0, size: formatBytes(Number(data?.totalSize ?? 0)) })}
        </p>
        {/* The consequence the admin cannot see from this page: a wipe deletes
            the segments a live encode is still writing, so anyone being
            transcoded rebuffers. The server has always logged it - which is no
            use to someone standing in front of the button. */}
        {(data?.activeEncodingSessions ?? 0) > 0 && (
          <p className="text-sm mb-4" style={{ color: "var(--warning)" }} data-testid="clear-cache-live-warning">
            {t("admin.transcodeCache.clearConfirmLiveSessions", { count: data?.activeEncodingSessions ?? 0 })}
          </p>
        )}
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
