import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Card } from "@/components/ui/Card";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Spinner } from "@/components/ui/Spinner";
import {
  useAdminServiceGetPluginConfig,
  useAdminServiceUpdatePluginConfig,
} from "@evey/api";
import { toast } from "@evey/store";
import type { V1Plugin, V1PluginState } from "@evey/api";

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

interface ConfigFieldSchema {
  type?: string;
  description?: string;
  default?: unknown;
}

interface PluginConfigPanelProps {
  pluginId: string;
  configSchema?: Record<string, ConfigFieldSchema>;
}

function SchemaField({
  fieldKey,
  schema,
  value,
  onChange,
}: {
  fieldKey: string;
  schema: ConfigFieldSchema;
  value: string;
  onChange: (val: string) => void;
}) {
  if (schema.type === "boolean") {
    return (
      <label className="flex items-center gap-2 text-sm text-[var(--text)]">
        <input
          type="checkbox"
          checked={value === "true"}
          onChange={(e) => onChange(String(e.target.checked))}
          className="rounded"
        />
        <span>{fieldKey}</span>
        {schema.description && (
          <span className="text-xs text-[var(--text-muted)]">{schema.description}</span>
        )}
      </label>
    );
  }
  if (schema.type === "integer" || schema.type === "number") {
    return (
      <Input
        label={fieldKey}
        type="number"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
    );
  }
  return (
    <Input
      label={fieldKey}
      value={value}
      onChange={(e) => onChange(e.target.value)}
    />
  );
}

function PluginConfigPanel({ pluginId, configSchema }: PluginConfigPanelProps) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { data, isLoading } = useAdminServiceGetPluginConfig(pluginId);
  const [edits, setEdits] = useState<Record<string, string>>({});

  const updateConfig = useAdminServiceUpdatePluginConfig({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/plugins", pluginId, "config"] });
        setEdits({});
        toast.success(t("admin.plugin.configSaved"));
      },
      onError: (err: Error) => toast.error(err.message || t("admin.plugin.configSaveFailed")),
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
    updateConfig.mutate({ pluginId, data: { values: current } });
  }

  return (
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
              onChange={(val) => setEdits((prev) => ({ ...prev, [key]: val }))}
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
  );
}

interface PluginCardProps {
  plugin: PluginWithPermissions;
  onEnable: () => void;
  onDisable: () => void;
  onUninstall: () => void;
}

export function PluginCard({ plugin, onEnable, onDisable, onUninstall }: PluginCardProps) {
  const { t } = useTranslation();
  const [showConfig, setShowConfig] = useState(false);

  const stateKey = STATE_LABEL_KEYS[plugin.state ?? ""] ?? "admin.plugin.state.unknown";

  return (
    <Card className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-2">
        <div className="flex flex-col gap-0.5 min-w-0">
          <span className="font-semibold truncate text-[var(--text)]">
            {plugin.name}
          </span>
          <span className="text-xs text-[var(--text-muted)] flex items-center gap-1.5">
            v{plugin.version} — {plugin.language}
            {plugin.sourceHash && (
              <button
                type="button"
                className="font-mono text-[10px] px-1.5 py-0.5 rounded bg-[var(--surface-raised)] hover:bg-[var(--surface-hover)] transition-colors-fast cursor-pointer"
                title={plugin.sourceHash}
                onClick={async () => {
                  try {
                    await navigator.clipboard.writeText(plugin.sourceHash!);
                    toast.success(t("admin.plugin.hashCopied"));
                  } catch { /* clipboard not available */ }
                }}
              >
                {plugin.sourceHash.slice(0, 8)}
              </button>
            )}
          </span>
        </div>
        <div className="flex items-center gap-2 shrink-0">
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

      <div className="flex gap-2 pt-1 flex-wrap">
        {plugin.enabled ? (
          <Button variant="secondary" onClick={onDisable}>{t("common.disable")}</Button>
        ) : (
          <Button variant="primary" onClick={onEnable}>{t("common.enable")}</Button>
        )}
        <Button variant="ghost" onClick={() => setShowConfig((v) => !v)}>
          {showConfig ? t("admin.plugin.hideConfig") : t("admin.plugin.config")}
        </Button>
        <Link
          to="/admin/logs"
          className="inline-flex items-center px-3 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]"
        >
          {t("admin.plugin.logs")}
        </Link>
        <Button variant="danger" onClick={onUninstall}>{t("admin.plugin.uninstall")}</Button>
      </div>

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
