import { useState, useEffect } from "react";
import { Folder } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { toast, useLocaleStore } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { PageHeader } from "@/components/ui/PageHeader";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Checkbox } from "@/components/ui/Checkbox";
import { Card } from "@/components/ui/Card";

const SMTP_KEYS = ["smtp.host", "smtp.port", "smtp.username", "smtp.password", "smtp.from", "smtp.tls"] as const;
type SmtpKey = (typeof SMTP_KEYS)[number];

const TLS_OPTIONS: { label: string; value: string }[] = [
  { label: "None", value: "none" },
  { label: "STARTTLS", value: "starttls" },
  { label: "SSL/TLS", value: "ssl" },
];

export const Route = createLazyFileRoute("/_auth/admin/server-config")({
  component: ServerConfigPage,
});

interface ConfigEntry {
  key: string;
  value: string;
  updatedAt: string;
}

const QUERY_KEY = ["/api/v1/admin/server-config"];

function useServerConfig() {
  return useQuery<ConfigEntry[]>({
    queryKey: QUERY_KEY,
    queryFn: async () => {
      const data = await eveyFetch<{ entries: ConfigEntry[] }>({ url: "/api/v1/admin/server-config", method: "GET" });
      return data.entries ?? [];
    },
  });
}

function useSetConfigEntry() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      eveyFetch<void>({ url: `/api/v1/admin/server-config/${encodeURIComponent(key)}`, method: "PUT", data: { value } }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      toast.success(t("admin.serverConfig.configUpdated"));
    },
    onError: () => toast.error(t("admin.serverConfig.failedUpdate")),
  });
}

function useDeleteConfigEntry() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (key: string) =>
      eveyFetch<void>({ url: `/api/v1/admin/server-config/${encodeURIComponent(key)}`, method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEY });
      toast.success(t("admin.serverConfig.configDeleted"));
    },
    onError: () => toast.error(t("admin.serverConfig.failedDeleteEntry")),
  });
}

function ConfigRow({ entry }: { entry: ConfigEntry }) {
  const { t } = useTranslation();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(entry.value);
  const setEntry = useSetConfigEntry();
  const deleteEntry = useDeleteConfigEntry();

  function handleSave() {
    setEntry.mutate({ key: entry.key, value });
    setEditing(false);
  }

  return (
    <tr className="border-b border-[var(--border)]">
      <td className="px-4 py-3 font-mono text-sm text-[var(--text)]">
        {entry.key}
      </td>
      <td className="px-4 py-3 text-sm text-[var(--text)]">
        {editing ? (
          <input
            className="w-full px-2 py-1 rounded text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") handleSave();
              if (e.key === "Escape") { setValue(entry.value); setEditing(false); }
            }}
            autoFocus
          />
        ) : (
          <span className="font-mono text-xs break-all">{entry.value}</span>
        )}
      </td>
      <td className="px-4 py-3 text-xs whitespace-nowrap text-[var(--text-muted)]">
        {new Date(entry.updatedAt).toLocaleString()}
      </td>
      <td className="px-4 py-3">
        <div className="flex gap-2">
          {editing ? (
            <>
              <Button variant="secondary" onClick={handleSave} loading={setEntry.isPending} className="text-xs">{t("common.save")}</Button>
              <Button variant="ghost" onClick={() => { setValue(entry.value); setEditing(false); }} className="text-xs">{t("common.cancel")}</Button>
            </>
          ) : (
            <>
              <Button variant="secondary" onClick={() => setEditing(true)} className="text-xs">{t("common.edit")}</Button>
              <Button variant="danger" onClick={() => deleteEntry.mutate(entry.key)} loading={deleteEntry.isPending} className="text-xs">{t("common.delete")}</Button>
            </>
          )}
        </div>
      </td>
    </tr>
  );
}

