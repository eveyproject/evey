import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Switch } from "@/components/ui/Switch";
import { Spinner } from "@/components/ui/Spinner";
import { SettingRow } from "@/components/ui/SettingRow";
import { SettingsGroup } from "@/components/ui/SettingsGroup";

// The /api/v1/admin/server-settings resource: one object of typed settings,
// each with its value, its default, whether an override is stored, whether
// the server reads it only at boot, and whether the demo greys it.
export interface SettingValue<T> {
  value: T;
  default: T;
  source: "override" | "default";
  requiresRestart?: boolean;
  locked?: boolean;
}

export type ServerSettings = Partial<Record<string, SettingValue<unknown>>>;

export const SERVER_SETTINGS_KEY = ["/api/v1/admin/server-settings"];
const SERVER_SETTINGS_URL = "/api/v1/admin/server-settings";

export function useServerSettings() {
  return useQuery<ServerSettings>({
    queryKey: SERVER_SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: SERVER_SETTINGS_URL, method: "GET" }),
  });
}

type FieldKind = "text" | "number" | "toggle" | "select";

interface FieldSpec {
  kind: FieldKind;
  labelKey: string;
  /** Plain-language explanation behind the "i". Left out on purpose for introAutoSkipEnabled. */
  infoKey?: string;
  hintKey?: string;
  testId: string;
  /** Shown as the placeholder / default when the server does not report one. */
  fallbackDefault: string | number | boolean;
  min?: number;
  options?: string[];
}

// Every field of the server-settings resource the server configuration and
// transcoding pages edit through these groups, with its copy. The sections
// pick fields from here by key; a field belongs to exactly one group.
export const SERVER_SETTING_FIELDS = {
  serverName: { kind: "text", labelKey: "admin.serverSettings.serverName", infoKey: "admin.settingsHelp.serverName", testId: "server-name-input", fallbackDefault: "Evey" },
  logLevel: { kind: "select", labelKey: "admin.serverSettings.logLevel", infoKey: "admin.settingsHelp.logLevel", testId: "log-level-select", fallbackDefault: "info", options: ["debug", "info", "warn", "error"] },
  libraryWatcherEnabled: { kind: "toggle", labelKey: "admin.serverSettings.libraryWatcher", infoKey: "admin.settingsHelp.libraryWatcher", testId: "library-watcher-toggle", fallbackDefault: true },
  periodicScanInterval: { kind: "text", labelKey: "admin.serverSettings.periodicScanInterval", infoKey: "admin.settingsHelp.periodicScanInterval", testId: "scan-interval-input", fallbackDefault: "2m" },
  skipPreviewEnabled: { kind: "toggle", labelKey: "admin.serverSettings.skipPreview", infoKey: "admin.serverSettings.skipPreviewInfo", testId: "skip-preview-toggle", fallbackDefault: false },
  // No infoKey: the owner wants this switch bare.
  introAutoSkipEnabled: { kind: "toggle", labelKey: "admin.serverSettings.introAutoSkip", testId: "cast-intro-auto-skip-toggle", fallbackDefault: false },
  passwordRecoveryEnabled: { kind: "toggle", labelKey: "admin.serverSettings.passwordRecovery", infoKey: "admin.serverSettings.passwordRecoveryHint", testId: "password-recovery-toggle", fallbackDefault: false },
  loginRateLimit: { kind: "number", labelKey: "admin.serverSettings.loginRateLimit", infoKey: "admin.settingsHelp.loginRateLimit", testId: "login-rate-limit-input", fallbackDefault: 10, min: 0 },
  globalRateLimit: { kind: "number", labelKey: "admin.serverSettings.globalRateLimit", infoKey: "admin.settingsHelp.globalRateLimit", testId: "global-rate-limit-input", fallbackDefault: 300, min: 0 },
  auditRetentionDays: { kind: "number", labelKey: "admin.serverSettings.auditRetentionDays", infoKey: "admin.settingsHelp.auditRetentionDays", hintKey: "admin.settingsLayout.nextSweepHint", testId: "audit-retention-input", fallbackDefault: 0, min: 0 },
  metricsRetentionDays: { kind: "number", labelKey: "admin.serverSettings.metricsRetentionDays", infoKey: "admin.settingsHelp.metricsRetentionDays", hintKey: "admin.settingsLayout.nextSweepHint", testId: "metrics-retention-input", fallbackDefault: 90, min: 0 },
  qoeRetentionDays: { kind: "number", labelKey: "admin.serverSettings.qoeRetentionDays", infoKey: "admin.settingsHelp.qoeRetentionDays", hintKey: "admin.settingsLayout.nextSweepHint", testId: "qoe-retention-input", fallbackDefault: 30, min: 0 },
  playSessionRetentionDays: { kind: "number", labelKey: "admin.serverSettings.playSessionRetentionDays", infoKey: "admin.settingsHelp.playSessionRetentionDays", hintKey: "admin.settingsLayout.nextSweepHint", testId: "session-retention-input", fallbackDefault: 0, min: 0 },
  backupIncludeTelemetry: { kind: "toggle", labelKey: "admin.serverSettings.backupTelemetry", infoKey: "admin.serverSettings.backupTelemetryHint", testId: "backup-telemetry-toggle", fallbackDefault: false },
  transcodeCacheMaxGB: { kind: "number", labelKey: "admin.serverSettings.transcodeCacheMaxGB", infoKey: "admin.settingsHelp.transcodeCacheMaxGB", testId: "cache-budget-input", fallbackDefault: 10, min: 1 },
} satisfies Record<string, FieldSpec>;

