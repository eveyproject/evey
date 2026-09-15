import { useCallback, useEffect, useMemo, useState } from "react";
import { View, Text, FlatList, Pressable, Switch, Alert, ActivityIndicator, Modal, TextInput, StyleSheet, ScrollView } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useAdminPluginServiceListPlugins,
  useAdminPluginServiceEnablePlugin,
  useAdminPluginServiceDisablePlugin,
  useAdminPluginServiceUninstallPlugin,
  useAdminPluginServiceInstallPlugin,
  adminPluginServiceInstallPlugin,
  NO_TIMEOUT,
  useAdminPluginServiceGetPluginConfig,
  useAdminPluginServiceUpdatePluginConfig,
  useAdminPluginServiceGetPluginLogs,
  getAdminPluginServiceListPluginsQueryKey,
  getAdminPluginServiceGetPluginConfigQueryKey,
  eveyFetch,
  apiErrorMessage,
} from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";
import {
  type ConfigFieldSchema,
  normaliseOptions,
  validateConfigField,
  parseEscalationEntries,
  parseBlockedIssues,
  type InstallPreview,
  installPreviewHasWarnings,
} from "../../../src/components/admin/plugins/pluginConfigUtils";
import { PluginResourceLimitsCard } from "../../../src/components/admin/plugins/PluginResourceLimitsCard";
import { PluginSecurityDashboard } from "../../../src/components/admin/plugins/PluginSecurityDashboard";
import { previewPluginFile, installPluginFile } from "../../../src/components/admin/plugins/pluginFileInstall";
import * as DocumentPicker from "expo-document-picker";

type Theme = ReturnType<typeof useTheme>;

interface DetectedRuntime { language: string; version: string }

interface PluginExtractedURL {
  url: string;
  file?: string;
  line?: number;
}

interface PluginAnalysisReport {
  safe?: boolean;
  issues?: string[];
  urls?: PluginExtractedURL[];
}

interface Plugin {
  id: string;
  name: string;
  version: string;
  description?: string;
  enabled: boolean;
  author?: string;
  language?: string;
  state?: string;
  permissions?: string[];
  capabilities?: string[];
  sourceHash?: string;
  errorMessage?: string;
  analysisReport?: PluginAnalysisReport;
}

const STATE_LABEL_KEYS: Record<string, string> = {
  PLUGIN_STATE_RUNNING: "admin.plugin.state.running",
  PLUGIN_STATE_INITIALIZING: "admin.plugin.state.starting",
  PLUGIN_STATE_ERROR: "admin.plugin.state.error",
  PLUGIN_STATE_STOPPED: "admin.plugin.state.stopped",
  PLUGIN_STATE_INSTALLED: "admin.plugin.state.installed",
};

const STATE_LABEL_FALLBACKS: Record<string, string> = {
  PLUGIN_STATE_RUNNING: "En cours",
  PLUGIN_STATE_INITIALIZING: "Démarrage",
  PLUGIN_STATE_ERROR: "Erreur",
  PLUGIN_STATE_STOPPED: "Arrêté",
  PLUGIN_STATE_INSTALLED: "Installé",
};

function stateColor(theme: Theme, state?: string): string {
  switch (state) {
    case "PLUGIN_STATE_RUNNING": return theme.colors.success;
    case "PLUGIN_STATE_ERROR": return theme.colors.error;
    case "PLUGIN_STATE_STOPPED": return theme.colors.warning;
    default: return theme.colors.textMuted;
  }
}

function inputStyle(theme: Theme) {
  return {
    backgroundColor: theme.colors.bg,
    color: theme.colors.text,
    borderRadius: theme.radii.md,
    padding: theme.spacing.sm + 2,
    borderWidth: 1,
    borderColor: theme.colors.border,
    fontSize: 13,
  };
}

function StatusBadge({ label, color }: { label: string; color: string }) {
  return (
    <View style={[styles.badge, { borderColor: color }]}>
      <Text style={{ fontSize: 10, fontWeight: "700", color }}>{label}</Text>
    </View>
  );
}

// --- Per-plugin configuration panel, driven by configSchemaJson --------

function SchemaField({
  fieldKey,
  schema,
  value,
  onChange,
  error,
  theme,
}: {
  fieldKey: string;
  schema?: ConfigFieldSchema;
  value: string;
  onChange: (val: string) => void;
  error?: string;
  theme: Theme;
}) {
  const label = fieldKey + (schema?.required ? " *" : "");

  if (schema?.type === "boolean") {
    return (
      <View style={{ flexDirection: "row", alignItems: "center", justifyContent: "space-between", gap: 8 }}>
        <View style={{ flex: 1 }}>
          <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{label}</Text>
          {schema.description ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{schema.description}</Text>
          ) : null}
        </View>
        <Switch
          value={value === "true"}
          onValueChange={(v) => onChange(String(v))}
          trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
        />
      </View>
    );
  }

  if (schema?.type === "select") {
    const options = normaliseOptions(schema.options);
    return (
      <View style={{ gap: 6 }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontWeight: "600" }}>{label}</Text>
        <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 6 }}>
          {options.map((o) => (
            <Pressable
              key={o.value}
              onPress={() => onChange(o.value)}
              style={[
                styles.chip,
                {
                  borderColor: value === o.value ? theme.colors.accent : theme.colors.border,
                  backgroundColor: value === o.value ? theme.colors.accent : "transparent",
                },
              ]}
            >
              <Text style={{ fontSize: 12, color: value === o.value ? theme.colors.onAccent : theme.colors.text }}>
                {o.label}
              </Text>
            </Pressable>
          ))}
        </View>
        {error ? <Text style={{ color: theme.colors.error, fontSize: 11 }}>{error}</Text> : null}
      </View>
    );
  }

  const keyboardType = schema?.type === "integer" || schema?.type === "number" ? "numeric" : "default";
  const multiline = schema?.type === "text";
  const secure = schema?.type === "password";

  return (
    <View style={{ gap: 4 }}>
      <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontWeight: "600" }}>{label}</Text>
      <TextInput
        accessibilityLabel={label}
        value={value}
        onChangeText={onChange}
        placeholderTextColor={theme.colors.textMuted}
        keyboardType={keyboardType}
        multiline={multiline}
        numberOfLines={multiline ? 3 : 1}
        secureTextEntry={secure}
        autoCapitalize={secure ? "none" : undefined}
        style={[inputStyle(theme), multiline ? { minHeight: 72, textAlignVertical: "top" as const } : null]}
      />
      {schema?.description ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{schema.description}</Text>
      ) : null}
      {error ? <Text style={{ color: theme.colors.error, fontSize: 11 }}>{error}</Text> : null}
    </View>
  );
}

