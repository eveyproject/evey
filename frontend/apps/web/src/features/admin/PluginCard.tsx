import { useState, useMemo } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { type ConfigFieldSchema, normaliseOptions, validateConfigField } from "@evey/client";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Textarea } from "@/components/ui/Textarea";
import { Checkbox } from "@/components/ui/Checkbox";
import { Spinner } from "@/components/ui/Spinner";
import {
  useAdminPluginServiceGetPluginConfig,
  useAdminPluginServiceUpdatePluginConfig,
  useAdminPluginServiceGetPluginLogs,
  getAdminPluginServiceGetPluginConfigQueryKey,
  getAdminPluginServiceListPluginsQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import type { V1Plugin, V1PluginState } from "@evey/api";
import { PluginUpdateDialog } from "./PluginUpdateDialog";
import { useDemoLock } from "@/hooks/useDemoMode";
import { DemoReadOnly } from "@/components/layout/DemoReadOnly";

type PluginWithPermissions = V1Plugin & {
  permissions?: string[];
  capabilities?: string[];
};

type BadgeVariant = "default" | "success" | "warning" | "error" | "info";

function stateBadgeVariant(state?: V1PluginState | string): BadgeVariant {
  switch (state) {
    case "PLUGIN_STATE_RUNNING": return "success";
    case "PLUGIN_STATE_INITIALIZING": return "info";
    case "PLUGIN_STATE_ERROR": return "error";
    case "PLUGIN_STATE_STOPPED": return "warning";
    case "PLUGIN_STATE_INSTALLED": return "info";
    default: return "default";
  }
}

const STATE_LABEL_KEYS: Record<string, string> = {
  PLUGIN_STATE_RUNNING: "admin.plugin.state.running",
  PLUGIN_STATE_INITIALIZING: "admin.plugin.state.starting",
  PLUGIN_STATE_ERROR: "admin.plugin.state.error",
  PLUGIN_STATE_STOPPED: "admin.plugin.state.stopped",
  PLUGIN_STATE_INSTALLED: "admin.plugin.state.installed",
};

function SchemaField({
  fieldKey,
  schema,
  value,
  onChange,
  error,
}: {
  fieldKey: string;
  schema: ConfigFieldSchema;
  value: string;
  onChange: (val: string) => void;
  error?: string;
}) {
  const label = fieldKey + (schema.required ? " *" : "");

  const description = schema.description ? (
    <span className="text-xs text-[var(--text-muted)]">{schema.description}</span>
  ) : null;

  switch (schema.type) {
    case "boolean":
      return (
        <div className="flex flex-col gap-1">
          <Checkbox
            label={label}
            checked={value === "true"}
            onChange={(checked) => onChange(String(checked))}
          />
          {description}
        </div>
      );

    case "select":
      return (
        <div className="flex flex-col gap-1">
          <Select
            label={label}
            value={value}
            onChange={onChange}
            options={normaliseOptions(schema.options)}
            error={error}
          />
          {description}
        </div>
      );

    case "text":
      return (
        <div className="flex flex-col gap-1">
          <Textarea
            label={label}
            value={value}
            onChange={(e) => onChange(e.target.value)}
          />
          {description}
          {error && <span className="text-xs text-[var(--color-danger)]" role="alert">{error}</span>}
        </div>
      );

    case "password":
      return (
        <div className="flex flex-col gap-1">
          <Input
            label={label}
            type="password"
            value={value}
            onChange={(e) => onChange(e.target.value)}
            error={error}
          />
          {description}
        </div>
      );

    case "integer":
    case "number":
      return (
        <div className="flex flex-col gap-1">
          <Input
            label={label}
            type="number"
            value={value}
            onChange={(e) => onChange(e.target.value)}
            min={schema.min}
            max={schema.max}
            step={schema.type === "integer" ? 1 : "any"}
            error={error}
          />
          {description}
        </div>
      );

    case "string":
    default:
      return (
        <div className="flex flex-col gap-1">
          <Input
            label={label}
            value={value}
            onChange={(e) => onChange(e.target.value)}
            error={error}
          />
          {description}
        </div>
      );
  }
}

function PluginConfigPanel({ pluginId }: { pluginId: string }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data, isLoading } = useAdminPluginServiceGetPluginConfig(pluginId);
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});

  const configSchema: Record<string, ConfigFieldSchema> | undefined = useMemo(() => {
    if (!data?.configSchemaJson) return undefined;
    try { return JSON.parse(data.configSchemaJson); }
    catch (err) { console.warn("parse failed", err); return undefined; }
  }, [data?.configSchemaJson]);

  const updateConfig = useAdminPluginServiceUpdatePluginConfig({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: getAdminPluginServiceGetPluginConfigQueryKey(pluginId) });
        setEdits({});
        setErrors({});
        toast.success(t("admin.plugin.configSaved"));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.configSaveFailed"))),
    },
  });

  if (isLoading) {
    return (
      <div className="flex justify-center py-4">
        <Spinner size="sm" />
      </div>
    );
  }

  const values = data?.values ?? {};
  const schemaKeys = configSchema ? Object.keys(configSchema) : [];
  const keys = schemaKeys.length > 0 ? schemaKeys : Object.keys(values);

  if (keys.length === 0) {
    return (
      <p className="text-xs py-2 text-[var(--text-muted)]">
        {t("admin.plugin.noConfig")}
      </p>
    );
  }

  const current = { ...values, ...edits };
  const isDirty = Object.keys(edits).length > 0;

  function handleSave() {
    // Resolve each field exactly as it is rendered (fall back to the schema
    // default). Otherwise a required field that shows its default but was never
    // edited validates as empty ("<key> is required") and can never be saved.
    const resolved: Record<string, string> = { ...current };
    for (const key of keys) {
      const schema = configSchema?.[key];
      if (schema && resolved[key] == null && schema.default !== undefined) {
        resolved[key] = String(schema.default);
      }
    }
    if (configSchema) {
      const newErrors: Record<string, string> = {};
      for (const key of keys) {
        const schema = configSchema[key];
        if (schema) {
          const err = validateConfigField(key, resolved[key] ?? "", schema, t);
          if (err) newErrors[key] = err;
        }
      }
      if (Object.keys(newErrors).length > 0) {
        setErrors(newErrors);
        return;
      }
    }
    setErrors({});
    updateConfig.mutate({ pluginId, data: { values: resolved } });
  }

  // The seeded plugin is shared by every visitor: its config is read-only
  // on the demo (the server refuses the write, the fields say so).
  return (
    <DemoReadOnly>
    <div className="flex flex-col gap-3">
      {keys.map((key) => {
        const schema = configSchema?.[key];
        if (schema) {
          return (
            <SchemaField
              key={key}
              fieldKey={key}
              schema={schema}
              value={current[key] ?? String(schema.default ?? "")}
              onChange={(val) => {
                setEdits((prev) => ({ ...prev, [key]: val }));
                setErrors((prev) => {
                  const next = { ...prev };
                  delete next[key];
                  return next;
                });
              }}
              error={errors[key]}
            />
          );
        }
        return (
          <Input
            key={key}
            label={key}
            value={current[key] ?? ""}
            onChange={(e) => setEdits((prev) => ({ ...prev, [key]: e.target.value }))}
          />
        );
      })}
      {isDirty && (
        <div className="flex justify-end">
          <Button
            variant="primary"
            onClick={handleSave}
            loading={updateConfig.isPending}
          >
            {t("common.save")}
          </Button>
        </div>
      )}
    </div>
    </DemoReadOnly>
  );
}

