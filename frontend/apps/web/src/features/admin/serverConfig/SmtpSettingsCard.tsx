import { useState, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const SMTP_KEYS = ["smtp.host", "smtp.port", "smtp.username", "smtp.password", "smtp.from", "smtp.tls"] as const;
type SmtpKey = (typeof SMTP_KEYS)[number];

const TLS_OPTIONS: { label: string; value: string }[] = [
  { label: "None", value: "none" },
  { label: "STARTTLS", value: "starttls" },
  { label: "SSL/TLS", value: "ssl" },
];

export function SmtpSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
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