function PluginConfigSection({ pluginId, theme, t }: { pluginId: string; theme: Theme; t: TFunction }) {
  const qc = useQueryClient();
  const { data, isLoading } = useAdminPluginServiceGetPluginConfig(pluginId);
  const [edits, setEdits] = useState<Record<string, string>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});

  const configSchema: Record<string, ConfigFieldSchema> | undefined = useMemo(() => {
    if (!data?.configSchemaJson) return undefined;
    try {
      return JSON.parse(data.configSchemaJson);
    } catch {
      return undefined;
    }
  }, [data?.configSchemaJson]);

  const updateConfig = useAdminPluginServiceUpdatePluginConfig({
    mutation: {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getAdminPluginServiceGetPluginConfigQueryKey(pluginId) });
        setEdits({});
        setErrors({});
        toast.success(t("admin.plugin.configSaved", "Configuration enregistrée"));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.configSaveFailed", "Échec de l'enregistrement"))),
    },
  });

  if (isLoading) {
    return <ActivityIndicator color={theme.colors.accent} size="small" style={{ paddingVertical: 12 }} />;
  }

  const values = data?.values ?? {};
  const schemaKeys = configSchema ? Object.keys(configSchema) : [];
  const keys = schemaKeys.length > 0 ? schemaKeys : Object.keys(values);

  if (keys.length === 0) {
    return (
      <Text style={{ color: theme.colors.textMuted, fontSize: 12, paddingVertical: 8 }}>
        {t("admin.plugin.noConfig", "Ce plugin n'a pas de configuration")}
      </Text>
    );
  }

  const current = { ...values, ...edits };
  const isDirty = Object.keys(edits).length > 0;

  function handleChange(key: string, val: string) {
    setEdits((prev) => ({ ...prev, [key]: val }));
    setErrors((prev) => {
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }

  function handleSave() {
    // Resolve each field exactly as rendered (fall back to the schema default),
    // otherwise a required field showing its default but never edited would
    // validate as empty and could never be saved.
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
          const err = validateConfigField(key, resolved[key] ?? "", schema);
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

  return (
    <View style={{ gap: 10 }}>
      {keys.map((key) => {
        const schema = configSchema?.[key];
        const value = current[key] ?? String(schema?.default ?? "");
        return (
          <SchemaField
            key={key}
            fieldKey={key}
            schema={schema}
            value={value}
            onChange={(val) => handleChange(key, val)}
            error={errors[key]}
            theme={theme}
          />
        );
      })}
      {isDirty ? (
        <Pressable
          onPress={handleSave}
          disabled={updateConfig.isPending}
          style={[styles.smallBtn, { backgroundColor: theme.colors.accent, opacity: updateConfig.isPending ? 0.5 : 1 }]}
        >
          {updateConfig.isPending ? (
            <ActivityIndicator color={theme.colors.onAccent} size="small" />
          ) : (
            <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 13 }}>{t("common.save")}</Text>
          )}
        </Pressable>
      ) : null}
    </View>
  );
}

// --- Per-plugin logs panel ----------------------------------------------

function PluginLogsSection({ pluginId, theme, t }: { pluginId: string; theme: Theme; t: TFunction }) {
  const { data, isLoading } = useAdminPluginServiceGetPluginLogs(
    pluginId,
    { limit: 100 },
    { query: { staleTime: 10_000 } },
  );

  if (isLoading) {
    return <ActivityIndicator color={theme.colors.accent} size="small" style={{ paddingVertical: 12 }} />;
  }

  const entries = data?.entries ?? [];
  if (entries.length === 0) {
    return (
      <Text style={{ color: theme.colors.textMuted, fontSize: 12, paddingVertical: 8 }}>
        {t("admin.plugin.noLogs", "Aucun journal pour l'instant")}
      </Text>
    );
  }

  return (
    <ScrollView style={{ maxHeight: 220 }} nestedScrollEnabled>
      {entries.map((entry, i) => (
        <View key={i} style={{ flexDirection: "row", gap: 6, paddingVertical: 2, flexWrap: "wrap" }}>
          <Text
            style={{
              fontSize: 10,
              fontWeight: "700",
              color: entry.level === "ERROR" ? theme.colors.error : entry.level === "WARN" ? theme.colors.warning : theme.colors.textMuted,
            }}
          >
            {entry.level || "INFO"}
          </Text>
          <Text style={{ fontSize: 10, color: theme.colors.textMuted }}>
            {entry.timestamp ? new Date(entry.timestamp).toLocaleTimeString() : ""}
          </Text>
          <Text style={{ fontSize: 11, color: theme.colors.text, flexShrink: 1 }}>{entry.message}</Text>
        </View>
      ))}
    </ScrollView>
  );
}

// --- Install-from-URL preview -------------------------------------------

