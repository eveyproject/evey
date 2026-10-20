import { useEffect, useRef, useState, type ReactNode } from "react";
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
import { SegmentedControl } from "@/components/ui/SegmentedControl";

// The /api/v1/admin/server-settings resource: one object of typed settings,
// each with its value, its default, whether an override is stored, whether
// the server reads it only at boot, and whether the demo greys it. This file
// is the one place the web app names it: the query key, the value type and
// the form every admin page edits it through.
export interface SettingValue<T = unknown> {
  value: T;
  default: T;
  source: "override" | "default";
  requiresRestart?: boolean;
  locked?: boolean;
}

export type ServerSettings = Partial<Record<string, SettingValue>>;

export const SERVER_SETTINGS_KEY = ["/api/v1/admin/server-settings"];
export const SERVER_SETTINGS_URL = "/api/v1/admin/server-settings";

export function useServerSettings() {
  return useQuery<ServerSettings>({
    queryKey: SERVER_SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: SERVER_SETTINGS_URL, method: "GET" }),
  });
}

type FieldKind = "text" | "number" | "toggle" | "select" | "segmented";

interface FieldSpec {
  kind: FieldKind;
  labelKey: string;
  /** Plain-language explanation behind the "i". Left out on purpose for introAutoSkipEnabled. */
  infoKey?: string;
  hintKey?: string;
  testId: string;
  /** Shown as the placeholder / default when the server does not report one. */
  fallbackDefault: string | number | boolean;
  /** Smallest whole number accepted (number fields). */
  min?: number;
  /** Values offered by a select or a segmented control ("" = the default / Auto). */
  options?: string[];
  /** Label of the "" option of a segmented control. */
  autoKey?: string;
  /** Placeholder of a blank number field (i18n key, {{value}} = the default). */
  placeholderKey?: string;
  /** What an emptied field means on Save: keep the stored value (default) or clear the override (null). */
  blank?: "keep" | "reset";
  /** 0 means "no override" on the server: show it as the default (""). */
  zeroIsDefault?: boolean;
  /** The text must be a Go duration (time.ParseDuration), greater than zero. */
  duration?: boolean;
}

// The software height ladder the server accepts (settings_handler.go
// validSoftwareHeights); "" is "use the measured value".
const SOFTWARE_HEIGHTS = ["", "480", "720", "1080", "1440", "2160"];
// The AV1 software ceilings it accepts (validSoftwareAV1Heights); "" is Auto.
const SOFTWARE_AV1_HEIGHTS = ["", "480", "720", "1080", "1440"];

