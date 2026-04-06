import { useState, useEffect, useCallback } from "react";
import { Film } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { useAdminSystemServiceGetSystemInfo } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Select } from "@/components/ui/Select";
import { Checkbox } from "@/components/ui/Checkbox";
import { Input } from "@/components/ui/Input";
import { EmptyState } from "@/components/ui/EmptyState";
import { PageHeader } from "@/components/ui/PageHeader";
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

const HW_ACCEL_OPTIONS = [
  { value: "none", label: "None (Software)" },
  { value: "vaapi", label: "VAAPI (Intel/AMD)" },
  { value: "nvenc", label: "NVENC (NVIDIA)" },
  { value: "qsv", label: "QSV (Intel Quick Sync)" },
];

const HW_DECODE_CODECS = ["h264", "hevc", "vp9", "av1"];

const CONFIG_QUERY_KEY = ["/api/v1/admin/server-config"];

interface ConfigEntry { key: string; value: string }

function useServerConfig() {
  return useQuery<Record<string, string>>({
    queryKey: CONFIG_QUERY_KEY,
    queryFn: async () => {
      const data = await eveyFetch<{ entries: ConfigEntry[] }>({ url: "/api/v1/admin/server-config", method: "GET" });
      const map: Record<string, string> = {};
      for (const e of data.entries ?? []) map[e.key] = e.value;
      return map;
    },
  });
}

async function setConfigKey(key: string, value: string) {
  await eveyFetch<void>({ url: `/api/v1/admin/server-config/${key}`, method: "PUT", data: { value } });
}

function TranscodingSettings() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: config, isLoading } = useServerConfig();
  const { data: sysInfo } = useAdminSystemServiceGetSystemInfo();

  const [hwAccel, setHwAccel] = useState("none");
  const [hwCodecs, setHwCodecs] = useState<Set<string>>(new Set());
  const [maxConcurrent, setMaxConcurrent] = useState("2");
  const [throttle, setThrottle] = useState(true);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!config) return;
    setHwAccel(config["transcoding.hw_accel_type"] ?? "none");
    setHwCodecs(new Set((config["transcoding.hw_decode_codecs"] ?? "h264,hevc").split(",").filter(Boolean)));
    setMaxConcurrent(config["transcoding.max_concurrent"] ?? "2");
    setThrottle(config["transcoding.throttle"] !== "false");
  }, [config]);

  const toggleCodec = useCallback((codec: string) => {
    setHwCodecs((prev) => {
      const next = new Set(prev);
      if (next.has(codec)) next.delete(codec); else next.add(codec);
      return next;
    });
  }, []);

  const handleSave = useCallback(async () => {
    setSaving(true);
    try {
      await setConfigKey("transcoding.hw_accel_type", hwAccel);
      await setConfigKey("transcoding.hw_decode_codecs", Array.from(hwCodecs).join(","));
      await setConfigKey("transcoding.max_concurrent", maxConcurrent);
      await setConfigKey("transcoding.throttle", throttle ? "true" : "false");
      await qc.invalidateQueries({ queryKey: CONFIG_QUERY_KEY });
      toast.success(t("admin.transcodeCache.settingsSaved") || "Transcoding settings saved");
    } catch {
      toast.error("Failed to save settings");
    }
    setSaving(false);
  }, [hwAccel, hwCodecs, maxConcurrent, throttle, qc, t]);

  const hwDevices = sysInfo?.hwDevices ?? [];

  if (isLoading) return <div className="flex justify-center py-8"><Spinner size="md" /></div>;

  return (
    <Card>
      <div className="p-5 flex flex-col gap-5">
        <h3 className="text-sm font-semibold text-[var(--text)]">
          {t("admin.transcodeCache.transcodingSettings") || "Transcoding Settings"}
        </h3>

        <Select
          label={t("admin.transcodeCache.hwAccelType") || "Hardware acceleration"}
          options={HW_ACCEL_OPTIONS}
          value={hwAccel}
          onChange={setHwAccel}
        />

        {hwDevices.length > 0 && (
          <div className="flex flex-wrap gap-2">
            {hwDevices.map((d, i) => (
              <span key={i} className="text-xs px-2 py-1 rounded bg-[var(--surface-raised)] text-[var(--accent)]">
                {d.type?.replace("HW_ACCEL_TYPE_", "")} - {d.name ?? "unknown"}
              </span>
            ))}
          </div>
        )}

        {hwAccel !== "none" && (
          <div>
            <p className="text-xs font-medium uppercase tracking-wide mb-2 text-[var(--text-muted)]">
              {t("admin.transcodeCache.hwDecodeCodecs") || "Hardware decode codecs"}
            </p>
            <div className="flex flex-wrap gap-3">
              {HW_DECODE_CODECS.map((codec) => (
                <Checkbox
                  key={codec}
                  label={codec.toUpperCase()}
                  checked={hwCodecs.has(codec)}
                  onChange={() => toggleCodec(codec)}
                />
              ))}
            </div>
          </div>
        )}

        <Input
          label={t("admin.transcodeCache.maxConcurrent") || "Max concurrent transcodes"}
          type="number"
          min={1}
          max={16}
          value={maxConcurrent}
          onChange={(e) => setMaxConcurrent(e.target.value)}
        />

        <Checkbox
          label={t("admin.transcodeCache.throttle") || "Throttle transcodes when client buffer is full"}
          checked={throttle}
          onChange={setThrottle}
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
                {formatSize(data?.totalSize ?? 0)}
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

          {files.length === 0 ? (
            <div
              className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
            >
              <EmptyState icon={<Film size={48} strokeWidth={1.5} />} title={t("admin.transcodeCache.cacheEmpty")} />
            </div>
          ) : (
            <div
              className="rounded-[var(--radius-lg)] overflow-hidden bg-[var(--surface)] border border-[var(--border)]"
            >
              <table className="w-full text-sm">
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
                      <td className="px-4 py-3 font-mono text-xs truncate max-w-[200px] sm:max-w-[400px] text-[var(--text)]" title={file.name}>
                        {file.name}
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap text-[var(--text-muted)]">
                        {formatSize(file.size)}
                      </td>
                      <td className="px-4 py-3 whitespace-nowrap hidden sm:table-cell text-[var(--text-muted)]">
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
        <p className="text-sm mb-4 text-[var(--text-muted)]">
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