function InstallPreviewPanel({ preview, theme, t, installing, onCancel, onConfirm }: {
  preview: InstallPreview;
  theme: Theme;
  t: TFunction;
  installing: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const warn = installPreviewHasWarnings(preview);
  const issues = preview.analysis?.issues ?? [];
  const urls = preview.analysis?.urls ?? [];
  return (
    <>
      <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "700" }}>{preview.name}</Text>
      <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
        v{preview.version}{preview.language ? ` - ${preview.language}` : ""}{preview.author ? ` - ${preview.author}` : ""}
      </Text>
      {preview.description ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{preview.description}</Text>
      ) : null}
      {(preview.permissions?.length ?? 0) > 0 ? (
        <View style={{ gap: 2 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontWeight: "600" }}>{t("admin.plugins.permissions", "Permissions")}</Text>
          <Text style={{ color: theme.colors.text, fontSize: 12, fontFamily: "monospace" }}>{preview.permissions!.join(", ")}</Text>
        </View>
      ) : null}
      {(preview.capabilities?.length ?? 0) > 0 ? (
        <View style={{ gap: 2 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontWeight: "600" }}>{t("admin.plugins.capabilities", "Capabilities")}</Text>
          <Text style={{ color: theme.colors.text, fontSize: 12, fontFamily: "monospace" }}>{preview.capabilities!.join(", ")}</Text>
        </View>
      ) : null}
      {issues.length > 0 ? (
        <View style={{ flexDirection: "row", alignItems: "flex-start", gap: 8 }}>
          <Ionicons name="alert-circle-outline" size={20} color={theme.colors.warning} />
          <View style={{ flex: 1, gap: 4 }}>
            {issues.map((issue, i) => (
              <Text
                key={i}
                style={{
                  color: issue.startsWith("[CRITICAL]") ? theme.colors.error : issue.startsWith("[WARNING]") ? theme.colors.warning : theme.colors.textMuted,
                  fontSize: 12,
                  fontFamily: "monospace",
                }}
              >
                {issue}
              </Text>
            ))}
          </View>
        </View>
      ) : null}
      {urls.length > 0 ? (
        <View style={{ gap: 2 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontWeight: "600" }}>{t("admin.plugin.analysisReport.urls", "Detected URLs")}</Text>
          {urls.map((u, i) => (
            <Text key={i} style={{ color: theme.colors.textMuted, fontSize: 11, fontFamily: "monospace" }} numberOfLines={1}>{u}</Text>
          ))}
        </View>
      ) : null}
      <View style={{ flexDirection: "row", gap: theme.spacing.sm, marginTop: theme.spacing.sm }}>
        <Pressable
          onPress={onCancel}
          style={{ flex: 1, alignItems: "center", paddingVertical: theme.spacing.sm + 2, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.border }}
        >
          <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("common.cancel")}</Text>
        </Pressable>
        <Pressable
          onPress={onConfirm}
          disabled={installing}
          style={{
            flex: 1,
            alignItems: "center",
            paddingVertical: theme.spacing.sm + 2,
            borderRadius: theme.radii.md,
            backgroundColor: warn ? theme.colors.warning : theme.colors.accent,
            opacity: installing ? 0.5 : 1,
          }}
        >
          {installing ? (
            <ActivityIndicator color={theme.colors.onAccent} size="small" />
          ) : (
            <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>
              {warn ? t("admin.plugins.installAnyway", "Install anyway") : t("admin.plugins.install", "Install")}
            </Text>
          )}
        </Pressable>
      </View>
    </>
  );
}

// --- Update-from-source preview/apply dialog ----------------------------

interface UpdatePreview {
  currentVersion?: string;
  candidateVersion?: string;
  permissionsAdded?: string[];
  permissionsRemoved?: string[];
  capabilitiesAdded?: string[];
  capabilitiesRemoved?: string[];
  slotsAdded?: string[];
  slotsRemoved?: string[];
  externalFetchAdded?: string[];
  externalFetchRemoved?: string[];
  newFindings?: { category?: string; severity?: string; file?: string; line?: number; description?: string }[];
  signatureStatus?: string;
  requiresAck?: boolean;
  sourceHash?: string;
}

function DiffRows({ theme, label, added, removed }: { theme: Theme; label: string; added?: string[]; removed?: string[] }) {
  const hasContent = (added?.length ?? 0) > 0 || (removed?.length ?? 0) > 0;
  return (
    <View style={{ marginTop: 10 }}>
      <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "700", textTransform: "uppercase" }}>{label}</Text>
      {!hasContent ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>-</Text>
      ) : (
        <View style={{ marginTop: 4, gap: 2 }}>
          {(added ?? []).map((item) => (
            <Text key={`add-${item}`} style={{ color: theme.colors.error, fontSize: 12 }}>+ {item}</Text>
          ))}
          {(removed ?? []).map((item) => (
            <Text key={`rem-${item}`} style={{ color: theme.colors.textMuted, fontSize: 12 }}>- {item}</Text>
          ))}
        </View>
      )}
    </View>
  );
}