// Every field of the server-settings resource the admin pages edit, with its
// copy. A page picks fields from here by key; a field belongs to one group.
export const SERVER_SETTING_FIELDS = {
  serverName: { kind: "text", labelKey: "admin.serverSettings.serverName", infoKey: "admin.settingsHelp.serverName", testId: "server-name-input", fallbackDefault: "Evey" },
  logLevel: { kind: "select", labelKey: "admin.serverSettings.logLevel", infoKey: "admin.settingsHelp.logLevel", testId: "log-level-select", fallbackDefault: "info", options: ["debug", "info", "warn", "error"] },
  libraryWatcherEnabled: { kind: "toggle", labelKey: "admin.serverSettings.libraryWatcher", infoKey: "admin.settingsHelp.libraryWatcher", testId: "library-watcher-toggle", fallbackDefault: true },
  periodicScanInterval: { kind: "text", labelKey: "admin.serverSettings.periodicScanInterval", infoKey: "admin.settingsHelp.periodicScanInterval", testId: "scan-interval-input", fallbackDefault: "2m", duration: true },
  skipPreviewEnabled: { kind: "toggle", labelKey: "admin.serverSettings.skipPreview", infoKey: "admin.serverSettings.skipPreviewInfo", testId: "skip-preview-toggle", fallbackDefault: false },
  // No infoKey: the owner wants this switch bare.
  introAutoSkipEnabled: { kind: "toggle", labelKey: "admin.serverSettings.introAutoSkip", testId: "cast-intro-auto-skip-toggle", fallbackDefault: false },
  passwordRecoveryEnabled: { kind: "toggle", labelKey: "admin.serverSettings.passwordRecovery", infoKey: "admin.serverSettings.passwordRecoveryHint", testId: "password-recovery-toggle", fallbackDefault: false },
  loginRateLimit: { kind: "number", labelKey: "admin.serverSettings.loginRateLimit", infoKey: "admin.settingsHelp.loginRateLimit", testId: "login-rate-limit-input", fallbackDefault: 10, min: 0 },
  globalRateLimit: { kind: "number", labelKey: "admin.serverSettings.globalRateLimit", infoKey: "admin.settingsHelp.globalRateLimit", testId: "global-rate-limit-input", fallbackDefault: 300, min: 0 },
  auditRetentionDays: { kind: "number", labelKey: "admin.serverSettings.auditRetentionDays", infoKey: "admin.settingsHelp.auditRetentionDays", hintKey: "admin.settingsLayout.nextSweepHint", testId: "audit-retention-input", fallbackDefault: 0, min: 0 },
  metricsRetentionDays: { kind: "number", labelKey: "admin.serverSettings.metricsRetentionDays", infoKey: "admin.settingsHelp.metricsRetentionDays", hintKey: "admin.settingsLayout.nextSweepHint", testId: "metrics-retention-input", fallbackDefault: 90, min: 0 },
  qoeRetentionDays: { kind: "number", labelKey: "admin.serverSettings.qoeRetentionDays", infoKey: "admin.settingsHelp.qoeRetentionDays", hintKey: "admin.settingsLayout.nextSweepHint", testId: "qoe-retention-input", fallbackDefault: 30, min: 0 },
  // Its sweep runs every few seconds, not daily.
  playSessionRetentionDays: { kind: "number", labelKey: "admin.serverSettings.playSessionRetentionDays", infoKey: "admin.settingsHelp.playSessionRetentionDays", hintKey: "admin.settingsLayout.liveSweepHint", testId: "session-retention-input", fallbackDefault: 0, min: 0 },
  backupIncludeTelemetry: { kind: "toggle", labelKey: "admin.serverSettings.backupTelemetry", infoKey: "admin.serverSettings.backupTelemetryHint", testId: "backup-telemetry-toggle", fallbackDefault: false },
  transcodeCacheMaxGB: { kind: "number", labelKey: "admin.serverSettings.transcodeCacheMaxGB", infoKey: "admin.settingsHelp.transcodeCacheMaxGB", testId: "cache-budget-input", fallbackDefault: 10, min: 1 },
  maxConcurrentTranscodes: { kind: "number", labelKey: "admin.settingsLayout.backgroundTranscodes", infoKey: "admin.settingsHelp.backgroundTranscodes", testId: "max-concurrent-input", fallbackDefault: 2, min: 1, blank: "reset", placeholderKey: "admin.settingsLayout.autoDefault" },
  transcodeMaxSessions: { kind: "number", labelKey: "admin.settingsLayout.maxSessionsLabel", infoKey: "admin.settingsHelp.transcodeMaxSessions", testId: "max-sessions-input", fallbackDefault: 0, min: 0, blank: "reset", placeholderKey: "admin.settingsLayout.perCorePlaceholder" },
  softwareMaxHeight: { kind: "segmented", labelKey: "admin.settingsLayout.softwareMaxHeightLabel", infoKey: "admin.settingsHelp.softwareMaxHeight", testId: "software-max-height", fallbackDefault: 0, options: SOFTWARE_HEIGHTS, autoKey: "admin.settingsLayout.autoMeasured", blank: "reset", zeroIsDefault: true },
  softwareHDRMaxHeight: { kind: "segmented", labelKey: "admin.settingsLayout.softwareHDRMaxHeightLabel", infoKey: "admin.settingsHelp.softwareHDRMaxHeight", testId: "software-hdr-max-height", fallbackDefault: 0, options: SOFTWARE_HEIGHTS, autoKey: "admin.settingsLayout.autoMeasured", blank: "reset", zeroIsDefault: true },
  softwareAV1MaxHeight: { kind: "segmented", labelKey: "admin.serverSettings.softwareAV1MaxHeight", infoKey: "admin.settingsHelp.softwareAV1MaxHeight", testId: "software-av1-max-height", fallbackDefault: 0, options: SOFTWARE_AV1_HEIGHTS, autoKey: "admin.settingsLayout.autoPlain", blank: "reset", zeroIsDefault: true },
  transcodeSingleVariant: { kind: "toggle", labelKey: "admin.transcoding.singleVariant", infoKey: "admin.settingsHelp.singleVariant", testId: "single-variant-toggle", fallbackDefault: true },
  transcodeHWPerVariant: { kind: "toggle", labelKey: "admin.transcoding.hwPerVariant", infoKey: "admin.settingsHelp.hwPerVariant", testId: "hw-per-variant-toggle", fallbackDefault: false },
} satisfies Record<string, FieldSpec>;

