import { useState, useEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { useDemoLock } from "@/hooks/useDemoMode";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { SegmentedControl } from "@/components/ui/SegmentedControl";
import { SettingRow } from "@/components/ui/SettingRow";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const SMTP_KEYS = ["smtp.host", "smtp.port", "smtp.username", "smtp.password", "smtp.from", "smtp.tls"] as const;
type SmtpKey = (typeof SMTP_KEYS)[number];

// "None" is translated at render time; the two protocol names are not words.
const TLS_OPTIONS: { label: string; value: string }[] = [
  { label: "None", value: "none" },
  { label: "STARTTLS", value: "starttls" },
  { label: "SSL/TLS", value: "ssl" },
];

export function SmtpSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry({ ownToast: true });

  function val(key: SmtpKey) {
    return entries?.find((e) => e.key === key)?.value ?? "";
  }

  const [host, setHost] = useState(() => val("smtp.host"));
  const [port, setPort] = useState(() => val("smtp.port") || "587");
  const [username, setUsername] = useState(() => val("smtp.username"));
  const [password, setPassword] = useState(() => val("smtp.password"));
  const [from, setFrom] = useState(() => val("smtp.from"));
  const [tls, setTls] = useState(() => val("smtp.tls") || "starttls");
  const [testing, setTesting] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const { lock } = useDemoLock();

  // Re-seed from the config the FIRST time it arrives. useState(() => val())
  // only seeds at mount, so if this card mounts BEFORE the config query
  // resolves, the fields stay empty and Save would overwrite the stored SMTP
  // settings with blanks (data loss). Seed once (ref guard) so subsequent user
  // edits are never clobbered by a refetch.
  const seeded = useRef(false);
  useEffect(() => {
    if (seeded.current || !entries) return;
    seeded.current = true;
    const get = (k: SmtpKey) => entries.find((e) => e.key === k)?.value ?? "";
    setHost(get("smtp.host"));
    setPort(get("smtp.port") || "587");
    setUsername(get("smtp.username"));
    setPassword(get("smtp.password"));
    setFrom(get("smtp.from"));
    setTls(get("smtp.tls") || "starttls");
  }, [entries]);

  // One toast for the six writes: TanStack keeps the callbacks of the last
  // mutate() call only, so per-call callbacks never all fired.
  async function handleSave() {
    const pairs: { key: SmtpKey; value: string }[] = [
      { key: "smtp.host", value: host },
      { key: "smtp.port", value: port },
      { key: "smtp.username", value: username },
      { key: "smtp.password", value: password },
      { key: "smtp.from", value: from },
      { key: "smtp.tls", value: tls },
    ];
    setSubmitting(true);
    const results = await Promise.allSettled(pairs.map((pair) => setEntry.mutateAsync(pair)));
    setSubmitting(false);
    const failed = pairs.filter((_, i) => results[i].status === "rejected").map((p) => p.key);
    if (failed.length === 0) toast.success(t("admin.serverConfig.smtpSaved"));
    else toast.error(t("admin.serverConfig.failedSmtp", { key: failed.join(", ") }));
  }

  // Sends with the SAVED settings (the server re-reads them) to the admin's own address.
  async function handleTest() {
    setTesting(true);
    try {
      const res = await eveyFetch<{ to?: string }>({ url: "/api/v1/admin/smtp/test", method: "POST", data: {} });
      toast.success(t("admin.serverConfig.testEmailSent", { to: res?.to ?? "" }));
    } catch (err) {
      const e = err as { errorCode?: string; message?: string };
      if (e.errorCode === "not_configured") toast.error(t("admin.serverConfig.testEmailNotConfigured"));
      else if (e.errorCode === "no_recipient") toast.error(t("admin.serverConfig.testEmailNoRecipient"));
      else toast.error(t("admin.serverConfig.testEmailFailed", { error: e.message ?? "" }));
    } finally {
      setTesting(false);
    }
  }

  const row = (key: string, labelKey: string, infoKey: string, control: ReactNode) => (
    <SettingRow label={t(labelKey)} htmlFor={key} info={t(infoKey)}>
      {control}
    </SettingRow>
  );
  return (
    <div className="flex flex-col divide-y divide-[var(--border)]">
      <p className="pb-3 text-xs text-[var(--text-muted)]">{t("admin.serverConfig.smtpEnvHint")}</p>
      {row("smtp-host", "admin.serverConfig.smtpHost", "admin.settingsHelp.smtpHost",
        <Input id="smtp-host" className="w-full" placeholder="smtp.example.com" value={host} onChange={(e) => setHost(e.target.value)} />)}
      {row("smtp-port", "admin.serverConfig.port", "admin.settingsHelp.smtpPort",
        <Input id="smtp-port" className="w-full" type="number" placeholder="587" value={port} onChange={(e) => setPort(e.target.value)} />)}
      <SettingRow label={t("admin.serverConfig.tlsMode")} labelId="smtp-tls-label" info={t("admin.settingsHelp.smtpTls")} kind="wide">
        <SegmentedControl
          id="smtp-tls"
          label={t("admin.serverConfig.tlsMode")}
          labelledBy="smtp-tls-label"
          value={tls}
          onChange={setTls}
          options={TLS_OPTIONS.map((o) => ({ ...o, label: o.value === "none" ? t("admin.settingsLayout.tlsNone") : o.label }))}
        />
      </SettingRow>
      {row("smtp-username", "admin.serverConfig.smtpUsername", "admin.settingsHelp.smtpUsername",
        <Input id="smtp-username" className="w-full" placeholder="user@example.com" value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" />)}
      {row("smtp-password", "admin.serverConfig.smtpPassword", "admin.settingsHelp.smtpPassword",
        <Input id="smtp-password" className="w-full" type="password" placeholder="••••••••" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />)}
      {row("smtp-from", "admin.serverConfig.fromAddress", "admin.settingsHelp.smtpFrom",
        <Input id="smtp-from" className="w-full" placeholder="Evey <noreply@example.com>" value={from} onChange={(e) => setFrom(e.target.value)} />)}

      <div className="flex flex-col gap-2 pt-3">
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Button variant="secondary" onClick={handleTest} loading={testing} {...lock}>
            {t("admin.serverConfig.sendTestEmail")}
          </Button>
          <Button onClick={handleSave} loading={submitting}>
            {t("admin.serverConfig.saveSmtp")}
          </Button>
        </div>
        <p className="text-right text-xs text-[var(--text-muted)]">{t("admin.serverConfig.testEmailHint")}</p>
      </div>
    </div>
  );
}
