import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api/src/fetcher";
import { toast, useLocaleStore } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";

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
    <tr style={{ borderBottom: "1px solid var(--border)" }}>
      <td className="px-4 py-3 font-mono text-sm" style={{ color: "var(--text)" }}>
        {entry.key}
      </td>
      <td className="px-4 py-3 text-sm" style={{ color: "var(--text)" }}>
        {editing ? (
          <input
            className="w-full px-2 py-1 rounded text-sm"
            style={{
              backgroundColor: "var(--surface-raised)",
              border: "1px solid var(--border)",
              color: "var(--text)",
            }}
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
      <td className="px-4 py-3 text-xs whitespace-nowrap" style={{ color: "var(--text-muted)" }}>
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
      className="rounded-[var(--radius-lg)] p-5"
      style={{ border: "1px solid var(--border)", backgroundColor: "var(--surface)" }}
    >
      <h2 className="text-base font-semibold mb-1" style={{ color: "var(--text)" }}>
        {t("admin.serverConfig.languageRegion")}
      </h2>
      <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
        {t("admin.serverConfig.languageRegionDesc")}
      </p>
      <div className="flex items-center gap-3">
        <label className="text-sm font-medium" style={{ color: "var(--text)" }} htmlFor="metadata-lang">
          {t("admin.serverConfig.metadataLanguage")}
        </label>
        <select
          id="metadata-lang"
          value={current}
          onChange={(e) => handleChange(e.target.value)}
          className="px-3 py-1.5 rounded text-sm"
          style={{
            backgroundColor: "var(--surface-raised)",
            border: "1px solid var(--border)",
            color: "var(--text)",
          }}
        >
          <option value="">{t("admin.serverConfig.envDefault")}</option>
          {LOCALE_OPTIONS.map((opt) => (
            <option key={opt.value} value={opt.value}>
              {opt.label}
            </option>
          ))}
        </select>
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
    <tr style={{ borderBottom: "1px solid var(--border)", backgroundColor: "var(--surface)" }}>
      <td className="px-4 py-3">
        <input
          placeholder="key"
          className="w-full px-2 py-1 rounded text-sm font-mono"
          style={{ backgroundColor: "var(--surface-raised)", border: "1px solid var(--border)", color: "var(--text)" }}
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
      </td>
      <td className="px-4 py-3">
        <input
          placeholder="value"
          className="w-full px-2 py-1 rounded text-sm"
          style={{ backgroundColor: "var(--surface-raised)", border: "1px solid var(--border)", color: "var(--text)" }}
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

const inputStyle = {
  backgroundColor: "var(--surface-raised)",
  border: "1px solid var(--border)",
  color: "var(--text)",
} as const;

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

  const labelClass = "text-sm font-medium" as const;
  const fieldStyle = { color: "var(--text)" } as const;

  return (
    <div
      className="rounded-[var(--radius-lg)] p-5"
      style={{ border: "1px solid var(--border)", backgroundColor: "var(--surface)" }}
    >
      <h2 className="text-base font-semibold mb-1" style={{ color: "var(--text)" }}>
        {t("admin.serverConfig.emailSmtp")}
      </h2>
      <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
        {t("admin.serverConfig.emailSmtpDesc")}
      </p>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mb-4">
        <div className="flex flex-col gap-1">
          <label className={labelClass} style={fieldStyle}>{t("admin.serverConfig.smtpHost")}</label>
          <input className="px-3 py-1.5 rounded text-sm w-full" style={inputStyle} placeholder="smtp.example.com" value={host} onChange={(e) => setHost(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1">
          <label className={labelClass} style={fieldStyle}>{t("admin.serverConfig.port")}</label>
          <input className="px-3 py-1.5 rounded text-sm w-full" style={inputStyle} type="number" placeholder="587" value={port} onChange={(e) => setPort(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1">
          <label className={labelClass} style={fieldStyle}>{t("admin.serverConfig.smtpUsername")}</label>
          <input className="px-3 py-1.5 rounded text-sm w-full" style={inputStyle} placeholder="user@example.com" value={username} onChange={(e) => setUsername(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1">
          <label className={labelClass} style={fieldStyle}>{t("admin.serverConfig.smtpPassword")}</label>
          <div className="flex gap-2">
            <input className="px-3 py-1.5 rounded text-sm flex-1" style={inputStyle} type={showPassword ? "text" : "password"} placeholder="\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022" value={password} onChange={(e) => setPassword(e.target.value)} />
            <button type="button" onClick={() => setShowPassword((v) => !v)} className="px-2 rounded text-xs" style={{ border: "1px solid var(--border)", backgroundColor: "var(--surface-raised)", color: "var(--text-muted)" }}>
              {showPassword ? t("admin.serverConfig.hide") : t("admin.serverConfig.show")}
            </button>
          </div>
        </div>
        <div className="flex flex-col gap-1">
          <label className={labelClass} style={fieldStyle}>{t("admin.serverConfig.fromAddress")}</label>
          <input className="px-3 py-1.5 rounded text-sm w-full" style={inputStyle} placeholder="Evey <noreply@example.com>" value={from} onChange={(e) => setFrom(e.target.value)} />
        </div>
        <div className="flex flex-col gap-1">
          <label className={labelClass} style={fieldStyle}>{t("admin.serverConfig.tlsMode")}</label>
          <select className="px-3 py-1.5 rounded text-sm w-full" style={inputStyle} value={tls} onChange={(e) => setTls(e.target.value)}>
            {TLS_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>{opt.label}</option>
            ))}
          </select>
        </div>
      </div>

      <Button onClick={handleSave} loading={setEntry.isPending}>
        {t("admin.serverConfig.saveSmtp")}
      </Button>
    </div>
  );
}

function ServerConfigPage() {
  const { t } = useTranslation();
  const { data: entries, isLoading } = useServerConfig();

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>{t("admin.serverConfig.title")}</h1>
        <p className="text-sm mt-1" style={{ color: "var(--text-muted)" }}>
          {t("admin.serverConfig.subtitle")}
        </p>
      </div>

      <LocaleSettingsCard entries={entries} />

      <SmtpSettingsCard entries={entries} />

      {isLoading ? (
        <div className="flex justify-center py-16"><Spinner size="lg" /></div>
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)]" style={{ border: "1px solid var(--border)" }}>
          <table className="w-full text-sm">
            <thead>
              <tr style={{ backgroundColor: "var(--surface)", borderBottom: "1px solid var(--border)" }}>
                {[t("admin.serverConfig.tableHeaders.key"), t("admin.serverConfig.tableHeaders.value"), t("admin.serverConfig.tableHeaders.updated"), t("admin.serverConfig.tableHeaders.actions")].map((h) => (
                  <th key={h} className="px-4 py-2 text-left font-medium" style={{ color: "var(--text-muted)" }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {entries && entries.length === 0 ? (
                <tr>
                  <td colSpan={4}>
                    <EmptyState icon="library" title={t("admin.serverConfig.noEntries")} description={t("admin.serverConfig.noEntriesDesc")} />
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