export type ServerSettingField = keyof typeof SERVER_SETTING_FIELDS;

// Settings the server reads only at boot. The server reports this itself
// (requiresRestart); the list covers a server too old to say so.
const RESTART_FALLBACK = new Set<string>([
  "libraryWatcherEnabled", "periodicScanInterval", "transcodeCacheMaxGB", "transcodeMaxSessions",
  "softwareMaxHeight", "softwareHDRMaxHeight", "transcodeSingleVariant", "transcodeHWPerVariant",
]);

// A Go time.Duration as time.ParseDuration reads it: one or more number+unit
// pairs ("90s", "1h30m"), here required to be above zero. The server silently
// ignores anything else.
const GO_DURATION = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/;
function durationOK(v: string): boolean {
  return GO_DURATION.test(v) && /[1-9]/.test(v);
}

type Draft = string | boolean;
type Translate = (key: string, opts?: Record<string, unknown>) => string;

const spec = (key: string): FieldSpec => SERVER_SETTING_FIELDS[key as ServerSettingField];

// seedValue is what a control shows for a setting: a toggle shows the
// effective value; any other field shows only an override, blank meaning
// "use the default" (pre-filling the effective value would turn every save
// into an override of a value nobody chose).
function seedValue(f: FieldSpec, s: SettingValue | undefined): Draft {
  if (f.kind === "toggle") return Boolean(s?.value ?? f.fallbackDefault);
  if (s?.source !== "override") return "";
  if (f.zeroIsDefault && Number(s.value) === 0) return "";
  return String(s.value);
}

// validate returns the message of an invalid draft, or null. The server drops
// an invalid value without an error, so the form must refuse it itself.
function validate(f: FieldSpec, v: Draft | undefined, t: Translate): string | null {
  if (typeof v !== "string" || v.trim() === "") return null;
  if (f.kind === "number") {
    const min = f.min ?? 0;
    if (!/^\d+$/.test(v.trim()) || parseInt(v, 10) < min) return t("admin.settingsLayout.errorNumber", { min });
  }
  if (f.duration && !durationOK(v.trim())) return t("admin.settingsLayout.errorDuration");
  return null;
}

function bodyValue(f: FieldSpec, v: Draft | undefined): unknown {
  if (f.kind === "toggle") return Boolean(v);
  const s = String(v ?? "").trim();
  if (s === "") return null;
  return f.kind === "number" || f.kind === "segmented" ? parseInt(s, 10) : s;
}

export interface ServerSettingsForm {
  data: ServerSettings | undefined;
  isLoading: boolean;
  dirty: boolean;
  saving: boolean;
  /** Validates and saves the changed fields. Resolves false when an error kept something from being saved. */
  submit: () => Promise<boolean>;
  renderField: (key: ServerSettingField) => ReactNode;
}