function PluginUpdateModal({
  pluginId,
  pluginName,
  source,
  theme,
  t,
  onClose,
  onApplied,
}: {
  pluginId: string;
  pluginName: string;
  source: string;
  theme: Theme;
  t: TFunction;
  onClose: () => void;
  onApplied: () => void;
}) {
  const [phase, setPhase] = useState<"previewing" | "reviewing" | "applying" | "error">("previewing");
  const [preview, setPreview] = useState<UpdatePreview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [ack, setAck] = useState(false);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setPhase("previewing");
      setError(null);
      try {
        const data = await eveyFetch<UpdatePreview>({
          url: `/api/v1/admin/plugins/${pluginId}/preview-update`,
          method: "POST",
          data: { source },
        });
        if (!cancelled) {
          setPreview(data);
          setPhase("reviewing");
        }
      } catch (err) {
        if (!cancelled) {
          setError(apiErrorMessage(err as Error, t, t("admin.plugin.updateDialog.previewError", "Échec de l'aperçu")));
          setPhase("error");
        }
      }
    })();
    return () => { cancelled = true; };
  }, [pluginId, source, t]);

  async function handleApply() {
    if (!preview?.sourceHash) return;
    setPhase("applying");
    setError(null);
    try {
      await eveyFetch({
        url: `/api/v1/admin/plugins/${pluginId}/apply-update`,
        method: "POST",
        data: { source, acknowledged_hash: preview.sourceHash },
      });
      onApplied();
    } catch (err) {
      const msg = (err as Error)?.message ?? "";
      // The candidate changed between preview and apply (409/Aborted): the
      // acknowledged hash no longer matches, so the admin must re-preview
      // rather than see a generic failure (mirror web handleApply).
      if (/409|aborted|hash|changed/i.test(msg)) {
        setError(t("admin.plugin.updateDialog.hashMismatch", "The plugin changed since preview - re-open to review again."));
      } else {
        setError(apiErrorMessage(err as Error, t, t("admin.plugin.updateDialog.applyError", "Échec de la mise à jour")));
      }
      setPhase("reviewing");
    }
  }

  const applyDisabled =
    phase === "previewing" ||
    phase === "applying" ||
    (phase === "error" && !preview) ||
    (!!preview?.requiresAck && !ack);

  return (
    <Modal visible transparent animationType="fade" onRequestClose={onClose}>
      <Pressable style={{ flex: 1, backgroundColor: "rgba(0,0,0,0.5)" }} onPress={onClose} />
      <View
        style={{
          position: "absolute",
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: theme.colors.surface,
          borderTopLeftRadius: theme.radii.lg,
          borderTopRightRadius: theme.radii.lg,
          padding: theme.spacing.md,
          paddingBottom: theme.spacing.xl,
          maxHeight: "80%",
        }}
      >
        <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "700", marginBottom: 8 }}>
          {t("admin.plugin.updateDialog.title", { name: pluginName || pluginId, defaultValue: "Mettre à jour {{name}}" })}
        </Text>

        {phase === "previewing" ? (
          <View style={{ alignItems: "center", paddingVertical: 24 }}>
            <ActivityIndicator color={theme.colors.accent} size="large" />
            <Text style={{ color: theme.colors.textMuted, marginTop: 8 }}>
              {t("admin.plugin.updateDialog.analyzing", "Analyse de la mise à jour...")}
            </Text>
          </View>
        ) : null}

        {preview && phase !== "previewing" ? (
          <ScrollView style={{ maxHeight: 320 }}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: 8 }}>
              v{preview.currentVersion ?? "?"} {"->"} v{preview.candidateVersion ?? "?"}
            </Text>
            {preview.requiresAck ? (
              <View style={[styles.noticeBox, { borderColor: theme.colors.error }]}>
                <Text style={{ color: theme.colors.error, fontWeight: "700", fontSize: 12 }}>
                  {t("admin.plugin.updateDialog.bannerRequiresAck", "Cette mise à jour élargit les permissions du plugin")}
                </Text>
              </View>
            ) : (
              <View style={[styles.noticeBox, { borderColor: theme.colors.success }]}>
                <Text style={{ color: theme.colors.success, fontWeight: "700", fontSize: 12 }}>
                  {t("admin.plugin.updateDialog.bannerSafe", "Aucun changement de permission détecté")}
                </Text>
              </View>
            )}
            <DiffRows theme={theme} label={t("admin.plugins.permissions", "Permissions")} added={preview.permissionsAdded} removed={preview.permissionsRemoved} />
            <DiffRows theme={theme} label={t("admin.plugins.capabilities", "Capacités")} added={preview.capabilitiesAdded} removed={preview.capabilitiesRemoved} />
            <DiffRows theme={theme} label={t("admin.plugin.updateDialog.tabSlots", "Slots")} added={preview.slotsAdded} removed={preview.slotsRemoved} />
            <DiffRows theme={theme} label={t("admin.plugin.updateDialog.tabExternalFetch", "Accès réseau externe")} added={preview.externalFetchAdded} removed={preview.externalFetchRemoved} />

            {/* New security findings introduced by the update. */}
            <View style={{ marginTop: 10 }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "700", textTransform: "uppercase" }}>
                {t("admin.plugin.updateDialog.tabFindings", "Alertes de sécurité")}
              </Text>
              {(preview.newFindings?.length ?? 0) === 0 ? (
                <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>{t("admin.plugin.updateDialog.noFindings", "Aucune nouvelle alerte")}</Text>
              ) : (
                preview.newFindings!.map((f, i) => (
                  <View key={i} style={{ marginTop: 4 }}>
                    <View style={{ flexDirection: "row", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
                      <Text style={{ color: f.severity === "critical" ? theme.colors.error : theme.colors.warning, fontSize: 11, fontWeight: "700" }}>{(f.severity ?? "info").toUpperCase()}</Text>
                      <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{f.category}</Text>
                    </View>
                    {f.description ? <Text style={{ color: theme.colors.text, fontSize: 11 }}>{f.description}</Text> : null}
                    {f.file ? <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>{f.file}:{f.line ?? 0}</Text> : null}
                  </View>
                ))
              )}
            </View>

            {/* Signature status of the candidate. */}
            <View style={{ marginTop: 10 }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "700", textTransform: "uppercase" }}>
                {t("admin.plugin.updateDialog.tabSignature", "Signature")}
              </Text>
              <Text style={{ fontSize: 12, marginTop: 2, fontWeight: "600", color: (preview.signatureStatus === "signed" || preview.signatureStatus === "SIGNED") ? theme.colors.success : (preview.signatureStatus === "invalid" || preview.signatureStatus === "INVALID") ? theme.colors.error : theme.colors.warning }}>
                {(preview.signatureStatus === "signed" || preview.signatureStatus === "SIGNED")
                  ? t("admin.plugins.security.signed", "signed")
                  : (preview.signatureStatus === "invalid" || preview.signatureStatus === "INVALID")
                    ? t("admin.plugin.updateDialog.signatureInvalid", "invalid signature")
                    : t("admin.plugins.security.unsigned", "unsigned")}
              </Text>
            </View>
            {preview.requiresAck ? (
              <Pressable onPress={() => setAck((v) => !v)} style={{ flexDirection: "row", alignItems: "center", gap: 8, marginTop: 12 }}>
                <Ionicons name={ack ? "checkbox" : "square-outline"} size={20} color={theme.colors.accent} />
                <Text style={{ color: theme.colors.text, fontSize: 12, flex: 1 }}>
                  {t("admin.plugin.updateDialog.ackLabel", "J'ai vérifié et j'accepte ces changements de permissions")}
                </Text>
              </Pressable>
            ) : null}
            {error ? <Text style={{ color: theme.colors.error, fontSize: 12, marginTop: 8 }}>{error}</Text> : null}
          </ScrollView>
        ) : null}

        {phase === "error" && !preview ? (
          <Text style={{ color: theme.colors.error, fontSize: 13 }}>{error}</Text>
        ) : null}

        <View style={{ flexDirection: "row", gap: theme.spacing.sm, marginTop: theme.spacing.sm }}>
          <Pressable
            onPress={onClose}
            style={{ flex: 1, alignItems: "center", paddingVertical: theme.spacing.sm + 2, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.border }}
          >
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("common.cancel")}</Text>
          </Pressable>
          <Pressable
            onPress={handleApply}
            disabled={applyDisabled}
            style={{
              flex: 1,
              alignItems: "center",
              paddingVertical: theme.spacing.sm + 2,
              borderRadius: theme.radii.md,
              backgroundColor: theme.colors.accent,
              opacity: applyDisabled ? 0.5 : 1,
            }}
          >
            {phase === "applying" ? (
              <ActivityIndicator color={theme.colors.onAccent} size="small" />
            ) : (
              <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("admin.plugin.updateDialog.apply", "Appliquer la mise à jour")}</Text>
            )}
          </Pressable>
        </View>
      </View>
    </Modal>
  );
}

// --- Plugin card ----------------------------------------------------------

