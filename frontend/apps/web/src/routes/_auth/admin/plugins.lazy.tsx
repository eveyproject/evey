import { useState, useRef } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import {
  useAdminServiceListPlugins,
  useAdminServiceEnablePlugin,
  useAdminServiceDisablePlugin,
  useAdminServiceUninstallPlugin,
  useAdminServiceInstallPlugin,
} from "@evey/api";
import { useAuthStore, toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Input } from "@/components/ui/Input";
import { Badge } from "@/components/ui/Badge";
import { PluginCard } from "@/features/admin/PluginCard";

export const Route = createLazyFileRoute("/_auth/admin/plugins")({
  component: PluginsPage,
});

const PLUGINS_QUERY_KEY = ["/api/v1/admin/plugins"];

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
    total_files_scanned: number;
  };
}

type InstallStep = "idle" | "previewing" | "reviewing" | "installing" | "done";

function SubprocessWarning() {
  return (
    <div
      className="flex items-start gap-3 p-4 rounded-[var(--radius-md)]"
      style={{
        backgroundColor: "var(--badge-warning-bg, #713f12)",
        border: "1px solid var(--badge-warning-text, #fde047)",
      }}
    >
      <svg
        className="w-5 h-5 shrink-0 mt-0.5"
        style={{ color: "var(--badge-warning-text, #fde047)" }}
        viewBox="0 0 20 20"
        fill="currentColor"
      >
        <path
          fillRule="evenodd"
          d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.168 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z"
          clipRule="evenodd"
        />
      </svg>
      <div>
        <p
          className="font-semibold text-sm"
          style={{ color: "var(--badge-warning-text, #fde047)" }}
        >
          Subprocess Permission Required
        </p>
        <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>
          This plugin can execute system commands, bypassing the seccomp sandbox.
          Only install if you trust the author.
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
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <span className="font-semibold" style={{ color: "var(--text)" }}>
          {preview.name}
        </span>
        <span className="text-xs" style={{ color: "var(--text-muted)" }}>
          v{preview.version} — {preview.language}
          {preview.author ? ` — by ${preview.author}` : ""}
        </span>
        {preview.description && (
          <p className="text-sm mt-1" style={{ color: "var(--text-muted)" }}>
            {preview.description}
          </p>
        )}
      </div>

      {preview.permissions?.length > 0 && (
        <div className="flex flex-col gap-2">
          <span className="text-xs font-semibold" style={{ color: "var(--text-muted)" }}>
            Permissions
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
          <span className="text-xs font-semibold" style={{ color: "var(--text-muted)" }}>
            Capabilities
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
        <div className="flex flex-col gap-1 text-xs" style={{ color: "var(--text-muted)" }}>
          <span className="font-semibold">Static Analysis</span>
          {preview.analysis.issues.map((issue, i) => (
            <span key={i}>{issue}</span>
          ))}
        </div>
      )}

      <div className="flex justify-end gap-2 pt-2">
        <Button variant="ghost" onClick={onCancel}>Cancel</Button>
        <Button
          variant={hasWarnings ? "danger" : "primary"}
          loading={installing}
          onClick={onConfirm}
        >
          {hasWarnings ? "Install Anyway" : "Install"}
        </Button>
      </div>
    </div>
  );
}