// useServerSettingsForm edits a few fields of the server-settings resource.
// The server takes a partial object (an absent field is left alone, an
// explicit null resets it), so a save sends only the fields changed here.
// Several forms share the query: a refetch after any of their saves re-seeds
// only the fields not being edited, and a save clears only the fields it sent.
export function useServerSettingsForm(fields: ServerSettingField[]): ServerSettingsForm {
  const { t } = useTranslation();
  const tr = t as unknown as Translate;
  const qc = useQueryClient();
  const { data, isLoading } = useServerSettings();
  const [drafts, setDrafts] = useState<Record<string, Draft>>({});
  const [errors, setErrors] = useState<Record<string, string>>({});
  const dirty = useRef<Set<string>>(new Set());
  const [dirtyCount, setDirtyCount] = useState(0);
  // The field list is a literal at each call site (a new array every render):
  // read through a ref so the seed follows the data only.
  const fieldsRef = useRef(fields);
  fieldsRef.current = fields;
  const dataRef = useRef(data);
  dataRef.current = data;

  function seed(keys: readonly string[]) {
    setDrafts((prev) => {
      const next = { ...prev };
      for (const key of keys) next[key] = seedValue(spec(key), dataRef.current?.[key]);
      return next;
    });
  }
  const seedRef = useRef(seed);
  seedRef.current = seed;

  useEffect(() => {
    if (!data) return;
    seedRef.current(fieldsRef.current.filter((k) => !dirty.current.has(k)));
  }, [data]);

  function forget(keys: readonly string[]) {
    for (const k of keys) dirty.current.delete(k);
    setDirtyCount(dirty.current.size);
  }

  function edit(key: ServerSettingField, value: Draft) {
    setDrafts((p) => ({ ...p, [key]: value }));
    setErrors((e) => {
      if (!(key in e)) return e;
      const next = { ...e };
      delete next[key];
      return next;
    });
    dirty.current.add(key);
    setDirtyCount(dirty.current.size);
  }

  const save = useMutation({
    mutationFn: ({ body }: { body: Record<string, unknown>; keys: string[] }) =>
      eveyFetch<ServerSettings>({ url: SERVER_SETTINGS_URL, method: "PUT", data: body }),
    onSuccess: (_r: unknown, vars: { body: Record<string, unknown>; keys: string[] }) => {
      // Only what this save carried: an edit made meanwhile stays pending.
      forget(vars.keys);
      void qc.invalidateQueries({ queryKey: SERVER_SETTINGS_KEY });
      const restart = vars.keys.some((k) => dataRef.current?.[k]?.requiresRestart ?? RESTART_FALLBACK.has(k));
      toast.success(restart ? t("admin.transcoding.savedRestart") : t("admin.serverSettings.saved"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  const reset = useMutation({
    mutationFn: (field: string) =>
      eveyFetch<ServerSettings>({ url: SERVER_SETTINGS_URL, method: "PUT", data: { [field]: null } }),
    onSuccess: (_r: unknown, field: string) => {
      forget([field]);
      void qc.invalidateQueries({ queryKey: SERVER_SETTINGS_KEY });
      toast.success(t("admin.serverSettings.saved"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  async function submit(): Promise<boolean> {
    const keys = fieldsRef.current.filter((k) => dirty.current.has(k));
    const found: Record<string, string> = {};
    for (const k of keys) {
      const msg = validate(spec(k), drafts[k], tr);
      if (msg) found[k] = msg;
    }
    if (Object.keys(found).length > 0) {
      setErrors(found);
      return false;
    }
    const body: Record<string, unknown> = {};
    const kept: string[] = [];
    for (const k of keys) {
      const f = spec(k);
      // An emptied "keep" field means "leave what is there": clearing an
      // override is the Reset link's job. Show what is in force again.
      if (f.kind !== "toggle" && String(drafts[k] ?? "").trim() === "" && (f.blank ?? "keep") === "keep") kept.push(k);
      else body[k] = bodyValue(f, drafts[k]);
    }
    if (kept.length > 0) {
      forget(kept);
      seed(kept);
    }
    const sent = Object.keys(body);
    if (sent.length === 0) return true;
    try {
      await save.mutateAsync({ body, keys: sent });
      return true;
    } catch {
      return false;
    }
  }

  const onOff = (v: unknown) => (v ? t("admin.settingsLayout.on") : t("admin.settingsLayout.off"));

  function renderField(key: ServerSettingField): ReactNode {
    const f = spec(key);
    const s = data?.[key];
    const id = `server-setting-${key}`;
    const label = t(f.labelKey);
    const def = s?.default ?? f.fallbackDefault;
    const locked = s?.locked === true;
    const optionLabel = (v: string) =>
      v === "" ? t(f.autoKey ?? "admin.settingsLayout.defaultOption", { value: String(def) }) : `${v}p`;
    const isZeroDefault = f.zeroIsDefault && Number(def) === 0;
    const defaultText =
      f.kind === "toggle"
        ? onOff(def)
        : f.kind === "segmented" && isZeroDefault
          ? optionLabel("")
          : f.placeholderKey
            ? t(f.placeholderKey, { value: String(def) })
            : String(def);
    const common = {
      label,
      info: f.infoKey ? t(f.infoKey) : undefined,
      hint: f.hintKey ? t(f.hintKey) : undefined,
      error: errors[key],
      restart: s?.requiresRestart ?? RESTART_FALLBACK.has(key),
      reset: s
        ? {
            overridden: s.source === "override" && !(f.zeroIsDefault && Number(s.value) === 0),
            defaultText,
            onReset: () => reset.mutate(key),
            disabled: locked || reset.isPending,
          }
        : undefined,
      "data-testid": `setting-row-${key}`,
    };
    const draft = drafts[key] ?? seedValue(f, s);

    if (f.kind === "toggle") {
      return (
        <SettingRow key={key} {...common} kind="toggle">
          <Switch checked={Boolean(draft)} onChange={() => edit(key, !draft)} label={label} disabled={locked} data-testid={f.testId} />
        </SettingRow>
      );
    }
    if (f.kind === "segmented") {
      const labelId = `${id}-label`;
      return (
        <SettingRow key={key} {...common} labelId={labelId} kind="wide">
          <SegmentedControl
            id={id}
            label={label}
            labelledBy={labelId}
            data-testid={f.testId}
            value={String(draft)}
            disabled={locked}
            onChange={(v) => edit(key, v)}
            options={(f.options ?? []).map((v) => ({ value: v, label: optionLabel(v) }))}
            formatUnknown={(v) => `${v}p`}
          />
        </SettingRow>
      );
    }
    if (f.kind === "select") {
      return (
        <SettingRow key={key} {...common} htmlFor={id}>
          <Select
            id={id}
            data-testid={f.testId}
            value={String(draft)}
            disabled={locked}
            onChange={(v) => edit(key, v)}
            options={[
              { label: t("admin.settingsLayout.defaultOption", { value: String(def) }), value: "" },
              ...(f.options ?? []).map((o) => ({ label: o, value: o })),
            ]}
          />
        </SettingRow>
      );
    }
    return (
      <SettingRow key={key} {...common} htmlFor={id}>
        <Input
          id={id}
          data-testid={f.testId}
          className="w-full"
          type={f.kind === "number" ? "number" : "text"}
          inputMode={f.kind === "number" ? "numeric" : undefined}
          min={f.min}
          placeholder={f.placeholderKey ? t(f.placeholderKey, { value: String(def) }) : String(def)}
          value={String(draft)}
          disabled={locked}
          aria-invalid={errors[key] ? true : undefined}
          onChange={(e) => edit(key, e.target.value)}
        />
      </SettingRow>
    );
  }

  return { data, isLoading, dirty: dirtyCount > 0, saving: save.isPending, submit, renderField };
}

interface ServerSettingsGroupProps {
  title: string;
  description?: string;
  info?: string;
  fields: ServerSettingField[];
  /** data-testid of the group's Save button. */
  saveTestId: string;
  /** A line shown above the fields (a note, a warning). */
  before?: ReactNode;
  "data-testid"?: string;
}

// ServerSettingsGroup is a titled card of server settings with its own Save.
export function ServerSettingsGroup({ title, description, info, fields, saveTestId, before, "data-testid": testId }: ServerSettingsGroupProps) {
  const { t } = useTranslation();
  const form = useServerSettingsForm(fields);
  return (
    <SettingsGroup
      title={title}
      description={description}
      info={info}
      data-testid={testId}
      footer={
        form.data ? (
          <Button onClick={() => void form.submit()} loading={form.saving} disabled={!form.dirty} data-testid={saveTestId}>
            {t("common.save")}
          </Button>
        ) : undefined
      }
    >
      {form.isLoading || !form.data ? (
        <div className="flex justify-center py-4"><Spinner size="md" /></div>
      ) : (
        <>
          {before}
          {fields.map((key) => form.renderField(key))}
        </>
      )}
    </SettingsGroup>
  );
}
