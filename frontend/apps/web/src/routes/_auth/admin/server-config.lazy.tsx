import { useState, useEffect, useMemo } from "react";
import { Folder } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  eveyFetch,
  useServerConfigServiceListServerConfigEntries,
  serverConfigServiceSetServerConfigEntry,
  serverConfigServiceDeleteServerConfigEntry,
  getServerConfigServiceListServerConfigEntriesQueryKey,
} from "@evey/api";
import type { V1ServerConfigEntry } from "@evey/api";
import { toast, useLocaleStore } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { PageHeader } from "@/components/ui/PageHeader";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { Checkbox } from "@/components/ui/Checkbox";
import { CollapsibleCard } from "@/components/ui/CollapsibleCard";
import { InfoTooltip } from "@/components/ui/InfoTooltip";
import { FeedConfigCard } from "@/features/admin/FeedConfigCard";
import { ISO_REGIONS } from "@/features/user/regionList";
import { defaultRegionForLocale } from "@/features/user/regionDefaults";
import { supportedLanguages } from "@/i18n";

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

type ConfigEntry = V1ServerConfigEntry;

const QUERY_KEY = getServerConfigServiceListServerConfigEntriesQueryKey();

function useServerConfig() {
  return useServerConfigServiceListServerConfigEntries({
    query: { select: (d) => d.entries ?? [] },
  });
}

function useSetConfigEntry() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      serverConfigServiceSetServerConfigEntry(key, { value }),
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
    mutationFn: (key: string) => serverConfigServiceDeleteServerConfigEntry(key),
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
  const [value, setValue] = useState(entry.value ?? "");
  const setEntry = useSetConfigEntry();
  const deleteEntry = useDeleteConfigEntry();

  function handleSave() {
    setEntry.mutate({ key: entry.key ?? "", value });
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
              if (e.key === "Escape") { setValue(entry.value ?? ""); setEditing(false); }
            }}
            autoFocus
          />
        ) : (
          <span className="font-mono text-xs break-all">{entry.value}</span>
        )}
      </td>
      <td className="px-4 py-3 text-xs whitespace-nowrap text-[var(--text-muted)]">
        {entry.updatedAt ? new Date(entry.updatedAt).toLocaleString() : ""}
      </td>
      <td className="px-4 py-3">
        <div className="flex gap-2">
          {editing ? (
            <>
              <Button variant="secondary" onClick={handleSave} loading={setEntry.isPending} className="text-xs">{t("common.save")}</Button>
              <Button variant="ghost" onClick={() => { setValue(entry.value ?? ""); setEditing(false); }} className="text-xs">{t("common.cancel")}</Button>
            </>
          ) : (
            <>
              <Button variant="secondary" onClick={() => setEditing(true)} className="text-xs">{t("common.edit")}</Button>
              <Button variant="danger" onClick={() => deleteEntry.mutate(entry.key ?? "")} loading={deleteEntry.isPending} className="text-xs">{t("common.delete")}</Button>
            </>
          )}
        </div>
      </td>
    </tr>
  );
}

// Kept in sync with backend i18n supported locales + frontend setup wizard.
// Admin panel must use the same short codes ("fr", "en" ...) that the setup
// wizard persists to server.language — otherwise the Select can't match the
// stored value and shows the placeholder ("envDefault") on every render.
const LOCALE_OPTIONS = supportedLanguages.map((l) => ({ value: l.code, label: l.label }));

const SERVER_LANG_KEY = "server.language";
const SERVER_REGION_KEY = "server.region";
const METADATA_LANG_KEY = "metadata.language";

function regionOptionsForLocale(locale: string): { value: string; label: string }[] {
  const sv = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf;
  let codes: string[] = [];
  try { codes = typeof sv === "function" ? sv("region") : []; } catch { codes = []; }
  if (codes.length === 0) codes = Array.from(ISO_REGIONS);
  let names: Intl.DisplayNames | null = null;
  try { names = new Intl.DisplayNames([locale || "en"], { type: "region" }); } catch { names = null; }
  return codes
    .filter((c) => /^[A-Z]{2}$/.test(c))
    .map((c) => ({ value: c, label: names?.of(c) ?? c }))
    .sort((a, b) => a.label.localeCompare(b.label, locale || "en"));
}

function LocaleSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t, i18n } = useTranslation();
  const setEntry = useSetConfigEntry();
  const setLocale = useLocaleStore((s) => s.setLocale);

  const serverLang = entries?.find((e) => e.key === SERVER_LANG_KEY)?.value ?? "";
  const serverRegion = entries?.find((e) => e.key === SERVER_REGION_KEY)?.value ?? "";
  const metaLang = entries?.find((e) => e.key === METADATA_LANG_KEY)?.value ?? "";

  function handleServerLang(value: string) {
    setEntry.mutate(
      { key: SERVER_LANG_KEY, value },
      { onSuccess: () => {
          if (value) setLocale(value);
          toast.success(t("admin.serverConfig.languageUpdated"));
        } },
    );
  }
  function handleServerRegion(value: string) {
    setEntry.mutate(
      { key: SERVER_REGION_KEY, value },
      { onSuccess: () => toast.success(t("admin.serverConfig.languageUpdated")) },
    );
  }
  function handleMetaLang(value: string) {
    setEntry.mutate(
      { key: METADATA_LANG_KEY, value },
      { onSuccess: () => toast.success(t("admin.serverConfig.languageUpdated")) },
    );
  }

  const regionOptions = useMemo(() => regionOptionsForLocale(i18n.language || "en"), [i18n.language]);

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Select
          id="server-lang"
          label={t("admin.serverConfig.serverLanguage")}
          value={serverLang}
          onChange={handleServerLang}
          options={[{ label: t("admin.serverConfig.envDefault"), value: "" }, ...LOCALE_OPTIONS]}
        />
        <Select
          id="server-region"
          label={t("admin.serverConfig.serverRegion")}
          value={serverRegion}
          onChange={handleServerRegion}
          options={[
            { label: `${t("admin.serverConfig.envDefault")} (${defaultRegionForLocale(serverLang)})`, value: "" },
            ...regionOptions,
          ]}
        />
        <Select
          id="metadata-lang"
          label={t("admin.serverConfig.metadataLanguage")}
          value={metaLang}
          onChange={handleMetaLang}
          options={[{ label: t("admin.serverConfig.envDefault"), value: "" }, ...LOCALE_OPTIONS]}
        />
      </div>
      {setEntry.isPending && <div><Spinner size="sm" /></div>}
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
          aria-label={t("admin.serverConfig.tableHeaders.key")}
          className="w-full px-2 py-1 rounded text-sm font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
      </td>
      <td className="px-4 py-3">
        <input
          placeholder="value"
          aria-label={t("admin.serverConfig.tableHeaders.value")}
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
  {
    dbKey: "api.tidb_key",
    labelKey: "admin.serverConfig.apiKeys.tidb",
    linkKey: "admin.serverConfig.apiKeys.tidbLink",
    linkUrl: "https://theintrodb.org/account",
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
    <div className="flex flex-col gap-4">
      <p className="text-sm text-[var(--text-muted)]">
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
                      aria-label={isShown ? t("admin.serverConfig.hide") : t("admin.serverConfig.show")}
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
    <div className="space-y-4">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Input label={t("admin.serverConfig.smtpHost")} placeholder="smtp.example.com" value={host} onChange={(e) => setHost(e.target.value)} />
        <Input label={t("admin.serverConfig.port")} type="number" placeholder="587" value={port} onChange={(e) => setPort(e.target.value)} />
        <Input label={t("admin.serverConfig.smtpUsername")} placeholder="user@example.com" value={username} onChange={(e) => setUsername(e.target.value)} />
        <div className="flex flex-col gap-1">
          <Input label={t("admin.serverConfig.smtpPassword")} type={showPassword ? "text" : "password"} placeholder="••••••••" value={password} onChange={(e) => setPassword(e.target.value)} />
          <button type="button" onClick={() => setShowPassword((v) => !v)} className="self-start px-2 rounded text-xs border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text-muted)]" aria-label={showPassword ? t("admin.serverConfig.hide") : t("admin.serverConfig.show")}>
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
const INTRO_PROVIDERS_KEY = "intro_providers.enabled";
const INTRO_TIDB_SUBMIT_KEY = "intro_providers.tidb_submit_enabled";
const ARTWORK_AUTO_DOWNLOAD_KEY = "artwork.auto_download";

// Surfaces the runtime toggle for fire-and-forget artwork download on scan,
// plus a one-shot "retry now" button that backfills any rows still missing
// their local copy. Default ON — without local caching the frontend depends
// on external CDN URLs (Amazon for OMDb), some of which Firefox blocks under
// Opaque Response Blocking.
function ArtworkSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();
  // Default-true semantics: only "false" disables. Missing key or any other
  // value resolves to true to match the server's resolveAutoDownloadDefault.
  const stored = entries?.find((e) => e.key === ARTWORK_AUTO_DOWNLOAD_KEY)?.value;
  const enabled = stored !== "false";

  const retry = useMutation({
    mutationFn: () =>
      // Runs as a pollable background job server-side (a full retry can take
      // minutes); progress is tracked on the Artworks admin page. Here it's a
      // secondary trigger, so we just confirm it started.
      eveyFetch<{ jobId: string; total: number }>({
        url: "/api/v1/admin/artworks/retry-missing",
        method: "POST",
      }),
    onSuccess: () => toast.success(t("admin.serverConfig.artworkRetryStarted")),
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <div className="flex flex-col gap-3">
        <Checkbox
          label={t("admin.serverConfig.artworkAutoDownload")}
          checked={enabled}
          onChange={(v) =>
            setEntry.mutate({ key: ARTWORK_AUTO_DOWNLOAD_KEY, value: v ? "true" : "false" })
          }
          disabled={setEntry.isPending}
        />
        <div className="flex items-center gap-3 mt-1">
          <Button
            variant="secondary"
            onClick={() => retry.mutate()}
            loading={retry.isPending}
            className="text-xs"
          >
            {t("admin.serverConfig.artworkRetryNow")}
          </Button>
          <span className="text-xs text-[var(--text-muted)]">
            {t("admin.serverConfig.artworkRetryDesc")}
          </span>
        </div>
        {setEntry.isPending && <Spinner size="sm" />}
    </div>
  );
}

// IntroDetectionSettingsCard surfaces the runtime knobs of the external
// intro/recap/credits providers (TIDB + IntroDB). The TIDB Bearer token
// itself lives in the API keys card above (api.tidb_key) — it's only
// useful when this toggle is on.
//
// Toggling 'enabled' here writes to server_config but the providers are
// only constructed once at boot — admins must restart the server for the
// change to take effect. The note in the description spells that out.
function IntroDetectionSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const enabled = entries?.find((e) => e.key === INTRO_PROVIDERS_KEY)?.value === "true";
  const tidbKey = entries?.find((e) => e.key === "api.tidb_key")?.value ?? "";
  const tidbSubmitEnabled = entries?.find((e) => e.key === INTRO_TIDB_SUBMIT_KEY)?.value === "true";

  return (
    <div className="flex flex-col gap-3">
        <Checkbox
          label={t("admin.serverConfig.introProvidersEnabled")}
          checked={enabled}
          onChange={(v) => setEntry.mutate({ key: INTRO_PROVIDERS_KEY, value: v ? "true" : "false" })}
          disabled={setEntry.isPending}
        />
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.serverConfig.introProvidersRestart")}
        </p>
        {enabled && (
          <div className="text-xs text-[var(--text-muted)] flex flex-col gap-1 mt-1 pl-1 border-l-2 border-[var(--border)] pl-3">
            <span>
              {t("admin.serverConfig.introProvidersTidbStatus")}{" "}
              {tidbKey ? (
                <span className="text-[var(--accent)]">
                  {t("admin.serverConfig.introProvidersTidbAuthed")}
                </span>
              ) : (
                <span className="text-[var(--color-warning)]">
                  {t("admin.serverConfig.introProvidersTidbAnonymous")}
                </span>
              )}
            </span>
            <span>{t("admin.serverConfig.introProvidersIntrodbStatus")}</span>
            {tidbKey && (
              <div className="mt-2">
                <Checkbox
                  label={t("admin.serverConfig.introProvidersTidbSubmit")}
                  checked={tidbSubmitEnabled}
                  onChange={(v) =>
                    setEntry.mutate({ key: INTRO_TIDB_SUBMIT_KEY, value: v ? "true" : "false" })
                  }
                  disabled={setEntry.isPending}
                />
                <p className="text-xs text-[var(--text-muted)] mt-1 pl-6">
                  {t("admin.serverConfig.introProvidersTidbSubmitDesc")}
                </p>
              </div>
            )}
          </div>
        )}
        {setEntry.isPending && <Spinner size="sm" />}
    </div>
  );
}

