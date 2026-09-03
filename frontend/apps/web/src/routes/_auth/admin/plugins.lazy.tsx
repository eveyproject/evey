import { useState, useRef, useEffect } from "react";
import { TriangleAlert, Upload } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminPluginServiceListPlugins,
  useAdminPluginServiceEnablePlugin,
  useAdminPluginServiceDisablePlugin,
  useAdminPluginServiceUninstallPlugin,
  useAdminPluginServiceInstallPlugin,
  adminPluginServiceInstallPlugin,
  NO_TIMEOUT,
  eveyFetch,
  eveyFetchRaw,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { Badge } from "@/components/ui/Badge";
import { PluginCard } from "@/features/admin/PluginCard";
import { PluginSecurityDashboard } from "@/features/admin/PluginSecurityDashboard";
import { PluginResourceLimits } from "@/features/admin/PluginResourceLimits";

export const Route = createLazyFileRoute("/_auth/admin/plugins")({
  component: PluginsPage,
});

const PLUGINS_QUERY_KEY = ["/api/v1/admin/plugins"];

interface DetectedRuntime {
  language: string;
  command: string;
  version: string;
}

function useAvailableRuntimes() {
  return useQuery({
    queryKey: ["/api/v1/admin/plugin-runtimes/available"],
    queryFn: () =>
      eveyFetch<{ runtimes: DetectedRuntime[] }>({
        url: "/api/v1/admin/plugin-runtimes/available",
        method: "GET",
      }),
    staleTime: 60_000,
  });
}

interface PluginPreview {
  name: string;
  version: string;
  description: string;
  author: string;
  language: string;
  permissions: string[];
  capabilities: string[];
  analysis?: {
    safe: boolean;
    issues: string[];
    urls?: string[];
    total_files_scanned: number;
  };
}

type InstallStep = "idle" | "previewing" | "reviewing" | "installing" | "done";

function SubprocessWarning() {
  const { t } = useTranslation();
  return (
    <div
      className="flex items-start gap-3 p-4 rounded-[var(--radius-md)] bg-[var(--badge-warning-bg,#713f12)] border border-[var(--badge-warning-text,#fde047)]"
    >
      <TriangleAlert
        size={ICON.lg}
        className="shrink-0 mt-0.5 text-[var(--badge-warning-text,#fde047)]"
      />
      <div>
        <p
          className="font-semibold text-sm text-[var(--badge-warning-text,#fde047)]"
        >
          {t("admin.plugins.subprocessRequired")}
        </p>
        <p className="text-xs mt-1 text-[var(--text-muted)]">
          {t("admin.plugins.subprocessWarning")}
        </p>
      </div>
    </div>
  );
}