const LOCALE_OPTIONS = [
  { label: "English (US)", value: "en-US" },
  { label: "Fran\u00E7ais (FR)", value: "fr-FR" },
  { label: "Deutsch (DE)", value: "de-DE" },
  { label: "Espa\u00F1ol (ES)", value: "es-ES" },
  { label: "Italiano (IT)", value: "it-IT" },
  { label: "Portugu\u00EAs (BR)", value: "pt-BR" },
  { label: "\u65E5\u672C\u8A9E (JP)", value: "ja-JP" },
  { label: "\uD55C\uAD6D\uC5B4 (KR)", value: "ko-KR" },
  { label: "\u4E2D\u6587 (CN)", value: "zh-CN" },
];

const METADATA_LANG_KEY = "metadata.language";

function LocaleSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();
  const setLocale = useLocaleStore((s) => s.setLocale);

  const current = entries?.find((e) => e.key === METADATA_LANG_KEY)?.value ?? "";

  function handleChange(value: string) {
    setEntry.mutate(
      { key: METADATA_LANG_KEY, value },
      {
        onSuccess: () => {
          setLocale(value);
          toast.success(t("admin.serverConfig.languageUpdated"));
        },
      }
    );
  }

  return (
    <div
      className="rounded-[var(--radius-lg)] p-5 border border-[var(--border)] bg-[var(--surface)]"
    >
      <h2 className="text-base font-semibold mb-1 text-[var(--text)]">
        {t("admin.serverConfig.languageRegion")}
      </h2>
      <p className="text-sm mb-4 text-[var(--text-muted)]">
        {t("admin.serverConfig.languageRegionDesc")}
      </p>
      <div className="flex items-end gap-3">
        <Select
          id="metadata-lang"
          label={t("admin.serverConfig.metadataLanguage")}
          value={current}
          onChange={handleChange}
          options={[{ label: t("admin.serverConfig.envDefault"), value: "" }, ...LOCALE_OPTIONS]}
        />
        {setEntry.isPending && <Spinner size="sm" />}
      </div>
    </div>
  );
}

function AddEntryRow() {
  const { t } = useTranslation();
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const setEntry = useSetConfigEntry();

  function handleAdd() {
    if (!key.trim()) return;
    setEntry.mutate({ key: key.trim(), value }, {
      onSuccess: () => { setKey(""); setValue(""); },
    });
  }

  return (
    <tr className="border-b border-[var(--border)] bg-[var(--surface)]">
      <td className="px-4 py-3">
        <input
          placeholder="key"
          className="w-full px-2 py-1 rounded text-sm font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
      </td>
      <td className="px-4 py-3">
        <input
          placeholder="value"
          className="w-full px-2 py-1 rounded text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Enter") handleAdd(); }}
        />
      </td>
      <td />
      <td className="px-4 py-3">
        <Button onClick={handleAdd} loading={setEntry.isPending} className="text-xs" disabled={!key.trim()}>{t("common.add")}</Button>
      </td>
    </tr>
  );
}

const API_KEY_FIELDS = [
  {
    dbKey: "api.tmdb_key",
    labelKey: "admin.serverConfig.apiKeys.tmdb",
    linkKey: "admin.serverConfig.apiKeys.tmdbLink",
    linkUrl: "https://www.themoviedb.org/settings/api",
    hasDefault: true,
  },
  {
    dbKey: "api.omdb_key",
    labelKey: "admin.serverConfig.apiKeys.omdb",
    linkKey: "admin.serverConfig.apiKeys.omdbLink",
    linkUrl: "https://www.omdbapi.com/apikey.aspx",
    hasDefault: false,
  },
  {
    dbKey: "api.opensubtitles_key",
    labelKey: "admin.serverConfig.apiKeys.opensubtitles",
    linkKey: "admin.serverConfig.apiKeys.opensubtitlesLink",
    linkUrl: "https://www.opensubtitles.com/en/consumers",
    hasDefault: false,
  },
] as const;

function maskKey(value: string): string {
  if (value.length <= 8) return value;
  return value.slice(0, 4) + "\u2022".repeat(6) + value.slice(-4);
}

function ApiKeysCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();
  const deleteEntry = useDeleteConfigEntry();

  const [drafts, setDrafts] = useState<Record<string, string>>({});
  const [editing, setEditing] = useState<Record<string, boolean>>({});
  const [showValue, setShowValue] = useState<Record<string, boolean>>({});

  function val(dbKey: string) {
    return entries?.find((e) => e.key === dbKey)?.value ?? "";
  }

  function handleSave(dbKey: string) {
    const v = drafts[dbKey] ?? "";
    setEntry.mutate(
      { key: dbKey, value: v },
      { onSuccess: () => toast.success(t("admin.serverConfig.apiKeys.saved")) },
    );
    setEditing((p) => ({ ...p, [dbKey]: false }));
  }

  function handleReset(dbKey: string) {
    deleteEntry.mutate(dbKey, {
      onSuccess: () => toast.success(t("admin.serverConfig.apiKeys.usingDefault")),
    });
    setEditing((p) => ({ ...p, [dbKey]: false }));
    setDrafts((p) => ({ ...p, [dbKey]: "" }));
  }

  return (
    <div
      className="rounded-[var(--radius-lg)] p-5 border border-[var(--border)] bg-[var(--surface)]"
    >
      <h2 className="text-base font-semibold mb-1 text-[var(--text)]">
        {t("admin.serverConfig.apiKeys.title")}
      </h2>
      <p className="text-sm mb-4 text-[var(--text-muted)]">
        {t("admin.serverConfig.apiKeys.description")}
      </p>

      <div className="flex flex-col gap-4">
        {API_KEY_FIELDS.map((field) => {
          const current = val(field.dbKey);
          const isEditing = editing[field.dbKey] ?? false;
          const isShown = showValue[field.dbKey] ?? false;

          return (
            <div key={field.dbKey} className="flex flex-col gap-1.5">
              <div className="flex items-center gap-2">
                <label className="text-sm font-medium text-[var(--text)]">
                  {t(field.labelKey)}
                </label>
                {field.hasDefault && !current && (
                  <span className="text-xs px-1.5 py-0.5 rounded bg-[var(--surface-raised)] text-[var(--text-muted)]">
                    {t("admin.serverConfig.apiKeys.usingDefault")}
                  </span>
                )}
                {current && (
                  <span className="text-xs px-1.5 py-0.5 rounded bg-[var(--accent-subtle)] text-[var(--accent)]">
                    {t("admin.serverConfig.apiKeys.usingCustom")}
                  </span>
                )}
              </div>

              {isEditing ? (
                <div className="flex gap-2">
                  <input
                    className="px-3 py-1.5 rounded text-sm flex-1 font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
                    type="text"
                    value={drafts[field.dbKey] ?? current}
                    onChange={(e) => setDrafts((p) => ({ ...p, [field.dbKey]: e.target.value }))}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") handleSave(field.dbKey);
                      if (e.key === "Escape") setEditing((p) => ({ ...p, [field.dbKey]: false }));
                    }}
                    autoFocus
                  />
                  <Button variant="secondary" onClick={() => handleSave(field.dbKey)} loading={setEntry.isPending} className="text-xs">
                    {t("common.save")}
                  </Button>
                  <Button variant="ghost" onClick={() => setEditing((p) => ({ ...p, [field.dbKey]: false }))} className="text-xs">
                    {t("common.cancel")}
                  </Button>
                </div>
              ) : (
                <div className="flex items-center gap-2">
                  {current ? (
                    <span className="font-mono text-xs text-[var(--text-muted)]">
                      {isShown ? current : maskKey(current)}
                    </span>
                  ) : (
                    <span className="text-xs text-[var(--text-muted)]">
                      {field.hasDefault ? maskKey("eyJhbGciOiJIUzI1NiJ9...V51Q") : "---"}
                    </span>
                  )}
                  {current && (
                    <button
                      type="button"
                      onClick={() => setShowValue((p) => ({ ...p, [field.dbKey]: !p[field.dbKey] }))}
                      className="px-1.5 rounded text-xs border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text-muted)]"
                    >
                      {isShown ? t("admin.serverConfig.hide") : t("admin.serverConfig.show")}
                    </button>
                  )}
                  <Button variant="secondary" onClick={() => { setDrafts((p) => ({ ...p, [field.dbKey]: current })); setEditing((p) => ({ ...p, [field.dbKey]: true })); }} className="text-xs">
                    {t("common.edit")}
                  </Button>
                  {field.hasDefault && current && (
                    <Button variant="ghost" onClick={() => handleReset(field.dbKey)} loading={deleteEntry.isPending} className="text-xs">
                      {t("admin.serverConfig.apiKeys.resetToDefault")}
                    </Button>
                  )}
                </div>
              )}

              <a
                href={field.linkUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="text-xs underline text-[var(--accent)]"
              >
                {t(field.linkKey)}
              </a>
            </div>
          );
        })}
      </div>
    </div>
  );
}

function SmtpSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  function val(key: SmtpKey) {
    return entries?.find((e) => e.key === key)?.value ?? "";
  }

  const [host, setHost] = useState(() => val("smtp.host"));
  const [port, setPort] = useState(() => val("smtp.port") || "587");
  const [username, setUsername] = useState(() => val("smtp.username"));
  const [password, setPassword] = useState(() => val("smtp.password"));
  const [from, setFrom] = useState(() => val("smtp.from"));
  const [tls, setTls] = useState(() => val("smtp.tls") || "starttls");
  const [showPassword, setShowPassword] = useState(false);

  function handleSave() {
    const pairs: { key: SmtpKey; value: string }[] = [
      { key: "smtp.host", value: host },
      { key: "smtp.port", value: port },
      { key: "smtp.username", value: username },
      { key: "smtp.password", value: password },
      { key: "smtp.from", value: from },
      { key: "smtp.tls", value: tls },
    ];
    let remaining = pairs.length;
    let hasError = false;
    for (const { key, value } of pairs) {
      setEntry.mutate(
        { key, value },
        {
          onSuccess: () => {
            remaining -= 1;
            if (remaining === 0 && !hasError) toast.success(t("admin.serverConfig.smtpSaved"));
          },
          onError: () => {
            hasError = true;
            toast.error(t("admin.serverConfig.failedSmtp", { key }));
          },
        }
      );
    }
  }

  return (
    <div
      className="rounded-[var(--radius-lg)] p-5 border border-[var(--border)] bg-[var(--surface)]"
    >
      <h2 className="text-base font-semibold mb-1 text-[var(--text)]">
        {t("admin.serverConfig.emailSmtp")}
      </h2>
      <p className="text-sm mb-4 text-[var(--text-muted)]">
        {t("admin.serverConfig.emailSmtpDesc")}
      </p>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
        <Input label={t("admin.serverConfig.smtpHost")} placeholder="smtp.example.com" value={host} onChange={(e) => setHost(e.target.value)} />
        <Input label={t("admin.serverConfig.port")} type="number" placeholder="587" value={port} onChange={(e) => setPort(e.target.value)} />
        <Input label={t("admin.serverConfig.smtpUsername")} placeholder="user@example.com" value={username} onChange={(e) => setUsername(e.target.value)} />
        <div className="flex flex-col gap-1">
          <Input label={t("admin.serverConfig.smtpPassword")} type={showPassword ? "text" : "password"} placeholder="••••••••" value={password} onChange={(e) => setPassword(e.target.value)} />
          <button type="button" onClick={() => setShowPassword((v) => !v)} className="self-start px-2 rounded text-xs border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text-muted)]">
            {showPassword ? t("admin.serverConfig.hide") : t("admin.serverConfig.show")}
          </button>
        </div>
        <Input label={t("admin.serverConfig.fromAddress")} placeholder="Evey <noreply@example.com>" value={from} onChange={(e) => setFrom(e.target.value)} />
        <Select label={t("admin.serverConfig.tlsMode")} value={tls} onChange={setTls} options={TLS_OPTIONS} />
      </div>

      <Button onClick={handleSave} loading={setEntry.isPending}>
        {t("admin.serverConfig.saveSmtp")}
      </Button>
    </div>
  );
}

const TRICKPLAY_KEY = "trickplay.enabled";
const TRICKPLAY_CPU_KEY = "trickplay.adaptive_cpu";

function TrickplaySettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const enabled = entries?.find((e) => e.key === TRICKPLAY_KEY)?.value !== "false";
  const adaptiveCpu = entries?.find((e) => e.key === TRICKPLAY_CPU_KEY)?.value !== "false";

  return (
    <div
      className="rounded-[var(--radius-lg)] p-5 border border-[var(--border)] bg-[var(--surface)]"
    >
      <h2 className="text-base font-semibold mb-1 text-[var(--text)]">
        {t("admin.serverConfig.trickplayTitle")}
      </h2>
      <p className="text-sm mb-4 text-[var(--text-muted)]">
        {t("admin.serverConfig.trickplayDesc")}
      </p>
      <div className="flex flex-col gap-3">
        <div className="flex items-center gap-3">
          <Checkbox
            label={t("admin.serverConfig.trickplayEnabled")}
            checked={enabled}
            onChange={(v) => setEntry.mutate({ key: TRICKPLAY_KEY, value: v ? "true" : "false" })}
            disabled={setEntry.isPending}
          />
        </div>
        <div className="flex items-center gap-3">
          <Checkbox
            label={t("admin.serverConfig.trickplayAdaptiveCpu")}
            checked={adaptiveCpu}
            onChange={(v) => setEntry.mutate({ key: TRICKPLAY_CPU_KEY, value: v ? "true" : "false" })}
            disabled={setEntry.isPending || !enabled}
          />
        </div>
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.serverConfig.trickplayAdaptiveCpuDesc")}
        </p>
        {setEntry.isPending && <Spinner size="sm" />}
      </div>
    </div>
  );
}

interface SettingValue<T> { value: T; default: T; source: "override" | "default"; }
interface ServerSettings {
  serverName: SettingValue<string>;
  maxConcurrentTranscodes: SettingValue<number>;
  hwAccelEnabled: SettingValue<boolean>;
  loginRateLimit: SettingValue<number>;
  globalRateLimit: SettingValue<number>;
}
const SETTINGS_KEY = ["/api/v1/admin/server-settings"];