const PARSER_CONFIG_KEY = "parser.config";

interface ParserConfigShape {
  extra_qualities?: string[];
  extra_edition_suffixes?: string[];
}

/**
 * Lets the admin extend the filename parser's quality / edition token
 * lists without redeploying. Baseline tokens stay hardcoded in the
 * filesystem package — this UI only manages the EXTENSIONS, persisted as
 * a single JSON blob under the parser.config server_config key. The
 * backend subscriber re-applies them in-process on every save.
 *
 * Quality tokens must be single-word (the parser splits filenames on
 * "." "_" "-" SPACE); edition suffixes can span multiple words because
 * they're stripped via regex before tokenisation.
 */
function ParserTokensCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const raw = entries?.find((e) => e.key === PARSER_CONFIG_KEY)?.value ?? "";
  const initial: ParserConfigShape = useMemo(() => {
    if (!raw) return {};
    try {
      return JSON.parse(raw) as ParserConfigShape;
    } catch {
      return {};
    }
  }, [raw]);

  const initialQualities = (initial.extra_qualities ?? []).join(", ");
  const initialEditions = (initial.extra_edition_suffixes ?? []).join(", ");

  const [qualities, setQualities] = useState(initialQualities);
  const [editions, setEditions] = useState(initialEditions);
  const [dirty, setDirty] = useState(false);

  // Re-sync when the persisted blob changes externally.
  useEffect(() => {
    setQualities(initialQualities);
    setEditions(initialEditions);
    setDirty(false);
  }, [initialQualities, initialEditions]);

  function handleSave() {
    const parse = (s: string) =>
      s.split(",").map((x) => x.trim()).filter((x) => x.length > 0);
    const payload: ParserConfigShape = {
      extra_qualities: parse(qualities),
      extra_edition_suffixes: parse(editions),
    };
    setEntry.mutate({ key: PARSER_CONFIG_KEY, value: JSON.stringify(payload) });
    setDirty(false);
  }

  return (
    <div className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium text-[var(--text)]">
            {t("admin.serverConfig.parserTokensQualities", "Extra quality tokens")}
          </label>
          <textarea
            data-testid="parser-tokens-qualities"
            value={qualities}
            onChange={(e) => { setQualities(e.target.value); setDirty(true); }}
            rows={2}
            className="w-full px-2 py-1 rounded text-sm font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
            placeholder="hdrip, hdr, web-dl"
          />
          <p className="text-xs text-[var(--text-muted)]">
            {t(
              "admin.serverConfig.parserTokensQualitiesHint",
              "Comma-separated, single-word tokens (lowercased). The parser splits filenames on '.', '_', '-' and space.",
            )}
          </p>
        </div>
        <div className="flex flex-col gap-1">
          <label className="text-sm font-medium text-[var(--text)]">
            {t("admin.serverConfig.parserTokensEditions", "Extra edition suffixes")}
          </label>
          <textarea
            data-testid="parser-tokens-editions"
            value={editions}
            onChange={(e) => { setEditions(e.target.value); setDirty(true); }}
            rows={2}
            className="w-full px-2 py-1 rounded text-sm font-mono bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)]"
            placeholder="distributor cut, anniversary edition, imax"
          />
          <p className="text-xs text-[var(--text-muted)]">
            {t(
              "admin.serverConfig.parserTokensEditionsHint",
              "Comma-separated, can span multiple words (regex pre-tokenisation). Stripped from the title before TMDB lookup.",
            )}
          </p>
        </div>
        <div className="flex justify-end pt-1">
          <Button
            onClick={handleSave}
            loading={setEntry.isPending}
            disabled={!dirty}
            data-testid="parser-tokens-save-btn"
          >
            {t("common.save")}
          </Button>
        </div>
    </div>
  );
}

function TrickplaySettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const enabled = entries?.find((e) => e.key === TRICKPLAY_KEY)?.value !== "false";
  const adaptiveCpu = entries?.find((e) => e.key === TRICKPLAY_CPU_KEY)?.value !== "false";

  return (
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
  );
}

interface SettingValue<T> { value: T; default: T; source: "override" | "default"; }
interface ServerSettings {
  serverName: SettingValue<string>;
  hwAccelEnabled: SettingValue<boolean>;
  loginRateLimit: SettingValue<number>;
  globalRateLimit: SettingValue<number>;
  logLevel: SettingValue<string>;
  libraryWatcherEnabled: SettingValue<boolean>;
  periodicScanInterval: SettingValue<string>;
  skipPreviewEnabled: SettingValue<boolean>;
}
const SETTINGS_KEY = ["/api/v1/admin/server-settings"];
const LOG_LEVELS = ["debug", "info", "warn", "error"];

function ServerSettingsCard() {
  const { t } = useTranslation();
  const { data, isLoading } = useQuery<ServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });
  const queryClient = useQueryClient();
  const [serverName, setServerName] = useState("");
  const [loginRate, setLoginRate] = useState("");
  const [globalRate, setGlobalRate] = useState("");
  const [logLevel, setLogLevel] = useState("");
  const [libraryWatcher, setLibraryWatcher] = useState(true);
  const [scanInterval, setScanInterval] = useState("");
  const [skipPreview, setSkipPreview] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (data) {
      setServerName(data.serverName.source === "override" ? String(data.serverName.value) : "");
      setLoginRate(data.loginRateLimit.source === "override" ? String(data.loginRateLimit.value) : "");
      setGlobalRate(data.globalRateLimit.source === "override" ? String(data.globalRateLimit.value) : "");
      setLogLevel(data.logLevel.source === "override" ? String(data.logLevel.value) : "");
      setLibraryWatcher(data.libraryWatcherEnabled.value);
      setScanInterval(data.periodicScanInterval.source === "override" ? String(data.periodicScanInterval.value) : "");
      setSkipPreview(data.skipPreviewEnabled.value);
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
    if (loginRate) body.loginRateLimit = parseInt(loginRate, 10);
    if (globalRate) body.globalRateLimit = parseInt(globalRate, 10);
    if (logLevel) body.logLevel = logLevel;
    body.libraryWatcherEnabled = libraryWatcher;
    if (scanInterval) body.periodicScanInterval = scanInterval;
    body.skipPreviewEnabled = skipPreview;
    saveMutation.mutate(body);
  }

  function ResetBtn({ setting, field }: { setting: SettingValue<unknown>; field: string }) {
    if (setting.source === "default") return <span className="text-xs text-[var(--text-muted)]">{t("admin.serverSettings.usingDefault")} {String(setting.default)}</span>;
    return <button type="button" onClick={() => resetMutation.mutate(field)} className="text-xs text-[var(--accent)] hover:underline cursor-pointer">{t("admin.serverSettings.reset")}</button>;
  }

  if (isLoading) return <div className="flex justify-center py-8"><Spinner size="md" /></div>;
  if (!data) return null;

  return (
    <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-1">
            <Input id="server-name" data-testid="server-name-input" label={t("admin.serverSettings.serverName")} placeholder={data.serverName.default ?? "Evey"} value={serverName} onChange={(e) => { setServerName(e.target.value); setDirty(true); }} />
            <ResetBtn setting={data.serverName} field="serverName" />
          </div>
          <p className="text-xs text-[var(--text-muted)]">{t("admin.serverSettings.transcodeMovedHint", "Transcoding (hardware acceleration, encode calibration, AV1 and concurrency) is configured in Transcoding configuration.")}</p>
          <div className="flex flex-col gap-1">
            <Input label={t("admin.serverSettings.loginRateLimit")} type="number" min={0} placeholder={String(data.loginRateLimit.default ?? 10)} value={loginRate} onChange={(e) => { setLoginRate(e.target.value); setDirty(true); }} />
            <ResetBtn setting={data.loginRateLimit} field="loginRateLimit" />
          </div>
          <div className="flex flex-col gap-1">
            <Input label={t("admin.serverSettings.globalRateLimit")} type="number" min={0} placeholder={String(data.globalRateLimit.default ?? 300)} value={globalRate} onChange={(e) => { setGlobalRate(e.target.value); setDirty(true); }} />
            <ResetBtn setting={data.globalRateLimit} field="globalRateLimit" />
          </div>
          <div className="flex flex-col gap-1">
            <Select
              label={t("admin.serverSettings.logLevel")}
              data-testid="log-level-select"
              value={logLevel}
              onChange={(v) => { setLogLevel(v); setDirty(true); }}
              options={[{ label: `${t("admin.serverConfig.envDefault")} (${data.logLevel.default ?? "info"})`, value: "" }, ...LOG_LEVELS.map((l) => ({ label: l, value: l }))]}
            />
            <ResetBtn setting={data.logLevel} field="logLevel" />
          </div>
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-3">
              <label className="text-sm font-medium text-[var(--text)]">
                {t("admin.serverSettings.libraryWatcher")} <span className="text-xs text-[var(--text-muted)]">{t("admin.serverSettings.restartHint")}</span>
              </label>
              <button type="button" role="switch" aria-checked={libraryWatcher} aria-label={t("admin.serverSettings.libraryWatcher")} data-testid="library-watcher-toggle" onClick={() => { setLibraryWatcher(!libraryWatcher); setDirty(true); }}
                className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors" style={{ backgroundColor: libraryWatcher ? "var(--accent)" : "var(--border)" }}>
                <span className="inline-block h-4 w-4 rounded-full bg-white transition-transform" style={{ transform: libraryWatcher ? "translateX(1.375rem)" : "translateX(0.25rem)" }} />
              </button>
            </div>
            <ResetBtn setting={data.libraryWatcherEnabled} field="libraryWatcherEnabled" />
          </div>
          <div className="flex flex-col gap-1">
            <Input label={`${t("admin.serverSettings.periodicScanInterval")} ${t("admin.serverSettings.restartHint")}`} placeholder={String(data.periodicScanInterval.default ?? "2m")} value={scanInterval} onChange={(e) => { setScanInterval(e.target.value); setDirty(true); }} data-testid="scan-interval-input" />
            <ResetBtn setting={data.periodicScanInterval} field="periodicScanInterval" />
          </div>
          <div className="flex flex-col gap-1">
            <div className="flex items-center gap-3">
              <label className="text-sm font-medium text-[var(--text)]">{t("admin.serverSettings.skipPreview")}</label>
              <InfoTooltip label={t("admin.serverSettings.skipPreview")}>
                {t("admin.serverSettings.skipPreviewInfo")}
              </InfoTooltip>
              <button type="button" role="switch" aria-checked={skipPreview} aria-label={t("admin.serverSettings.skipPreview")} data-testid="skip-preview-toggle" onClick={() => { setSkipPreview(!skipPreview); setDirty(true); }}
                className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors" style={{ backgroundColor: skipPreview ? "var(--accent)" : "var(--border)" }}>
                <span className="inline-block h-4 w-4 rounded-full bg-white transition-transform" style={{ transform: skipPreview ? "translateX(1.375rem)" : "translateX(0.25rem)" }} />
              </button>
            </div>
            <ResetBtn setting={data.skipPreviewEnabled} field="skipPreviewEnabled" />
          </div>
          <div className="flex justify-end pt-2">
            <Button onClick={handleSave} loading={saveMutation.isPending} disabled={!dirty} data-testid="server-config-save-btn">{t("common.save")}</Button>
          </div>
    </div>
  );
}