interface PluginCardProps {
  plugin: PluginWithPermissions;
  onEnable: () => void;
  onDisable: () => void;
  onUninstall: () => void;
  // True while the parent's enable/disable mutation for THIS plugin is in
  // flight. Drives the toggle button's disabled state to prevent double-submit.
  // Sourced from the mutation's isPending so it clears on settle (success AND
  // error) - a local "set true on click" flag would stay stuck on a failed
  // toggle, since the plugin prop never changes to release it.
  toggling?: boolean;
}

export function PluginCard({ plugin, onEnable, onDisable, onUninstall, toggling = false }: PluginCardProps) {
  const { t } = useTranslation();
  // The plugin's own configuration stays editable on the demo (the panel
  // above); the lifecycle - enable, disable, update, uninstall - does not.
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const [showConfig, setShowConfig] = useState(false);
  const [showReport, setShowReport] = useState(false);
  const [showLogs, setShowLogs] = useState(false);
  const [updateSource, setUpdateSource] = useState<string | null>(null);
  const [showUpdateInput, setShowUpdateInput] = useState(false);
  const [updateSourceInput, setUpdateSourceInput] = useState("");
  const { data: logsData, isLoading: logsLoading } = useAdminPluginServiceGetPluginLogs(
    plugin.id ?? "",
    { limit: 100 },
    { query: { enabled: showLogs && !!plugin.id, staleTime: 10_000 } },
  );
  const analysisReport = plugin.analysisReport;
  const hasWarnings =
    analysisReport != null &&
    ((analysisReport.issues?.length ?? 0) > 0 ||
      (analysisReport.urls?.length ?? 0) > 0);

  const stateKey = STATE_LABEL_KEYS[plugin.state ?? ""] ?? "admin.plugin.state.unknown";

  return (
    <Card className="flex flex-col gap-3">
      {/* Wraps on a narrow card: the badge column is shrink-0 and carries up to
          three badges, which left the name/version column ~51px on a phone and
          clipped the source-hash chip out of the card. The text column now
          claims the space it needs and the badges drop to their own line. */}
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="flex flex-col gap-0.5 min-w-0 flex-1 basis-[14rem]">
          <span className="font-semibold truncate text-[var(--text)]">
            {plugin.name}
          </span>
          {/* Wraps: the version, the language and the source-hash chip sat on one
              unbreakable line, so on a narrow screen the hash was clipped out of
              the card entirely instead of moving under it. */}
          <span className="text-xs text-[var(--text-muted)] flex flex-wrap items-center gap-x-1.5 gap-y-0.5 min-w-0">
            v{plugin.version} - {plugin.language}
            {plugin.sourceHash && (
              <button
                type="button"
                className="font-mono text-xs px-1.5 py-0.5 rounded bg-[var(--surface-raised)] hover:bg-[var(--surface-hover)] transition-colors-fast cursor-pointer"
                title={plugin.sourceHash}
                aria-label={t("admin.plugin.hashCopied", "Copy hash")}
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(plugin.sourceHash!);
                    toast.success(t("admin.plugin.hashCopied"));
                  } catch (err) { console.warn("clipboard copy failed", err); }
                }}
              >
                {plugin.sourceHash.slice(0, 8)}
              </button>
            )}
          </span>
        </div>
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Badge variant={stateBadgeVariant(plugin.state)}>{t(stateKey)}</Badge>
          {plugin.enabled ? (
            <Badge variant="success">{t("admin.plugin.enabled")}</Badge>
          ) : (
            <Badge variant="default">{t("admin.plugin.disabled")}</Badge>
          )}
          {plugin.permissions?.includes("subprocess") && (
            <Badge variant="warning">{t("admin.plugin.subprocess")}</Badge>
          )}
          {plugin.language !== "css" && !plugin.permissions?.includes("subprocess") && (
            <Badge variant="success">{t("admin.plugin.sandboxed")}</Badge>
          )}
        </div>
      </div>

      {plugin.description && (
        <p className="text-sm text-[var(--text-muted)]">
          {plugin.description}
        </p>
      )}

      {plugin.state === "PLUGIN_STATE_ERROR" && plugin.errorMessage && (
        <div className="rounded border border-[var(--color-danger,#7f1d1d)] bg-[var(--badge-error-bg,rgba(127,29,29,0.15))] px-3 py-2 text-xs">
          <p className="font-semibold text-[var(--badge-error-text,#fca5a5)] mb-1">
            {t("admin.plugin.errorDetails", "Error details")}
          </p>
          <p className="font-mono text-[var(--text)] break-all whitespace-pre-wrap">
            {plugin.errorMessage}
          </p>
        </div>
      )}

      <div className="flex gap-2 pt-1 flex-wrap">
        {plugin.enabled ? (
          <Button variant="secondary" disabled={toggling} onClick={onDisable} {...lock}>{t("common.disable")}</Button>
        ) : (
          <Button variant="primary" disabled={toggling} onClick={onEnable} {...lock}>{t("common.enable")}</Button>
        )}
        <Button variant="ghost" onClick={() => setShowConfig((v) => !v)}>
          {showConfig ? t("admin.plugin.hideConfig") : t("admin.plugin.config")}
        </Button>
        <Button variant="ghost" onClick={() => setShowLogs((v) => !v)}>
          {showLogs ? t("admin.plugin.hideLogs") : t("admin.plugin.logs")}
        </Button>
        <Button
          variant="secondary"
          onClick={() => setShowUpdateInput((v) => !v)}
          data-testid="plugin-update-toggle"
          {...lock}
        >
          {t("admin.plugin.update", "Update from source...")}
        </Button>
        <Button variant="danger" onClick={onUninstall} {...lock}>{t("admin.plugin.uninstall")}</Button>
      </div>

      {showUpdateInput && (
        <div className="border-t pt-3 mt-1 border-[var(--border)] flex flex-col gap-2">
          <p className="text-xs font-semibold text-[var(--text-muted)]">
            {t("admin.plugin.updateSourceLabel", "Source ZIP (path or URL)")}
          </p>
          <div className="flex gap-2 items-end">
            <Input
              label=""
              placeholder="/path/to/plugin-v2.zip"
              value={updateSourceInput}
              onChange={(e) => setUpdateSourceInput(e.target.value)}
            />
            <Button
              variant="primary"
              onClick={() => {
                if (updateSourceInput.trim()) {
                  setUpdateSource(updateSourceInput.trim());
                  setShowUpdateInput(false);
                }
              }}
              disabled={!updateSourceInput.trim()}
              data-testid="plugin-update-preview-btn"
            >
              {t("admin.plugin.previewUpdate", "Preview")}
            </Button>
          </div>
        </div>
      )}

      {updateSource && plugin.id && (
        <PluginUpdateDialog
          pluginId={plugin.id}
          pluginName={plugin.name}
          source={updateSource}
          onClose={() => {
            setUpdateSource(null);
            setUpdateSourceInput("");
          }}
          onApplied={() => {
            setUpdateSource(null);
            setUpdateSourceInput("");
            // Refresh the card's version / source hash / permission badges.
            void queryClient.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() });
          }}
        />
      )}

      {showLogs && (
        <div className="border-t pt-3 mt-1 border-[var(--border)]">
          {logsLoading ? (
            <div className="flex justify-center py-4"><Spinner size="sm" /></div>
          ) : (logsData?.entries?.length ?? 0) === 0 ? (
            <p className="text-xs text-[var(--text-muted)] py-2">{t("admin.plugin.noLogs")}</p>
          ) : (
            <div className="flex flex-col gap-1 max-h-64 overflow-y-auto font-mono text-xs">
              {logsData!.entries!.map((entry, i) => (
                <div key={i} className="flex gap-2 items-start">
                  <span className={
                    entry.level === "ERROR" ? "text-[var(--badge-error-text,#fca5a5)] shrink-0" :
                    entry.level === "WARN" ? "text-[var(--badge-warning-text,#fde047)] shrink-0" :
                    "text-[var(--text-muted)] shrink-0"
                  }>
                    {/* `||` et non `??` : une ligne de stdout brut du plugin
                        arrive avec un niveau VIDE, pas absent - `??` laissait
                        alors la colonne blanche. */}
                    {entry.level || "INFO"}
                  </span>
                  <span className="text-[var(--text-muted)] shrink-0">
                    {entry.timestamp ? new Date(entry.timestamp).toLocaleTimeString() : ""}
                  </span>
                  <span className="text-[var(--text)] break-all">{entry.message}</span>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {hasWarnings && (
        <div className="border-t pt-3 mt-1 border-[var(--border)]">
          <button
            type="button"
            onClick={() => setShowReport((v) => !v)}
            className="text-xs font-semibold text-[var(--color-warning,var(--text))] flex items-center gap-2 cursor-pointer"
            data-testid="plugin-analysis-toggle"
          >
            {t("admin.plugin.analysisReport.title", "Security notice")}
            <Badge variant="warning">
              {(analysisReport?.issues?.length ?? 0) + (analysisReport?.urls?.length ?? 0)}
            </Badge>
          </button>
          {showReport && (
            <div className="flex flex-col gap-2 mt-2 text-xs text-[var(--text-muted)]">
              {analysisReport?.issues && analysisReport.issues.length > 0 && (
                <div>
                  <p className="font-semibold text-[var(--text)]">
                    {t("admin.plugin.analysisReport.issues", "Issues")}
                  </p>
                  <ul className="list-disc pl-5 mt-1">
                    {analysisReport.issues.map((iss, i) => (
                      <li key={`iss-${i}`}>{iss}</li>
                    ))}
                  </ul>
                </div>
              )}
              {analysisReport?.urls && analysisReport.urls.length > 0 && (
                <div>
                  <p className="font-semibold text-[var(--text)]">
                    {t("admin.plugin.analysisReport.urls", "External URLs")}
                  </p>
                  <ul className="list-disc pl-5 mt-1">
                    {analysisReport.urls.map((u, i) => (
                      <li key={`url-${i}`} className="break-all">
                        <span className="font-mono">{u.url}</span>
                        {u.file && (
                          <span className="text-[var(--text-muted)]"> - {u.file}:{u.line ?? 0}</span>
                        )}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {showConfig && plugin.id && (
        <div
          className="border-t pt-3 mt-1 border-[var(--border)]"
        >
          <p className="text-xs font-semibold mb-2 text-[var(--text-muted)]">
            {t("admin.plugin.configuration")}
          </p>
          <PluginConfigPanel pluginId={plugin.id} />
        </div>
      )}
    </Card>
  );
}