function ServerSettingsCard() {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery<ServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });
  const queryClient = useQueryClient();
  const [serverName, setServerName] = useState("");
  const [maxTranscodes, setMaxTranscodes] = useState("");
  const [hwAccel, setHwAccel] = useState(false);
  const [loginRate, setLoginRate] = useState("");
  const [globalRate, setGlobalRate] = useState("");
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (data) {
      setServerName(data.serverName.source === "override" ? String(data.serverName.value) : "");
      setMaxTranscodes(data.maxConcurrentTranscodes.source === "override" ? String(data.maxConcurrentTranscodes.value) : "");
      setHwAccel(data.hwAccelEnabled.value);
      setLoginRate(data.loginRateLimit.source === "override" ? String(data.loginRateLimit.value) : "");
      setGlobalRate(data.globalRateLimit.source === "override" ? String(data.globalRateLimit.value) : "");
    }
  }, [data]);

  const saveMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: body }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: SETTINGS_KEY }); toast.success(t("admin.serverSettings.saved")); setDirty(false); },
    onError: (err: Error) => toast.error(err.message),
  });

  const resetMutation = useMutation({
    mutationFn: (field: string) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: { [field]: null } }),
    onSuccess: () => { queryClient.invalidateQueries({ queryKey: SETTINGS_KEY }); toast.success(t("admin.serverSettings.saved")); },
    onError: (err: Error) => toast.error(err.message),
  });

  function handleSave() {
    const body: Record<string, unknown> = {};
    if (serverName) body.serverName = serverName;
    if (maxTranscodes) body.maxConcurrentTranscodes = parseInt(maxTranscodes, 10);
    body.hwAccelEnabled = hwAccel;
    if (loginRate) body.loginRateLimit = parseInt(loginRate, 10);
    if (globalRate) body.globalRateLimit = parseInt(globalRate, 10);
    saveMutation.mutate(body);
  }

  function ResetBtn({ setting, field }: { setting: SettingValue<unknown>; field: string }) {
    if (setting.source === "default") return <span className="text-xs text-[var(--text-muted)]">{t("admin.serverSettings.usingDefault")} {String(setting.default)}</span>;
    return <button type="button" onClick={() => resetMutation.mutate(field)} className="text-xs text-[var(--accent)] hover:underline cursor-pointer">{t("admin.serverSettings.reset")}</button>;
  }

  if (isLoading) return <div className="flex justify-center py-8"><Spinner size="md" /></div>;
  if (!data) return null;

  return (
    <Card>
      <div className="p-5">
        <h2 className="text-base font-semibold mb-4 text-[var(--text)]">{t("admin.serverSettings.title")}</h2>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <Input label={t("admin.serverSettings.serverName")} placeholder={data.serverName.default ?? "Evey"} value={serverName} onChange={(e) => { setServerName(e.target.value); setDirty(true); }} />
            <ResetBtn setting={data.serverName} field="serverName" />
          </div>
          <div className="flex flex-col gap-1">
            <Input label={t("admin.serverSettings.maxConcurrentTranscodes")} type="number" min={0} placeholder={String(data.maxConcurrentTranscodes.default ?? 2)} value={maxTranscodes} onChange={(e) => { setMaxTranscodes(e.target.value); setDirty(true); }} />
            <ResetBtn setting={data.maxConcurrentTranscodes} field="maxConcurrentTranscodes" />
          </div>
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-3">
              <label className="text-sm font-medium text-[var(--text)]">{t("admin.serverSettings.hwAccelEnabled")}</label>
              <button type="button" role="switch" aria-checked={hwAccel} onClick={() => { setHwAccel(!hwAccel); setDirty(true); }}
                className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors" style={{ backgroundColor: hwAccel ? "var(--accent)" : "var(--border)" }}>
                <span className="inline-block h-4 w-4 rounded-full bg-white transition-transform" style={{ transform: hwAccel ? "translateX(1.375rem)" : "translateX(0.25rem)" }} />
              </button>
            </div>
            <ResetBtn setting={data.hwAccelEnabled} field="hwAccelEnabled" />
          </div>
          <div className="flex flex-col gap-1">
            <Input label={t("admin.serverSettings.loginRateLimit")} type="number" min={0} placeholder={String(data.loginRateLimit.default ?? 10)} value={loginRate} onChange={(e) => { setLoginRate(e.target.value); setDirty(true); }} />
            <ResetBtn setting={data.loginRateLimit} field="loginRateLimit" />
          </div>
          <div className="flex flex-col gap-1">
            <Input label={t("admin.serverSettings.globalRateLimit")} type="number" min={0} placeholder={String(data.globalRateLimit.default ?? 300)} value={globalRate} onChange={(e) => { setGlobalRate(e.target.value); setDirty(true); }} />
            <ResetBtn setting={data.globalRateLimit} field="globalRateLimit" />
          </div>
          <div className="flex justify-end pt-2">
            <Button onClick={handleSave} loading={saveMutation.isPending} disabled={!dirty}>{t("common.save")}</Button>
          </div>
        </div>
      </div>
    </Card>
  );
}

function ServerConfigPage() {
  const { t } = useTranslation();
  const { data: entries, isLoading } = useServerConfig();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.serverConfig.title")} subtitle={t("admin.serverConfig.subtitle")} />

      <LocaleSettingsCard entries={entries} />

      <ApiKeysCard entries={entries} />

      <SmtpSettingsCard entries={entries} />

      <TrickplaySettingsCard entries={entries} />

      <ServerSettingsCard />

      {isLoading ? (
        <div className="flex justify-center py-16"><Spinner size="lg" /></div>
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
                {[t("admin.serverConfig.tableHeaders.key"), t("admin.serverConfig.tableHeaders.value"), t("admin.serverConfig.tableHeaders.updated"), t("admin.serverConfig.tableHeaders.actions")].map((h) => (
                  <th key={h} className="px-4 py-2 text-left font-medium text-[var(--text-muted)]">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {entries && entries.length === 0 ? (
                <tr>
                  <td colSpan={4}>
                    <EmptyState icon={<Folder size={48} strokeWidth={1.5} />} title={t("admin.serverConfig.noEntries")} description={t("admin.serverConfig.noEntriesDesc")} />
                  </td>
                </tr>
              ) : (
                entries?.map((e) => <ConfigRow key={e.key} entry={e} />)
              )}
              <AddEntryRow />
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