function ServerConfigPage() {
  const { t } = useTranslation();
  const { data: entries, isLoading } = useServerConfig();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.serverConfig.title")} subtitle={t("admin.serverConfig.subtitle")} />

      <CollapsibleCard
        title={t("admin.serverSettings.title")}
        alwaysOpen
      >
        <ServerSettingsCard />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.languageRegion")}
        subtitle={t("admin.serverConfig.languageRegionDesc")}
      >
        <LocaleSettingsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.feedConfig.title")}
        subtitle={t("admin.serverConfig.feedConfig.subtitle")}
      >
        <FeedConfigCard />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.apiKeys.title")}
        subtitle={t("admin.serverConfig.apiKeys.description")}
      >
        <ApiKeysCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.emailSmtp")}
        subtitle={t("admin.serverConfig.emailSmtpDesc")}
      >
        <SmtpSettingsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.artworkTitle")}
        subtitle={t("admin.serverConfig.artworkDesc")}
        info={t("admin.serverConfig.artworkInfo")}
      >
        <ArtworkSettingsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.trickplayTitle")}
        subtitle={t("admin.serverConfig.trickplayDesc")}
      >
        <TrickplaySettingsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.introProvidersTitle")}
        subtitle={t("admin.serverConfig.introProvidersDesc")}
        info={t("admin.serverConfig.introProvidersInfo")}
      >
        <IntroDetectionSettingsCard entries={entries} />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.serverConfig.parserTokensTitle", "Parser tokens")}
        subtitle={t(
          "admin.serverConfig.parserTokensDesc",
          "Extra release tags the filename parser recognises beyond its baseline list.",
        )}
        info={t("admin.serverConfig.parserTokensInfo")}
      >
        <ParserTokensCard entries={entries} />
      </CollapsibleCard>

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