function PreviewPanel({
  preview,
  hasWarnings,
  installing,
  onCancel,
  onConfirm,
}: {
  preview: PluginPreview;
  hasWarnings: boolean;
  installing: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <span className="font-semibold text-[var(--text)]">
          {preview.name}
        </span>
        <span className="text-xs text-[var(--text-muted)]">
          v{preview.version} - {preview.language}
          {preview.author ? ` - by ${preview.author}` : ""}
        </span>
        {preview.description && (
          <p className="text-sm mt-1 text-[var(--text-muted)]">
            {preview.description}
          </p>
        )}
      </div>

      {preview.permissions?.length > 0 && (
        <div className="flex flex-col gap-2">
          <span className="text-xs font-semibold text-[var(--text-muted)]">
            {t("admin.plugins.permissions")}
          </span>
          <div className="flex flex-wrap gap-1.5">
            {preview.permissions.map((p) => (
              <Badge
                key={p}
                variant={p === "subprocess" ? "warning" : "default"}
              >
                {p}
              </Badge>
            ))}
          </div>
        </div>
      )}

      {preview.capabilities?.length > 0 && (
        <div className="flex flex-col gap-2">
          <span className="text-xs font-semibold text-[var(--text-muted)]">
            {t("admin.plugins.capabilities")}
          </span>
          <div className="flex flex-wrap gap-1.5">
            {preview.capabilities.map((c) => (
              <Badge key={c} variant="info">{c}</Badge>
            ))}
          </div>
        </div>
      )}

      {preview.permissions?.includes("subprocess") && <SubprocessWarning />}

      {preview.analysis?.issues && preview.analysis.issues.length > 0 && (
        <div className="flex flex-col gap-1.5 text-xs">
          <span className="font-semibold text-[var(--text-muted)]">{t("admin.plugins.staticAnalysis")}</span>
          {preview.analysis.issues.map((issue, i) => {
            const isCritical = issue.startsWith("[CRITICAL]");
            const isWarning = issue.startsWith("[WARNING]");
            return (
              <span
                key={i}
                className={
                  isCritical
                    ? "text-[var(--badge-error-text,#fca5a5)]"
                    : isWarning
                      ? "text-[var(--badge-warning-text,#fde047)]"
                      : "text-[var(--text-muted)]"
                }
              >
                {issue}
              </span>
            );
          })}
        </div>
      )}

      {preview.analysis?.urls && preview.analysis.urls.length > 0 && (
        <div className="flex flex-col gap-1.5 text-xs">
          <span className="font-semibold text-[var(--text-muted)]">
            {t("admin.plugin.analysisReport.urls", "URLs externes détectées")}
          </span>
          {preview.analysis.urls.map((url, i) => (
            <span key={i} className="font-mono break-all text-[var(--text-muted)]">
              {url}
            </span>
          ))}
        </div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <Button variant="ghost" onClick={onCancel}>{t("common.cancel")}</Button>
        <Button
          variant={hasWarnings ? "danger" : "primary"}
          loading={installing}
          onClick={onConfirm}
        >
          {hasWarnings ? t("admin.plugins.installAnyway") : t("admin.plugins.install")}
        </Button>
      </div>
    </div>
  );
}

// parseEscalationEntries pulls "permission:network, capability:foo, ..." out
// of the gRPC error message produced by checkPermissionEscalation server-side.
function parseEscalationEntries(msg: string): string[] {
  const marker = "new entries requested: ";
  const idx = msg.indexOf(marker);
  if (idx < 0) return [];
  return msg
    .slice(idx + marker.length)
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

function InstallPluginForm({ onSuccess }: { onSuccess: () => void }) {
  const { t } = useTranslation();
  const [mode, setMode] = useState<"file" | "url">("file");
  const [step, setStep] = useState<InstallStep>("idle");
  const [preview, setPreview] = useState<PluginPreview | null>(null);
  const [error, setError] = useState("");
  const [blockedIssues, setBlockedIssues] = useState<string[]>([]);
  const [escalationEntries, setEscalationEntries] = useState<string[]>([]);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [selectedFileName, setSelectedFileName] = useState("");
  const [selectedSigFile, setSelectedSigFile] = useState<File | null>(null);
  const [selectedSigFileName, setSelectedSigFileName] = useState("");
  const fileRef = useRef<HTMLInputElement>(null);
  const sigFileRef = useRef<HTMLInputElement>(null);
  const [source, setSource] = useState("");
  const [name, setName] = useState("");
  const installPlugin = useAdminPluginServiceInstallPlugin({
    mutation: {
      // The server downloads and verifies the archive inside the request:
      // opt out of the fetcher's default timeout.
      mutationFn: ({ data }) => adminPluginServiceInstallPlugin(data, NO_TIMEOUT),
      onSuccess: () => {
        setEscalationEntries([]);
        onSuccess();
      },
      onError: (err: unknown) => {
        // gRPC FailedPrecondition with "new entries requested: ..." means the
        // update widens permissions and the admin must explicitly acknowledge.
        const msg = err instanceof Error ? err.message : String(err);
        if (msg.includes("new entries requested:")) {
          setEscalationEntries(parseEscalationEntries(msg));
          setError("");
        } else {
          setError(msg || t("admin.plugins.installFailed"));
        }
      },
    },
  });

  function getFile(): File | null {
    return fileRef.current?.files?.[0] ?? null;
  }

  async function handleFilePreview() {
    const file = getFile();
    if (!file) { toast.error(t("admin.plugins.selectFileFirst")); return; }
    setSelectedFile(file);
    setStep("previewing");
    setError("");
    try {
      const formData = new FormData();
      formData.append("plugin", file);
      const res = await eveyFetchRaw({ url: "/api/v1/admin/plugins/preview", method: "POST", body: formData });
      const data = (await res.json()) as PluginPreview;
      setPreview(data);
      setStep("reviewing");
    } catch (err) {
      setError(apiErrorMessage(err as Error, t, t("admin.plugins.previewFailed")));
      setStep("idle");
    }
  }

  async function handleFileInstall(acceptPermissionChanges = false) {
    const file = selectedFile;
    if (!file) { toast.error(t("admin.plugins.fileMissing")); return; }
    setStep("installing");
    setError("");
    if (!acceptPermissionChanges) setEscalationEntries([]);
    try {
      const formData = new FormData();
      formData.append("plugin", file);
      if (selectedSigFile) formData.append("sig", selectedSigFile);
      if (acceptPermissionChanges) formData.append("accept_permission_changes", "true");
      // raw: true so a 428 (permission escalation) or 403 (blocked) comes back
      // as a Response to branch on, instead of the fetcher throwing the raw
      // (untranslated) backend message before we can show the proper UI.
      const res = await eveyFetchRaw({ url: "/api/v1/admin/plugins/upload", method: "POST", body: formData, raw: true });
      // 428 PreconditionRequired = update widens permissions; show the diff +
      // an Approve button, then re-POST with accept_permission_changes.
      if (res.status === 428) {
        const body = (await res.json().catch(() => ({}))) as { permission_changes?: boolean; new_entries?: string[] };
        if (body.permission_changes) {
          setEscalationEntries(body.new_entries ?? []);
          setStep("reviewing");
          return;
        }
      }
      // 403 Forbidden = blocked by the static security analysis.
      if (res.status === 403) {
        const body = (await res.json().catch(() => ({}))) as { blocked?: boolean; issues?: string[] };
        if (body.blocked && body.issues) {
          setBlockedIssues(body.issues);
          setError(t("admin.plugins.blockedByAnalysis"));
          setStep("reviewing");
          return;
        }
      }
      if (!res.ok) throw new Error(t("admin.plugins.installFailed"));
      setEscalationEntries([]);
      setStep("done");
      onSuccess();
    } catch (err) {
      setError(apiErrorMessage(err as Error, t, t("admin.plugins.installFailed")));
      setStep("reviewing");
    }
  }

  function resetFileFlow() {
    setStep("idle");
    setPreview(null);
    setError("");
    setBlockedIssues([]);
    setEscalationEntries([]);
    setSelectedFile(null);
    setSelectedFileName("");
    setSelectedSigFile(null);
    setSelectedSigFileName("");
  }

  // The URL path used to install blind: the static analysis ran server-side
  // but its WARNINGs (overreach, extracted URLs) were only visible once the
  // plugin was already running. Preview the source first, like a file.
  async function handleUrlPreview(e: React.FormEvent) {
    e.preventDefault();
    const src = source.trim();
    if (!src) return;
    setStep("previewing");
    setError("");
    try {
      const data = await eveyFetch<PluginPreview>({
        url: "/api/v1/admin/plugins/preview",
        method: "POST",
        data: { source: src },
      });
      setPreview(data);
      setStep("reviewing");
    } catch (err) {
      setError(apiErrorMessage(err as Error, t, t("admin.plugins.previewFailed")));
      setStep("idle");
    }
  }

  function handleUrlInstall() {
    setStep("installing");
    setError("");
    installPlugin.mutate(
      { data: { source: source.trim(), name } },
      { onSettled: () => setStep("reviewing") },
    );
  }

  function handleConfirmEscalation() {
    if (mode === "file") {
      void handleFileInstall(true);
    } else {
      installPlugin.mutate({ data: { source, name, acceptPermissionChanges: true } });
    }
  }

  const hasWarnings = !!preview?.permissions?.includes("subprocess") ||
    (preview?.analysis?.issues?.length ?? 0) > 0;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2">
        <Button
          variant={mode === "file" ? "primary" : "ghost"}
          onClick={() => { setMode("file"); resetFileFlow(); }}
        >
          {t("admin.plugins.uploadFile")}
        </Button>
        <Button
          variant={mode === "url" ? "primary" : "ghost"}
          onClick={() => { setMode("url"); resetFileFlow(); }}
        >
          {t("admin.plugins.fromUrl")}
        </Button>
      </div>

      {escalationEntries.length > 0 && (
        <div
          className="flex flex-col gap-3 p-4 rounded-[var(--radius-md)]"
          style={{
            backgroundColor: "var(--badge-warning-bg, #78350f)",
            border: "1px solid var(--badge-warning-text, #fde047)",
          }}
        >
          <div className="flex items-start gap-2">
            <TriangleAlert size={ICON.lg} className="shrink-0 mt-0.5" style={{ color: "var(--badge-warning-text, #fde047)" }} />
            <div className="flex flex-col gap-1">
              <p className="text-sm font-semibold" style={{ color: "var(--badge-warning-text, #fde047)" }}>
                {t("admin.plugins.permissionEscalation.title")}
              </p>
              <p className="text-xs" style={{ color: "var(--text-muted)" }}>
                {t("admin.plugins.permissionEscalation.description")}
              </p>
            </div>
          </div>
          <ul className="flex flex-col gap-1 text-xs list-disc pl-6 font-mono">
            {escalationEntries.map((e, i) => (
              <li key={i} style={{ color: "var(--badge-warning-text, #fde047)" }}>
                {e}
              </li>
            ))}
          </ul>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={resetFileFlow}>
              {t("common.cancel")}
            </Button>
            <Button
              onClick={handleConfirmEscalation}
              loading={installPlugin.isPending || step === "installing"}
            >
              {t("admin.plugins.permissionEscalation.confirm")}
            </Button>
          </div>
        </div>
      )}

      {mode === "file" && step !== "reviewing" && step !== "installing" && (
        <div className="flex flex-col gap-4">
          <div>
            <label
              className="block text-sm font-medium mb-1 text-[var(--text-muted)]"
            >
              {t("admin.plugins.pluginFileZip")}
            </label>
            <input
              ref={fileRef}
              type="file"
              accept=".zip"
              className="hidden"
              onChange={() => {
                const name = fileRef.current?.files?.[0]?.name;
                if (name) setSelectedFileName(name);
              }}
            />
            <button
              type="button"
              onClick={() => fileRef.current?.click()}
              className="flex items-center gap-2 w-full px-4 py-3 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast cursor-pointer"
              style={{
                backgroundColor: "var(--surface-raised, var(--surface))",
                color: selectedFileName ? "var(--text)" : "var(--text-muted)",
                border: "1px dashed var(--border)",
              }}
            >
              <Upload size={ICON.md} className="shrink-0" />
              {selectedFileName || t("admin.plugins.chooseFile")}
            </button>
          </div>
          <div>
            <label
              className="block text-sm font-medium mb-1 text-[var(--text-muted)]"
            >
              {t("admin.plugins.sigFileOptional")}
            </label>
            <input
              ref={sigFileRef}
              type="file"
              accept=".sig"
              className="hidden"
              onChange={() => {
                const f = sigFileRef.current?.files?.[0];
                if (f) { setSelectedSigFile(f); setSelectedSigFileName(f.name); }
              }}
            />
            <button
              type="button"
              onClick={() => sigFileRef.current?.click()}
              className="flex items-center gap-2 w-full px-4 py-3 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast cursor-pointer"
              style={{
                backgroundColor: "var(--surface-raised, var(--surface))",
                color: selectedSigFileName ? "var(--text)" : "var(--text-muted)",
                border: "1px dashed var(--border)",
              }}
            >
              <Upload size={ICON.md} className="shrink-0" />
              {selectedSigFileName || t("admin.plugins.chooseFile")}
            </button>
          </div>
          {error && (
            <p className="text-xs text-[var(--badge-error-text,#fca5a5)]">
              {error}
            </p>
          )}
          <div className="flex justify-end pt-2">
            <Button
              onClick={handleFilePreview}
              loading={step === "previewing"}
            >
              {t("admin.plugins.previewInstall")}
            </Button>
          </div>
        </div>
      )}

      {mode === "file" && (step === "reviewing" || step === "installing") && preview && (
        <>
          {error && (
            <div className="flex flex-col gap-2 p-3 rounded-[var(--radius-md)] bg-[var(--badge-error-bg,#7f1d1d)] border border-[var(--badge-error-text,#fca5a5)]">
              <p className="text-sm font-semibold text-[var(--badge-error-text,#fca5a5)]">
                {error}
              </p>
              {blockedIssues.length > 0 && (
                <ul className="flex flex-col gap-1 text-xs list-disc pl-4">
                  {blockedIssues.map((issue, i) => (
                    <li
                      key={i}
                      className={
                        issue.startsWith("[CRITICAL]")
                          ? "text-[var(--badge-error-text,#fca5a5)]"
                          : issue.startsWith("[WARNING]")
                            ? "text-[var(--badge-warning-text,#fde047)]"
                            : "text-[var(--text-muted)]"
                      }
                    >
                      {issue}
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )}
          <PreviewPanel
            preview={preview}
            hasWarnings={hasWarnings}
            installing={step === "installing"}
            onCancel={resetFileFlow}
            onConfirm={() => handleFileInstall(false)}
          />
        </>
      )}

      {mode === "url" && (step === "reviewing" || step === "installing") && preview && (
        <>
          {error && (
            <p className="text-sm font-semibold p-3 rounded-[var(--radius-md)] bg-[var(--badge-error-bg,#7f1d1d)] text-[var(--badge-error-text,#fca5a5)]">
              {error}
            </p>
          )}
          <PreviewPanel
            preview={preview}
            hasWarnings={hasWarnings}
            installing={step === "installing" || installPlugin.isPending}
            onCancel={resetFileFlow}
            onConfirm={handleUrlInstall}
          />
        </>
      )}

      {mode === "url" && step !== "reviewing" && step !== "installing" && (
        <form onSubmit={handleUrlPreview} className="flex flex-col gap-4">
          <Input
            label={t("admin.plugins.sourceUrl")}
            value={source}
            onChange={(e) => setSource(e.target.value)}
            placeholder="https://example.com/plugin.zip"
            required
          />
          <Input
            label={t("admin.plugins.nameOptional")}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-plugin"
          />
          {error && (
            <p className="text-xs text-[var(--badge-error-text,#fca5a5)]">
              {error}
            </p>
          )}
          <div className="flex justify-end pt-2">
            <Button type="submit" loading={step === "previewing"} disabled={!source.trim()}>
              {t("admin.plugins.previewInstall")}
            </Button>
          </div>
        </form>
      )}
    </div>
  );
}

function PluginsPage() {
  const { t } = useTranslation();
  const [showModal, setShowModal] = useState(false);
  const [uninstallTarget, setUninstallTarget] = useState<{ id: string; name: string } | null>(null);
  const queryClient = useQueryClient();
  const { data, isLoading } = useAdminPluginServiceListPlugins();
  const plugins = data?.plugins ?? [];
  const { data: runtimesData } = useAvailableRuntimes();
  const runtimes = runtimesData?.runtimes ?? [];

  const invalidate = () => queryClient.invalidateQueries({ queryKey: PLUGINS_QUERY_KEY });

  const installTimers = useRef<number[]>([]);
  useEffect(() => () => {
    installTimers.current.forEach((id) => clearTimeout(id));
    installTimers.current = [];
  }, []);

  const enablePlugin = useAdminPluginServiceEnablePlugin({
    mutation: {
      onSuccess: () => { invalidate(); toast.success(t("admin.plugins.pluginEnabled")); },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugins.failedEnable"))),
    },
  });
  const disablePlugin = useAdminPluginServiceDisablePlugin({
    mutation: {
      onSuccess: () => { invalidate(); toast.success(t("admin.plugins.pluginDisabled")); },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugins.failedDisable"))),
    },
  });
  const uninstallPlugin = useAdminPluginServiceUninstallPlugin({
    mutation: {
      onSuccess: () => { invalidate(); toast.success(t("admin.plugins.pluginUninstalled")); },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugins.failedUninstall"))),
    },
  });

  function handleInstallSuccess() {
    invalidate();
    setShowModal(false);
    toast.success(t("admin.plugins.pluginInstalled"));
    // Cancel any delayed re-invalidations still pending from a previous install
    // before queuing new ones, so repeated installs in one session don't pile up
    // overlapping timers each firing a redundant refetch.
    installTimers.current.forEach((id) => clearTimeout(id));
    installTimers.current = [
      window.setTimeout(() => invalidate(), 3000),
      window.setTimeout(() => invalidate(), 8000),
    ];
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold text-[var(--text)]">
          {t("admin.plugins.title")}
        </h2>
        <Button onClick={() => setShowModal(true)}>{t("admin.plugins.installPlugin")}</Button>
      </div>

      {runtimes.length > 0 && (
        <div className="flex flex-col gap-2">
          <span className="text-xs font-semibold text-[var(--text-muted)]">
            {t("admin.plugins.availableRuntimes")}
          </span>
          <div className="flex flex-wrap gap-2">
            {runtimes.map((rt) => (
              <Badge key={rt.language} variant="success">
                {rt.language} {rt.version}
              </Badge>
            ))}
          </div>
        </div>
      )}

      <PluginResourceLimits />

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : plugins.length === 0 ? (
        <div
          className="text-center py-16 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <p className="text-[var(--text-muted)]">{t("admin.plugins.noPlugins")}</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {plugins.map((plugin) => (
            <PluginCard
              key={plugin.id}
              plugin={plugin}
              toggling={
                (enablePlugin.isPending && enablePlugin.variables?.id === plugin.id) ||
                (disablePlugin.isPending && disablePlugin.variables?.id === plugin.id)
              }
              onEnable={() => enablePlugin.mutate({ id: plugin.id! })}
              onDisable={() => disablePlugin.mutate({ id: plugin.id! })}
              onUninstall={() => setUninstallTarget({ id: plugin.id!, name: plugin.name ?? "this plugin" })}
            />
          ))}
        </div>
      )}

      <div className="border-t border-[var(--border)] pt-6">
        <PluginSecurityDashboard />
      </div>

      <Modal open={showModal} onClose={() => setShowModal(false)} title={t("admin.plugins.installPlugin")} size="xl">
        <InstallPluginForm onSuccess={handleInstallSuccess} />
      </Modal>

      <Modal open={!!uninstallTarget} onClose={() => setUninstallTarget(null)} title={t("admin.plugins.uninstallTitle")}>
        <p className="text-sm text-[var(--text-muted)] mb-4">
          {t("admin.plugins.uninstallConfirm", { name: uninstallTarget?.name })}
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setUninstallTarget(null)}>{t("common.cancel")}</Button>
          <Button
            variant="danger"
            loading={uninstallPlugin.isPending}
            onClick={() => {
              if (uninstallTarget) {
                uninstallPlugin.mutate({ id: uninstallTarget.id });
                setUninstallTarget(null);
              }
            }}
          >
            {t("admin.plugin.uninstall")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
