import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
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
import type { V1Plugin, V1PluginState } from "@evey/api/src/generated/admin.schemas";

// Extends generated type until orval adds permissions/capabilities
type PluginWithPermissions = V1Plugin & {
  permissions?: string[];
  capabilities?: string[];
};

type BadgeVariant = "default" | "success" | "warning" | "error" | "info";

function stateBadgeVariant(state?: V1PluginState): BadgeVariant {
  switch (state) {
    case "PLUGIN_STATE_RUNNING": return "success";
    case "PLUGIN_STATE_ERROR": return "error";
    case "PLUGIN_STATE_STOPPED": return "warning";
    case "PLUGIN_STATE_INSTALLED": return "info";
    default: return "default";
  }
}

function stateLabel(state?: V1PluginState): string {
  switch (state) {
    case "PLUGIN_STATE_RUNNING": return "Running";
    case "PLUGIN_STATE_ERROR": return "Error";
    case "PLUGIN_STATE_STOPPED": return "Stopped";
    case "PLUGIN_STATE_INSTALLED": return "Installed";
    default: return "Unknown";
  }
}

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
      <label className="flex items-center gap-2 text-sm" style={{ color: "var(--text)" }}>
        <input
          type="checkbox"
          checked={value === "true"}
          onChange={(e) => onChange(String(e.target.checked))}
          className="rounded"
        />
        <span>{fieldKey}</span>
        {schema.description && (
          <span className="text-xs" style={{ color: "var(--text-muted)" }}>{schema.description}</span>
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
  const queryClient = useQueryClient();
  const { data, isLoading } = useAdminServiceGetPluginConfig(pluginId);
  const [edits, setEdits] = useState<Record<string, string>>({});

  const updateConfig = useAdminServiceUpdatePluginConfig({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/plugins", pluginId, "config"] });
        setEdits({});
        toast.success("Config saved");
      },
      onError: (err: Error) => toast.error(err.message || "Failed to save config"),
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
      <p className="text-xs py-2" style={{ color: "var(--text-muted)" }}>
        No config keys defined.
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
            Save
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
  const [showConfig, setShowConfig] = useState(false);

  return (
    <Card className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-2">
        <div className="flex flex-col gap-0.5 min-w-0">
          <span className="font-semibold truncate" style={{ color: "var(--text)" }}>
            {plugin.name}
          </span>
          <span className="text-xs" style={{ color: "var(--text-muted)" }}>
            v{plugin.version} — {plugin.language}
          </span>
        </div>
        <div className="flex items-center gap-2 shrink-0">
          <Badge variant={stateBadgeVariant(plugin.state)}>{stateLabel(plugin.state)}</Badge>
          {plugin.enabled ? (
            <Badge variant="success">Enabled</Badge>
          ) : (
            <Badge variant="default">Disabled</Badge>
          )}
          {plugin.permissions?.includes("subprocess") && (
            <Badge variant="warning">Subprocess</Badge>
          )}
          {plugin.language !== "css" && !plugin.permissions?.includes("subprocess") && (
            <Badge variant="success">Sandboxed</Badge>
          )}
        </div>
      </div>

      {plugin.description && (
        <p className="text-sm" style={{ color: "var(--text-muted)" }}>
          {plugin.description}
        </p>
      )}

      <div className="flex gap-2 pt-1 flex-wrap">
        {plugin.enabled ? (
          <Button variant="secondary" onClick={onDisable}>Disable</Button>
        ) : (
          <Button variant="primary" onClick={onEnable}>Enable</Button>
        )}
        <Button variant="ghost" onClick={() => setShowConfig((v) => !v)}>
          {showConfig ? "Hide Config" : "Config"}
        </Button>
        <Link
          to="/admin/logs"
          className="inline-flex items-center px-3 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]"
        >
          Logs
        </Link>
        <Button variant="danger" onClick={onUninstall}>Uninstall</Button>
      </div>

      {showConfig && plugin.id && (
        <div
          className="border-t pt-3 mt-1"
          style={{ borderColor: "var(--border)" }}
        >
          <p className="text-xs font-semibold mb-2" style={{ color: "var(--text-muted)" }}>
            Configuration
          </p>
          <PluginConfigPanel pluginId={plugin.id} />
        </div>
      )}
    </Card>
  );
}