function PluginRow({
  plugin,
  theme,
  t,
  toggling,
  onToggle,
  onUninstall,
}: {
  plugin: Plugin;
  theme: Theme;
  t: TFunction;
  toggling: boolean;
  onToggle: () => void;
  onUninstall: () => void;
}) {
  const qc = useQueryClient();
  const [showConfig, setShowConfig] = useState(false);
  const [showLogs, setShowLogs] = useState(false);
  const [showReport, setShowReport] = useState(false);
  const [showUpdateForm, setShowUpdateForm] = useState(false);
  const [updateSourceInput, setUpdateSourceInput] = useState("");
  const [updateSource, setUpdateSource] = useState<string | null>(null);

  const stateKey = STATE_LABEL_KEYS[plugin.state ?? ""] ?? "admin.plugin.state.unknown";
  const stateFallback = STATE_LABEL_FALLBACKS[plugin.state ?? ""] ?? "Inconnu";
  const isSandboxed = plugin.language !== "css" && !plugin.permissions?.includes("subprocess");
  const hasWarnings = (plugin.analysisReport?.issues?.length ?? 0) > 0 || (plugin.analysisReport?.urls?.length ?? 0) > 0;

  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
      <View style={{ flexDirection: "row", alignItems: "flex-start", gap: 12 }}>
        <View style={{ flex: 1, gap: 2, minWidth: 0 }}>
          <View style={{ flexDirection: "row", alignItems: "center", gap: 6, flexWrap: "wrap" }}>
            <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600" }}>{plugin.name}</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>v{plugin.version}</Text>
            {plugin.sourceHash ? (
              <Text style={{ color: theme.colors.textMuted, fontSize: 10, fontFamily: "monospace" }}>
                {plugin.sourceHash.slice(0, 8)}
              </Text>
            ) : null}
          </View>
          {plugin.description ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={2}>{plugin.description}</Text>
          ) : null}
          {plugin.author ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>by {plugin.author}</Text>
          ) : null}
        </View>
        <Switch
          value={plugin.enabled}
          onValueChange={onToggle}
          disabled={toggling}
          trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
        />
      </View>

      <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 6, marginTop: 8 }}>
        <StatusBadge label={t(stateKey, stateFallback)} color={stateColor(theme, plugin.state)} />
        <StatusBadge
          label={plugin.enabled ? t("admin.plugin.enabled", "Activé") : t("admin.plugin.disabled", "Désactivé")}
          color={plugin.enabled ? theme.colors.success : theme.colors.textMuted}
        />
        {plugin.permissions?.includes("subprocess") ? (
          <StatusBadge label={t("admin.plugin.subprocess", "Sous-processus")} color={theme.colors.warning} />
        ) : null}
        {isSandboxed ? (
          <StatusBadge label={t("admin.plugin.sandboxed", "Isolé (sandbox)")} color={theme.colors.success} />
        ) : null}
      </View>

      {plugin.state === "PLUGIN_STATE_ERROR" && plugin.errorMessage ? (
        <View style={[styles.noticeBox, { borderColor: theme.colors.error, marginTop: 8 }]}>
          <Text style={{ color: theme.colors.error, fontWeight: "700", fontSize: 11, marginBottom: 2 }}>
            {t("admin.plugin.errorDetails", "Détails de l'erreur")}
          </Text>
          <Text style={{ color: theme.colors.text, fontSize: 11 }}>{plugin.errorMessage}</Text>
        </View>
      ) : null}

      <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: 10 }}>
        <Pressable onPress={() => setShowConfig((v) => !v)} style={styles.chipBtn}>
          <Text style={{ color: theme.colors.accent, fontSize: 12, fontWeight: "600" }}>
            {showConfig ? t("admin.plugin.hideConfig", "Masquer la configuration") : t("admin.plugin.config", "Configuration")}
          </Text>
        </Pressable>
        <Pressable onPress={() => setShowLogs((v) => !v)} style={styles.chipBtn}>
          <Text style={{ color: theme.colors.accent, fontSize: 12, fontWeight: "600" }}>
            {showLogs ? t("admin.plugin.hideLogs", "Masquer les journaux") : t("admin.plugin.logs", "Journaux")}
          </Text>
        </Pressable>
        <Pressable onPress={() => setShowUpdateForm((v) => !v)} style={styles.chipBtn}>
          <Text style={{ color: theme.colors.accent, fontSize: 12, fontWeight: "600" }}>
            {t("admin.plugin.update", "Mettre à jour depuis une source...")}
          </Text>
        </Pressable>
        <Pressable onPress={onUninstall} style={styles.chipBtn}>
          <Text style={{ color: theme.colors.error, fontSize: 12, fontWeight: "600" }}>
            {t("admin.plugin.uninstall")}
          </Text>
        </Pressable>
      </View>

      {showUpdateForm ? (
        <View style={{ borderTopWidth: 1, borderTopColor: theme.colors.border, marginTop: 10, paddingTop: 10, gap: 8 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "600" }}>
            {t("admin.plugin.updateSourceLabel", "Fichier ZIP source (chemin ou URL)")}
          </Text>
          <View style={{ flexDirection: "row", gap: 8 }}>
            <TextInput
              value={updateSourceInput}
              onChangeText={setUpdateSourceInput}
              placeholder="/path/to/plugin-v2.zip"
              placeholderTextColor={theme.colors.textMuted}
              autoCapitalize="none"
              autoCorrect={false}
              style={[inputStyle(theme), { flex: 1 }]}
            />
            <Pressable
              onPress={() => {
                if (updateSourceInput.trim()) {
                  setUpdateSource(updateSourceInput.trim());
                  setShowUpdateForm(false);
                }
              }}
              disabled={!updateSourceInput.trim()}
              style={[styles.smallBtn, { backgroundColor: theme.colors.accent, opacity: updateSourceInput.trim() ? 1 : 0.5 }]}
            >
              <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 13 }}>
                {t("admin.plugin.previewUpdate", "Aperçu")}
              </Text>
            </Pressable>
          </View>
        </View>
      ) : null}

      {updateSource ? (
        <PluginUpdateModal
          pluginId={plugin.id}
          pluginName={plugin.name}
          source={updateSource}
          theme={theme}
          t={t}
          onClose={() => { setUpdateSource(null); setUpdateSourceInput(""); }}
          onApplied={() => {
            setUpdateSource(null);
            setUpdateSourceInput("");
            toast.success(t("admin.plugin.updateApplied", "Plugin mis à jour"));
            qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() });
          }}
        />
      ) : null}

      {hasWarnings ? (
        <View style={{ borderTopWidth: 1, borderTopColor: theme.colors.border, marginTop: 10, paddingTop: 10 }}>
          <Pressable onPress={() => setShowReport((v) => !v)} style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
            <Text style={{ color: theme.colors.warning, fontSize: 12, fontWeight: "700" }}>
              {t("admin.plugin.analysisReport.title", "Avis de sécurité")}
            </Text>
            <StatusBadge
              label={String((plugin.analysisReport?.issues?.length ?? 0) + (plugin.analysisReport?.urls?.length ?? 0))}
              color={theme.colors.warning}
            />
          </Pressable>
          {showReport ? (
            <View style={{ marginTop: 8, gap: 6 }}>
              {(plugin.analysisReport?.issues ?? []).map((issue, i) => (
                <Text key={`iss-${i}`} style={{ color: theme.colors.textMuted, fontSize: 11 }}>{issue}</Text>
              ))}
              {(plugin.analysisReport?.urls ?? []).map((u, i) => (
                <Text key={`url-${i}`} style={{ color: theme.colors.textMuted, fontSize: 11, fontFamily: "monospace" }}>
                  {u.url}{u.file ? ` - ${u.file}:${u.line ?? 0}` : ""}
                </Text>
              ))}
            </View>
          ) : null}
        </View>
      ) : null}

      {showConfig ? (
        <View style={{ borderTopWidth: 1, borderTopColor: theme.colors.border, marginTop: 10, paddingTop: 10 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "700", marginBottom: 8 }}>
            {t("admin.plugin.configuration", "Configuration")}
          </Text>
          <PluginConfigSection pluginId={plugin.id} theme={theme} t={t} />
        </View>
      ) : null}

      {showLogs ? (
        <View style={{ borderTopWidth: 1, borderTopColor: theme.colors.border, marginTop: 10, paddingTop: 10 }}>
          <PluginLogsSection pluginId={plugin.id} theme={theme} t={t} />
        </View>
      ) : null}
    </View>
  );
}

