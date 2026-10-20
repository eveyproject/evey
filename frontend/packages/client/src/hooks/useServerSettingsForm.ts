import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";

// The /api/v1/admin/server-settings resource: one object of typed settings,
// each with its value, its default, whether an override is stored, whether
// the server reads it only at boot, and whether the demo greys it. The web
// and the phone edit it through the form below; each draws the controls its
// own way from the field view the form hands out, so the rules (what a save
// sends, what a refetch may overwrite, what is refused) live here once.
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

export type ServerSettingFieldKind = "text" | "number" | "toggle" | "select" | "segmented";

export interface ServerSettingFieldSpec {
  kind: ServerSettingFieldKind;
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
  /** What an emptied text field means on Save: keep the stored value (default) or clear the override (null). A choice's "" option always resets. */
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
  logLevel: { kind: "select", labelKey: "admin.serverSettings.logLevel", infoKey: "admin.settingsHelp.logLevel", testId: "log-level-select", fallbackDefault: "info", options: ["debug", "info", "warn", "error"], blank: "reset" },
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
} satisfies Record<string, ServerSettingFieldSpec>;

export type ServerSettingField = keyof typeof SERVER_SETTING_FIELDS;

// Settings the server reads only at boot. The server reports this itself
// (requiresRestart); the list covers a server too old to say so.
export const SERVER_SETTINGS_RESTART_FALLBACK: ReadonlySet<string> = new Set<string>([
  "libraryWatcherEnabled", "periodicScanInterval", "transcodeCacheMaxGB", "transcodeMaxSessions",
  "softwareMaxHeight", "softwareHDRMaxHeight", "transcodeSingleVariant", "transcodeHWPerVariant",
]);

// A Go time.Duration as time.ParseDuration reads it: one or more number+unit
// pairs ("90s", "1h30m"), here required to be above zero. The server silently
// ignores anything else.
const GO_DURATION = /^(\d+(\.\d+)?(ns|us|µs|ms|s|m|h))+$/;
export function isPositiveGoDuration(v: string): boolean {
  return GO_DURATION.test(v) && /[1-9]/.test(v);
}

export type ServerSettingDraft = string | boolean;
export type ServerSettingsTranslate = (key: string, opts?: Record<string, unknown>) => string;

const spec = (key: string): ServerSettingFieldSpec => SERVER_SETTING_FIELDS[key as ServerSettingField];

// seedValue is what a control shows for a setting: a toggle shows the
// effective value; any other field shows only an override, blank meaning
// "use the default" (pre-filling the effective value would turn every save
// into an override of a value nobody chose).
export function serverSettingSeed(f: ServerSettingFieldSpec, s: SettingValue | undefined): ServerSettingDraft {
  if (f.kind === "toggle") return Boolean(s?.value ?? f.fallbackDefault);
  if (s?.source !== "override") return "";
  if (f.zeroIsDefault && Number(s.value) === 0) return "";
  return String(s.value);
}

// serverSettingError returns the message of an invalid draft, or null. The
// server drops an invalid value without an error, so the form must refuse it
// itself.
export function serverSettingError(f: ServerSettingFieldSpec, v: ServerSettingDraft | undefined, t: ServerSettingsTranslate): string | null {
  if (typeof v !== "string" || v.trim() === "") return null;
  if (f.kind === "number") {
    const min = f.min ?? 0;
    if (!/^\d+$/.test(v.trim()) || parseInt(v, 10) < min) return t("admin.settingsLayout.errorNumber", { min });
  }
  if (f.duration && !isPositiveGoDuration(v.trim())) return t("admin.settingsLayout.errorDuration");
  return null;
}

// blankRule says what an emptied field means on Save. On a choice ("select"
// or "segmented") the "" option is the Default / Auto pick itself, so it can
// only mean "clear the override": keeping the stored value would make that
// pick a silent no-op.
function blankRule(f: ServerSettingFieldSpec): "keep" | "reset" {
  if (f.kind === "select" || f.kind === "segmented") return "reset";
  return f.blank ?? "keep";
}

function bodyValue(f: ServerSettingFieldSpec, v: ServerSettingDraft | undefined): unknown {
  if (f.kind === "toggle") return Boolean(v);
  const s = String(v ?? "").trim();
  if (s === "") return null;
  return f.kind === "number" || f.kind === "segmented" ? parseInt(s, 10) : s;
}