function InstallPluginForm({ onSuccess }: { onSuccess: () => void }) {
  const [mode, setMode] = useState<"file" | "url">("file");
  const [step, setStep] = useState<InstallStep>("idle");
  const [preview, setPreview] = useState<PluginPreview | null>(null);
  const [error, setError] = useState("");
  const [blockedIssues, setBlockedIssues] = useState<string[]>([]);
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const fileRef = useRef<HTMLInputElement>(null);
  const token = useAuthStore((s) => s.accessToken);

  // URL mode state
  const [source, setSource] = useState("");
  const [name, setName] = useState("");
  const installPlugin = useAdminServiceInstallPlugin({ mutation: { onSuccess } });

  function getFile(): File | null {
    return fileRef.current?.files?.[0] ?? null;
  }

  async function handleFilePreview() {
    const file = getFile();
    if (!file) { toast.error("Select a plugin file first"); return; }
    setSelectedFile(file);
    setStep("previewing");
    setError("");
    try {
      const formData = new FormData();
      formData.append("plugin", file);
      // Raw fetch required: multipart upload (eveyFetch is JSON-only)
      const res = await fetch("/api/v1/admin/plugins/preview", {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: formData,
      });
      if (res.status === 401) window.dispatchEvent(new Event("evey:unauthorized"));
      if (!res.ok) {
        const body = await res.json().catch(() => ({})) as { message?: string };
        throw new Error(body.message || `Preview failed (${res.status})`);
      }
      const data = (await res.json()) as PluginPreview;
      setPreview(data);
      setStep("reviewing");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Preview failed");
      setStep("idle");
    }
  }

  async function handleFileInstall() {
    const file = selectedFile;
    if (!file) { toast.error("File missing — select again"); return; }
    setStep("installing");
    setError("");
    try {
      const formData = new FormData();
      formData.append("plugin", file);
      // Raw fetch required: multipart upload (eveyFetch is JSON-only)
      const res = await fetch("/api/v1/admin/plugins/upload", {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        body: formData,
      });
      if (res.status === 401) window.dispatchEvent(new Event("evey:unauthorized"));
      if (!res.ok) {
        const body = await res.json().catch(() => ({})) as { message?: string; blocked?: boolean; issues?: string[] };
        if (body.blocked && body.issues) {
          setBlockedIssues(body.issues);
          setError("Plugin blocked by security analysis");
          setStep("reviewing");
          return;
        }
        throw new Error(body.message || `Install failed (${res.status})`);
      }
      setStep("done");
      onSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Install failed");
      setStep("reviewing");
    }
  }

  function resetFileFlow() {
    setStep("idle");
    setPreview(null);
    setError("");
    setBlockedIssues([]);
    setSelectedFile(null);
  }

  function handleUrlSubmit(e: React.FormEvent) {
    e.preventDefault();
    installPlugin.mutate({ data: { source, name } });
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
          Upload File
        </Button>
        <Button
          variant={mode === "url" ? "primary" : "ghost"}
          onClick={() => { setMode("url"); resetFileFlow(); }}
        >
          From URL
        </Button>
      </div>

      {mode === "file" && step !== "reviewing" && step !== "installing" && (
        <div className="flex flex-col gap-4">
          <div>
            <label
              className="block text-sm font-medium mb-1"
              style={{ color: "var(--text-muted)" }}
            >
              Plugin file (.zip)
            </label>
            <input
              ref={fileRef}
              type="file"
              accept=".zip"
              className="block w-full text-sm"
              style={{ color: "var(--text)" }}
            />
          </div>
          {error && (
            <p className="text-xs" style={{ color: "var(--badge-error-text, #fca5a5)" }}>
              {error}
            </p>
          )}
          <div className="flex justify-end pt-2">
            <Button
              onClick={handleFilePreview}
              loading={step === "previewing"}
            >
              Preview &amp; Install
            </Button>
          </div>
        </div>
      )}

      {mode === "file" && (step === "reviewing" || step === "installing") && preview && (
        <>
          {error && (
            <div className="flex flex-col gap-2 p-3 rounded-[var(--radius-md)]" style={{ backgroundColor: "var(--badge-error-bg, #7f1d1d)", border: "1px solid var(--badge-error-text, #fca5a5)" }}>
              <p className="text-sm font-semibold" style={{ color: "var(--badge-error-text, #fca5a5)" }}>
                {error}
              </p>
              {blockedIssues.length > 0 && (
                <ul className="flex flex-col gap-1 text-xs list-disc pl-4" style={{ color: "var(--text-muted)" }}>
                  {blockedIssues.map((issue, i) => (
                    <li key={i}>{issue}</li>
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
            onConfirm={handleFileInstall}
          />
        </>
      )}

      {mode === "url" && (
        <form onSubmit={handleUrlSubmit} className="flex flex-col gap-4">
          <p className="text-xs" style={{ color: "var(--text-muted)" }}>
            Note: URL installs skip the preview step.
          </p>
          <Input
            label="Source URL"
            value={source}
            onChange={(e) => setSource(e.target.value)}
            placeholder="https://example.com/plugin.zip"
            required
          />
          <Input
            label="Name (optional)"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-plugin"
          />
          <div className="flex justify-end pt-2">
            <Button type="submit" loading={installPlugin.isPending} disabled={!source.trim()}>
              Install
            </Button>
          </div>
        </form>
      )}
    </div>
  );
}

function PluginsPage() {
  const [showModal, setShowModal] = useState(false);
  const [uninstallTarget, setUninstallTarget] = useState<{ id: string; name: string } | null>(null);
  const queryClient = useQueryClient();
  const { data, isLoading } = useAdminServiceListPlugins();
  const plugins = data?.plugins ?? [];

  const invalidate = () => queryClient.invalidateQueries({ queryKey: PLUGINS_QUERY_KEY });

  const enablePlugin = useAdminServiceEnablePlugin({
    mutation: {
      onSuccess: () => { invalidate(); toast.success("Plugin enabled"); },
      onError: (err: Error) => toast.error(err.message || "Failed to enable plugin"),
    },
  });
  const disablePlugin = useAdminServiceDisablePlugin({
    mutation: {
      onSuccess: () => { invalidate(); toast.success("Plugin disabled"); },
      onError: (err: Error) => toast.error(err.message || "Failed to disable plugin"),
    },
  });
  const uninstallPlugin = useAdminServiceUninstallPlugin({
    mutation: {
      onSuccess: () => { invalidate(); toast.success("Plugin uninstalled"); },
      onError: (err: Error) => toast.error(err.message || "Failed to uninstall plugin"),
    },
  });

  function handleInstallSuccess() {
    invalidate();
    setShowModal(false);
    toast.success("Plugin installed");
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold" style={{ color: "var(--text)" }}>
          Plugins
        </h2>
        <Button onClick={() => setShowModal(true)}>Install Plugin</Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : plugins.length === 0 ? (
        <div
          className="text-center py-16 rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <p style={{ color: "var(--text-muted)" }}>No plugins installed.</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {plugins.map((plugin) => (
            <PluginCard
              key={plugin.id}
              plugin={plugin}
              onEnable={() => enablePlugin.mutate({ id: plugin.id! })}
              onDisable={() => disablePlugin.mutate({ id: plugin.id! })}
              onUninstall={() => setUninstallTarget({ id: plugin.id!, name: plugin.name ?? "this plugin" })}
            />
          ))}
        </div>
      )}

      <Modal open={showModal} onClose={() => setShowModal(false)} title="Install Plugin">
        <InstallPluginForm onSuccess={handleInstallSuccess} />
      </Modal>

      <Modal open={!!uninstallTarget} onClose={() => setUninstallTarget(null)} title="Uninstall Plugin">
        <p className="text-sm text-[var(--text-muted)] mb-4">
          Uninstall plugin "{uninstallTarget?.name}"? This cannot be undone.
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setUninstallTarget(null)}>Cancel</Button>
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
            Uninstall
          </Button>
        </div>
      </Modal>
    </div>
  );
}