export default function AdminPluginsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data, isLoading, isError, refetch } = useAdminPluginServiceListPlugins();
  const plugins: Plugin[] = (data?.plugins ?? []) as Plugin[];

  // Runtimes the host detected for plugin execution (mirror web useAvailableRuntimes).
  const { data: runtimesData } = useQuery({
    queryKey: ["/api/v1/admin/plugin-runtimes/available"],
    queryFn: () => eveyFetch<{ runtimes: DetectedRuntime[] }>({ url: "/api/v1/admin/plugin-runtimes/available", method: "GET" }),
    staleTime: 60_000,
  });
  const runtimes = runtimesData?.runtimes ?? [];

  const enablePlugin = useAdminPluginServiceEnablePlugin({
    mutation: {
      onSuccess: () => { toast.success(t("admin.plugins.pluginEnabled")); qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() }); },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugins.toggleFailed", "Failed to enable plugin"))),
    },
  });
  const disablePlugin = useAdminPluginServiceDisablePlugin({
    mutation: {
      onSuccess: () => { toast.success(t("admin.plugins.pluginDisabled")); qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() }); },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugins.toggleFailed", "Failed to disable plugin"))),
    },
  });
  const uninstallPlugin = useAdminPluginServiceUninstallPlugin({
    mutation: {
      onSuccess: () => { toast.success(t("admin.plugins.pluginUninstalled")); qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() }); },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugins.uninstallFailed", "Failed to uninstall plugin"))),
    },
  });

  const [installOpen, setInstallOpen] = useState(false);
  const [installSource, setInstallSource] = useState("");
  const [escalationEntries, setEscalationEntries] = useState<string[]>([]);
  const [blockedIssues, setBlockedIssues] = useState<string[]>([]);
  // Install-from-URL previews the source before installing: the static
  // analysis WARNINGs (overreach, extracted URLs) used to be visible only once
  // the plugin was already running.
  const [installPreview, setInstallPreview] = useState<InstallPreview | null>(null);
  const [previewing, setPreviewing] = useState(false);
  // Install-from-file (mirror web file mode). When set, the confirm/escalation
  // buttons install via multipart upload instead of the URL/source JSON client.
  const [installFileUri, setInstallFileUri] = useState<string | null>(null);
  const [fileBusy, setFileBusy] = useState(false);
  const accessToken = useAuthStore((st) => st.accessToken);

  const installPlugin = useAdminPluginServiceInstallPlugin({
    mutation: {
      // The server downloads and verifies the archive inside the request:
      // opt out of the fetcher's default timeout.
      mutationFn: ({ data }) => adminPluginServiceInstallPlugin(data, NO_TIMEOUT),
      onSuccess: () => {
        toast.success(t("admin.plugins.pluginInstalled", "Plugin installed"));
        qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() });
        setInstallOpen(false);
        setInstallSource("");
        setEscalationEntries([]);
        setBlockedIssues([]);
        setInstallPreview(null);
      },
      onError: (err: Error) => {
        const msg = err.message || "";
        // gRPC FAILED_PRECONDITION with "new entries requested: ..." means the
        // install would widen permissions/capabilities beyond what was already
        // approved - surface the diff instead of a blind toast so the admin
        // reviews before confirming (mirrors the web install-from-URL flow).
        if (msg.includes("new entries requested:")) {
          setEscalationEntries(parseEscalationEntries(msg));
        } else if (msg.includes("plugin blocked:")) {
          // The server refused on its static analysis - same verdict web gets.
          // Listing the findings is the difference between "install failed" and
          // an admin who can see what was found.
          setBlockedIssues(parseBlockedIssues(msg));
        } else {
          toast.error(msg || t("admin.plugins.installFailed", "Install failed"));
        }
      },
    },
  });

  const handleToggle = useCallback((plugin: Plugin) => {
    if (plugin.enabled) {
      disablePlugin.mutate({ id: plugin.id });
    } else {
      enablePlugin.mutate({ id: plugin.id });
    }
  }, [enablePlugin, disablePlugin]);

  const handleUninstall = useCallback((plugin: Plugin) => {
    Alert.alert(t("admin.plugin.uninstall"), `${t("admin.plugin.uninstall")} "${plugin.name}"?`, [
      { text: t("common.cancel"), style: "cancel" },
      { text: t("admin.plugin.uninstall"), style: "destructive", onPress: () => uninstallPlugin.mutate({ id: plugin.id }) },
    ]);
  }, [uninstallPlugin]);

  const closeInstallModal = useCallback(() => {
    setInstallOpen(false);
    setInstallSource("");
    setEscalationEntries([]);
    setBlockedIssues([]);
    setInstallPreview(null);
    setInstallFileUri(null);
  }, []);

  // Run a multipart install of the picked archive, branching the same way the
  // URL install does: 428 -> escalation diff, 403 -> blocked findings.
  const runFileInstall = useCallback(async (accept: boolean) => {
    if (!installFileUri) return;
    setFileBusy(true);
    setEscalationEntries([]);
    setBlockedIssues([]);
    try {
      const res = await installPluginFile(installFileUri, { accept }, accessToken);
      if (res.ok) {
        toast.success(t("admin.plugins.pluginInstalled", "Plugin installed"));
        qc.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() });
        closeInstallModal();
      } else if (res.escalation && res.escalation.length > 0) {
        setEscalationEntries(res.escalation);
      } else if (res.blocked && res.blocked.length > 0) {
        setBlockedIssues(res.blocked);
      } else {
        toast.error(res.error || t("admin.plugins.installFailed", "Install failed"));
      }
    } finally {
      setFileBusy(false);
    }
  }, [installFileUri, accessToken, t, qc, closeInstallModal]);

  const handleConfirmEscalation = useCallback(() => {
    if (installFileUri) { void runFileInstall(true); return; }
    installPlugin.mutate({ data: { source: installSource.trim(), acceptPermissionChanges: true } });
  }, [installFileUri, runFileInstall, installPlugin, installSource]);

  // Pick a .zip off the device, preview it, then the same confirm flow installs it.
  const handlePickFile = useCallback(async () => {
    const picked = await DocumentPicker.getDocumentAsync({
      type: ["application/zip", "application/octet-stream"],
      copyToCacheDirectory: true,
    });
    if (picked.canceled || !picked.assets?.[0]?.uri) return;
    const uri = picked.assets[0].uri;
    setPreviewing(true);
    setInstallFileUri(uri);
    try {
      const preview = await previewPluginFile(uri, accessToken);
      setInstallPreview(preview);
    } catch (err) {
      setInstallFileUri(null);
      toast.error(apiErrorMessage(err as Error, t, t("admin.plugins.previewFailed", "Preview failed")));
    } finally {
      setPreviewing(false);
    }
  }, [accessToken, t]);

  const handlePreviewInstall = useCallback(async () => {
    const source = installSource.trim();
    if (!source) return;
    setPreviewing(true);
    try {
      const data = await eveyFetch<InstallPreview>({
        url: "/api/v1/admin/plugins/preview",
        method: "POST",
        data: { source },
      });
      setInstallPreview(data);
    } catch (err) {
      toast.error(apiErrorMessage(err as Error, t, t("admin.plugins.previewFailed", "Preview failed")));
    } finally {
      setPreviewing(false);
    }
  }, [installSource, t]);

  const handleInstallPreviewed = useCallback(() => {
    if (installFileUri) { void runFileInstall(false); return; }
    installPlugin.mutate({ data: { source: installSource.trim() } });
  }, [installFileUri, runFileInstall, installPlugin, installSource]);

  const renderPlugin = useCallback(({ item }: { item: Plugin }) => (
    <PluginRow
      plugin={item}
      theme={theme}
      t={t}
      toggling={
        (enablePlugin.isPending && enablePlugin.variables?.id === item.id) ||
        (disablePlugin.isPending && disablePlugin.variables?.id === item.id)
      }
      onToggle={() => handleToggle(item)}
      onUninstall={() => handleUninstall(item)}
    />
  ), [theme, t, enablePlugin.isPending, enablePlugin.variables, disablePlugin.isPending, disablePlugin.variables, handleToggle, handleUninstall]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.plugins.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
        headerRight: () => (
          <Pressable
            onPress={() => setInstallOpen(true)}
            style={{ paddingHorizontal: 12, paddingVertical: 6 }}
            accessible
            accessibilityRole="button"
            accessibilityLabel={t("admin.plugins.installFromUrl", "Install from URL")}
          >
            <Ionicons name="add" size={24} color={theme.colors.accent} />
          </Pressable>
        ),
      }} />
      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : isError ? (
        <LoadFailed onRetry={() => void refetch()} />
      ) : (
        <FlatList
          data={plugins}
          keyExtractor={(p) => p.id}
          renderItem={renderPlugin}
          contentContainerStyle={{ padding: 16, gap: 8 }}
          refreshing={false}
          onRefresh={refetch}
          ListHeaderComponent={
            <View style={{ gap: 8, marginBottom: 8 }}>
              {runtimes.length > 0 && (
                <View style={[styles.runtimesCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "600", textTransform: "uppercase" }}>
                    {t("admin.plugins.availableRuntimes", "Detected runtimes")}
                  </Text>
                  <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 6, marginTop: 6 }}>
                    {runtimes.map((r) => (
                      <View key={`${r.language}-${r.version}`} style={{ paddingHorizontal: 8, paddingVertical: 3, borderRadius: theme.radii.sm, backgroundColor: theme.colors.accent + "22" }}>
                        <Text style={{ color: theme.colors.accent, fontSize: 11, fontWeight: "600" }}>{r.language} {r.version}</Text>
                      </View>
                    ))}
                  </View>
                </View>
              )}
              <PluginResourceLimitsCard />
            </View>
          }
          ListFooterComponent={
            <View style={{ marginTop: 16 }}>
              <PluginSecurityDashboard />
            </View>
          }
          ListEmptyComponent={
            <View style={{ alignItems: "center", paddingTop: 60 }}>
              <Ionicons name="extension-puzzle-outline" size={48} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.textMuted, marginTop: 12 }}>{t("admin.plugins.noPlugins", "No plugins installed")}</Text>
            </View>
          }
        />
      )}

      <Modal visible={installOpen} transparent animationType="fade" onRequestClose={closeInstallModal}>
        <Pressable style={{ flex: 1, backgroundColor: "rgba(0,0,0,0.5)" }} onPress={closeInstallModal} />
        <View style={{
          position: "absolute",
          left: 0,
          right: 0,
          bottom: 0,
          backgroundColor: theme.colors.surface,
          borderTopLeftRadius: theme.radii.lg,
          borderTopRightRadius: theme.radii.lg,
          padding: theme.spacing.md,
          paddingBottom: theme.spacing.xl,
          gap: theme.spacing.sm,
        }}>
          {blockedIssues.length > 0 ? (
            <View style={{ flexDirection: "row", alignItems: "flex-start", gap: 8 }}>
              <Ionicons name="shield-outline" size={20} color={theme.colors.error} />
              <View style={{ flex: 1, gap: 4 }}>
                <Text style={{ color: theme.colors.error, fontWeight: "700", fontSize: 14 }}>
                  {t("admin.plugins.blockedByAnalysis")}
                </Text>
                {blockedIssues.map((issue, i) => (
                  <Text key={i} style={{ color: theme.colors.textMuted, fontSize: 12, fontFamily: "monospace" }}>{issue}</Text>
                ))}
              </View>
            </View>
          ) : null}
          {escalationEntries.length > 0 ? (
            <>
              <View style={{ flexDirection: "row", alignItems: "flex-start", gap: 8 }}>
                <Ionicons name="alert-circle-outline" size={20} color={theme.colors.warning} />
                <View style={{ flex: 1 }}>
                  <Text style={{ color: theme.colors.warning, fontWeight: "700", fontSize: 14 }}>
                    {t("admin.plugins.permissionEscalation.title", "Élévation de permissions")}
                  </Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
                    {t("admin.plugins.permissionEscalation.description", "Ce plugin demande plus de permissions que prévu.")}
                  </Text>
                </View>
              </View>
              <View style={{ gap: 4 }}>
                {escalationEntries.map((e, i) => (
                  <Text key={i} style={{ color: theme.colors.warning, fontSize: 12, fontFamily: "monospace" }}>{e}</Text>
                ))}
              </View>
              <View style={{ flexDirection: "row", gap: theme.spacing.sm, marginTop: theme.spacing.sm }}>
                <Pressable
                  onPress={closeInstallModal}
                  style={{ flex: 1, alignItems: "center", paddingVertical: theme.spacing.sm + 2, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.border }}
                >
                  <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("common.cancel")}</Text>
                </Pressable>
                <Pressable
                  onPress={handleConfirmEscalation}
                  disabled={installPlugin.isPending || fileBusy}
                  style={{
                    flex: 1,
                    alignItems: "center",
                    paddingVertical: theme.spacing.sm + 2,
                    borderRadius: theme.radii.md,
                    backgroundColor: theme.colors.warning,
                    opacity: installPlugin.isPending || fileBusy ? 0.5 : 1,
                  }}
                >
                  {installPlugin.isPending || fileBusy ? (
                    <ActivityIndicator color={theme.colors.onAccent} size="small" />
                  ) : (
                    <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>
                      {t("admin.plugins.permissionEscalation.confirm", "Installer quand même")}
                    </Text>
                  )}
                </Pressable>
              </View>
            </>
          ) : installPreview ? (
            <InstallPreviewPanel
              preview={installPreview}
              theme={theme}
              t={t}
              installing={installPlugin.isPending || fileBusy}
              onCancel={closeInstallModal}
              onConfirm={handleInstallPreviewed}
            />
          ) : (
            <>
              <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "700" }}>
                {t("admin.plugins.installFromUrl", "Install from URL")}
              </Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>
                {t("admin.plugins.installFromUrlHint", "Paste a plugin .zip URL - optionally append #sha256:<hash> to verify the download.")}
              </Text>
              <TextInput
                value={installSource}
                onChangeText={setInstallSource}
                placeholder="https://example.com/plugin.zip"
                placeholderTextColor={theme.colors.textMuted}
                autoCapitalize="none"
                autoCorrect={false}
                keyboardType="url"
                style={{
                  backgroundColor: theme.colors.bg,
                  color: theme.colors.text,
                  borderRadius: theme.radii.md,
                  padding: theme.spacing.sm + 2,
                  borderWidth: 1,
                  borderColor: theme.colors.border,
                  fontSize: 14,
                }}
              />
              <View style={{ flexDirection: "row", gap: theme.spacing.sm, marginTop: theme.spacing.sm }}>
                <Pressable
                  onPress={closeInstallModal}
                  style={{ flex: 1, alignItems: "center", paddingVertical: theme.spacing.sm + 2, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.border }}
                >
                  <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("common.cancel")}</Text>
                </Pressable>
                <Pressable
                  onPress={() => void handlePreviewInstall()}
                  disabled={!installSource.trim() || previewing}
                  style={{
                    flex: 1,
                    alignItems: "center",
                    paddingVertical: theme.spacing.sm + 2,
                    borderRadius: theme.radii.md,
                    backgroundColor: theme.colors.accent,
                    opacity: !installSource.trim() || previewing ? 0.5 : 1,
                  }}
                >
                  {previewing ? (
                    <ActivityIndicator color={theme.colors.onAccent} size="small" />
                  ) : (
                    <Text style={{ color: theme.colors.onAccent, fontWeight: "600" }}>{t("admin.plugins.previewInstall", "Preview & Install")}</Text>
                  )}
                </Pressable>
              </View>

              {/* Install from a picked .zip (mirror web file mode). */}
              <Pressable
                onPress={() => void handlePickFile()}
                disabled={previewing}
                style={{ flexDirection: "row", alignItems: "center", justifyContent: "center", gap: 8, marginTop: theme.spacing.sm, paddingVertical: theme.spacing.sm + 2, borderRadius: theme.radii.md, borderWidth: 1, borderColor: theme.colors.border, opacity: previewing ? 0.5 : 1 }}
              >
                <Ionicons name="folder-open-outline" size={18} color={theme.colors.text} />
                <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("admin.plugins.installFromFile", "Install from file (.zip)")}</Text>
              </Pressable>
            </>
          )}
        </View>
      </Modal>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { padding: 14 },
  runtimesCard: { padding: 14 },
  badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4, borderWidth: 1 },
  chipBtn: { paddingHorizontal: 10, paddingVertical: 6, borderRadius: 6 },
  smallBtn: { paddingHorizontal: 14, paddingVertical: 8, borderRadius: 6, alignItems: "center", justifyContent: "center" },
  noticeBox: { borderWidth: 1, borderRadius: 6, padding: 8 },
  chip: { paddingHorizontal: 10, paddingVertical: 6, borderRadius: 14, borderWidth: 1 },
});