export interface ServerSettingOption {
  value: string;
  label: string;
}

/** Everything a platform needs to draw one setting row and its control. */
export interface ServerSettingFieldView {
  key: ServerSettingField;
  spec: ServerSettingFieldSpec;
  label: string;
  info?: string;
  hint?: string;
  error?: string;
  /** The server reads it at boot: show the "applied on restart" badge. */
  restart: boolean;
  /** The default / reset line; absent until the server answered. */
  reset?: { overridden: boolean; defaultText: string; onReset: () => void; disabled: boolean };
  /** What the control shows: a boolean for a toggle, the text otherwise ("" = the default). */
  draft: ServerSettingDraft;
  /** The demo greys it. */
  locked: boolean;
  /** Choices of a select or a segmented field, the "" (default / Auto) one first. */
  options?: ServerSettingOption[];
  /** Label of a stored value no option offers (an off-ladder height). */
  formatUnknown: (value: string) => string;
  /** Placeholder of a text or number field. */
  placeholder: string;
  onChange: (value: ServerSettingDraft) => void;
}

export interface ServerSettingsFormState {
  data: ServerSettings | undefined;
  isLoading: boolean;
  dirty: boolean;
  saving: boolean;
  /** Validates and saves the changed fields. Resolves false when an error kept something from being saved. */
  submit: () => Promise<boolean>;
  field: (key: ServerSettingField) => ServerSettingFieldView;
}

// useServerSettingsFormState edits a few fields of the server-settings
// resource. The server takes a partial object (an absent field is left alone,
// an explicit null resets it), so a save sends only the fields changed here.
// Several forms share the query: a refetch after any of their saves re-seeds
// only the fields not being edited, and a save clears only the fields it sent.
// `t` is the app's translate function: this package carries no i18n binding.
export function useServerSettingsFormState(fields: ServerSettingField[], t: ServerSettingsTranslate): ServerSettingsFormState {
  const qc = useQueryClient();
  const { data, isLoading } = useServerSettings();
  const [drafts, setDrafts] = useState<Record<string, ServerSettingDraft>>({});
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
      for (const key of keys) next[key] = serverSettingSeed(spec(key), dataRef.current?.[key]);
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

  function edit(key: ServerSettingField, value: ServerSettingDraft) {
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
      const restart = vars.keys.some((k) => dataRef.current?.[k]?.requiresRestart ?? SERVER_SETTINGS_RESTART_FALLBACK.has(k));
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
      const msg = serverSettingError(spec(k), drafts[k], t);
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
      if (f.kind !== "toggle" && String(drafts[k] ?? "").trim() === "" && blankRule(f) === "keep") kept.push(k);
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

  function field(key: ServerSettingField): ServerSettingFieldView {
    const f = spec(key);
    const s = data?.[key];
    const def = s?.default ?? f.fallbackDefault;
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
    const locked = s?.locked === true;
    let options: ServerSettingOption[] | undefined;
    if (f.kind === "segmented") options = (f.options ?? []).map((v) => ({ value: v, label: optionLabel(v) }));
    else if (f.kind === "select")
      options = [
        { value: "", label: t("admin.settingsLayout.defaultOption", { value: String(def) }) },
        ...(f.options ?? []).map((o) => ({ value: o, label: o })),
      ];
    return {
      key,
      spec: f,
      label: t(f.labelKey),
      info: f.infoKey ? t(f.infoKey) : undefined,
      hint: f.hintKey ? t(f.hintKey) : undefined,
      error: errors[key],
      restart: s?.requiresRestart ?? SERVER_SETTINGS_RESTART_FALLBACK.has(key),
      reset: s
        ? {
            overridden: s.source === "override" && !(f.zeroIsDefault && Number(s.value) === 0),
            defaultText,
            onReset: () => reset.mutate(key),
            disabled: locked || reset.isPending,
          }
        : undefined,
      draft: drafts[key] ?? serverSettingSeed(f, s),
      locked,
      options,
      formatUnknown: (v) => `${v}p`,
      placeholder: f.placeholderKey ? t(f.placeholderKey, { value: String(def) }) : String(def),
      onChange: (v) => edit(key, v),
    };
  }

  return { data, isLoading, dirty: dirtyCount > 0, saving: save.isPending, submit, field };
}