export type ServerSettingField = keyof typeof SERVER_SETTING_FIELDS;

// Settings the server reads only at boot. The server reports this itself
// (requiresRestart); the list covers a server too old to say so.
const RESTART_FALLBACK = new Set<ServerSettingField>(["libraryWatcherEnabled", "periodicScanInterval", "transcodeCacheMaxGB"]);

type Draft = string | boolean;

// seedValue is what a control shows for a setting: a toggle shows the
// effective value; a text or number field shows only an override, blank
// meaning "use the default" (pre-filling the effective value would turn every
// save into an override of a value nobody chose).
function seedValue(spec: FieldSpec, s: SettingValue<unknown> | undefined): Draft {
  if (spec.kind === "toggle") return Boolean(s?.value ?? spec.fallbackDefault);
  return s?.source === "override" ? String(s.value) : "";
}

interface ServerSettingsGroupProps {
  title: string;
  description?: string;
  info?: string;
  fields: ServerSettingField[];
  /** data-testid of the group's Save button. */
  saveTestId: string;
  "data-testid"?: string;
}

// ServerSettingsGroup edits a few fields of the server-settings resource with
// its own Save. The server takes a partial object (an absent field is left
// alone, an explicit null resets it), so each group sends only the fields the
// admin changed in it. A refetch after another group's Save re-seeds only the
// fields not being edited here: splitting the old single form across sections
// must never wipe an unsaved edit.
export function ServerSettingsGroup({ title, description, info, fields, saveTestId, "data-testid": testId }: ServerSettingsGroupProps) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading } = useServerSettings();
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const dirty = useRef<Set<string>>(new Set());
  const [dirtyCount, setDirtyCount] = useState(0);
  // The field list is a literal at each call site (a new array every render):
  // read through a ref so the seed follows the data only.
  const fieldsRef = useRef(fields);
  fieldsRef.current = fields;

  useEffect(() => {
    if (!data) return;
    setDrafts((prev) => {
      const next = { ...prev };
      for (const key of fieldsRef.current) {
        if (!dirty.current.has(key)) next[key] = seedValue(SERVER_SETTING_FIELDS[key], data[key]);
      }
      return next;
    });
  }, [data]);

  function edit(key: ServerSettingField, value: Draft) {
    setDrafts((p) => ({ ...p, [key]: value }));
    dirty.current.add(key);
    setDirtyCount(dirty.current.size);
  }

  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({ url: SERVER_SETTINGS_URL, method: "PUT", data: body }),
    onSuccess: () => {
      dirty.current.clear();
      setDirtyCount(0);
      void qc.invalidateQueries({ queryKey: SERVER_SETTINGS_KEY });
      toast.success(t("admin.serverSettings.saved"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const reset = useMutation({
    mutationFn: (field: string) =>
      eveyFetch<ServerSettings>({ url: SERVER_SETTINGS_URL, method: "PUT", data: { [field]: null } }),
    onSuccess: (_r, field) => {
      dirty.current.delete(field);
      setDirtyCount(dirty.current.size);
      void qc.invalidateQueries({ queryKey: SERVER_SETTINGS_KEY });
      toast.success(t("admin.serverSettings.saved"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  function handleSave() {
    const body: Record<string, unknown> = {};
    for (const key of fields) {
      if (!dirty.current.has(key)) continue;
      const spec: FieldSpec = SERVER_SETTING_FIELDS[key];
      const v = drafts[key];
      if (spec.kind === "toggle") body[key] = Boolean(v);
      // An emptied field means "keep what is there": clearing an override
      // is the Reset link's job (an explicit null), never a silent 0.
      else if (v !== "" && v !== undefined) body[key] = spec.kind === "number" ? parseInt(String(v), 10) : String(v);
    }
    if (Object.keys(body).length === 0) {
      dirty.current.clear();
      setDirtyCount(0);
      return;
    }
    save.mutate(body);
  }

  const onOff = (v: unknown) => (v ? t("admin.settingsLayout.on") : t("admin.settingsLayout.off"));

  return (
    <SettingsGroup
      title={title}
      description={description}
      info={info}
      data-testid={testId}
      footer={
        data ? (
          <Button onClick={handleSave} loading={save.isPending} disabled={dirtyCount === 0} data-testid={saveTestId}>
            {t("common.save")}
          </Button>
        ) : undefined
      }
    >
      {isLoading || !data ? (
        <div className="flex justify-center py-4"><Spinner size="md" /></div>
      ) : (
        fields.map((key) => {
          const spec: FieldSpec = SERVER_SETTING_FIELDS[key];
          const s = data[key];
          const id = `server-setting-${key}`;
          const label = t(spec.labelKey);
          const def = s?.default ?? spec.fallbackDefault;
          const locked = s?.locked === true;
          const common = {
            label,
            info: spec.infoKey ? t(spec.infoKey) : undefined,
            hint: spec.hintKey ? t(spec.hintKey) : undefined,
            restart: s?.requiresRestart ?? RESTART_FALLBACK.has(key),
            reset: s
              ? {
                  overridden: s.source === "override",
                  defaultText: spec.kind === "toggle" ? onOff(def) : String(def),
                  onReset: () => reset.mutate(key),
                  disabled: locked || reset.isPending,
                }
              : undefined,
            "data-testid": `setting-row-${key}`,
          };
          const draft = drafts[key] ?? seedValue(spec, s);

          if (spec.kind === "toggle") {
            return (
              <SettingRow key={key} {...common} kind="toggle">
                <Switch
                  checked={Boolean(draft)}
                  onChange={() => edit(key, !draft)}
                  label={label}
                  disabled={locked}
                  data-testid={spec.testId}
                />
              </SettingRow>
            );
          }
          if (spec.kind === "select") {
            return (
              <SettingRow key={key} {...common} htmlFor={id}>
                <Select
                  id={id}
                  data-testid={spec.testId}
                  value={String(draft)}
                  disabled={locked}
                  onChange={(v) => edit(key, v)}
                  options={[
                    { label: `${t("admin.serverConfig.envDefault")} (${String(def)})`, value: "" },
                    ...(spec.options ?? []).map((o) => ({ label: o, value: o })),
                  ]}
                />
              </SettingRow>
            );
          }
          return (
            <SettingRow key={key} {...common} htmlFor={id}>
              <Input
                id={id}
                data-testid={spec.testId}
                className="w-full"
                type={spec.kind === "number" ? "number" : "text"}
                min={spec.min}
                placeholder={String(def)}
                value={String(draft)}
                disabled={locked}
                onChange={(e) => edit(key, e.target.value)}
              />
            </SettingRow>
          );
        })
      )}
    </SettingsGroup>
  );
}
