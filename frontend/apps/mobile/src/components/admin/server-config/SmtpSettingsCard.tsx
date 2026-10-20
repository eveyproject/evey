import { useState, useEffect, useRef, useCallback } from "react";
import { View, Text, TextInput, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { eveyFetch } from "@evey/api";
import { useTheme } from "../../../providers/ThemeProvider";
import { SettingRow } from "../settings/SettingRow";
import { SaveButton } from "../settings/SaveButton";
import { ChipSelect } from "./ChipSelect";
import type { ConfigEntry, SaveEntryAsync } from "./types";

const SMTP_KEYS = ["smtp.host", "smtp.port", "smtp.username", "smtp.password", "smtp.from", "smtp.tls"] as const;
type SmtpKey = (typeof SMTP_KEYS)[number];

const TLS_OPTIONS = [
  { value: "none", label: "None" },
  { value: "starttls", label: "STARTTLS" },
  { value: "ssl", label: "SSL/TLS" },
];

interface SmtpSettingsCardProps {
  entries: ConfigEntry[] | undefined;
  saveEntry: SaveEntryAsync;
  saving: boolean;
}

export function SmtpSettingsCard({ entries, saveEntry, saving }: SmtpSettingsCardProps) {
  const theme = useTheme();
  const { t } = useTranslation();

  function val(entryList: ConfigEntry[] | undefined, key: SmtpKey): string {
    return entryList?.find((e) => e.key === key)?.value ?? "";
  }

  const [host, setHost] = useState(() => val(entries, "smtp.host"));
  const [port, setPort] = useState(() => val(entries, "smtp.port") || "587");
  const [username, setUsername] = useState(() => val(entries, "smtp.username"));
  const [password, setPassword] = useState(() => val(entries, "smtp.password"));
  const [from, setFrom] = useState(() => val(entries, "smtp.from"));
  const [tls, setTls] = useState(() => val(entries, "smtp.tls") || "starttls");
  const [showPassword, setShowPassword] = useState(false);
  const [testing, setTesting] = useState(false);
  const [submitting, setSubmitting] = useState(false);

  // Re-seed from the config the FIRST time it arrives (mirrors the web card):
  // useState(() => val()) only seeds at mount, so a card that mounts before
  // the entries query resolves would otherwise stay empty and Save would
  // overwrite the stored SMTP settings with blanks.
  const seeded = useRef(false);
  useEffect(() => {
    if (seeded.current || !entries) return;
    seeded.current = true;
    setHost(val(entries, "smtp.host"));
    setPort(val(entries, "smtp.port") || "587");
    setUsername(val(entries, "smtp.username"));
    setPassword(val(entries, "smtp.password"));
    setFrom(val(entries, "smtp.from"));
    setTls(val(entries, "smtp.tls") || "starttls");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [entries]);

  // One toast for the six writes: TanStack keeps the callbacks of the last
  // mutate() call only, so per-call callbacks never all fired.
  const handleSave = useCallback(async () => {
    const pairs: { key: SmtpKey; value: string }[] = [
      { key: "smtp.host", value: host },
      { key: "smtp.port", value: port },
      { key: "smtp.username", value: username },
      { key: "smtp.password", value: password },
      { key: "smtp.from", value: from },
      { key: "smtp.tls", value: tls },
    ];
    setSubmitting(true);
    const results = await Promise.allSettled(pairs.map((pair) => saveEntry(pair)));
    setSubmitting(false);
    const failed = pairs.filter((_, i) => results[i].status === "rejected").map((p) => p.key);
    if (failed.length === 0) toast.success(t("admin.serverConfig.smtpSaved", "SMTP settings saved"));
    else toast.error(t("admin.serverConfig.failedSmtp", "Failed to save {{key}}", { key: failed.join(", ") }));
  }, [host, port, username, password, from, tls, saveEntry, t]);

  // Sends with the SAVED settings (the server re-reads them) to the admin's own
  // address - same route and messages as the web card.
  const handleTest = useCallback(async () => {
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
  }, [t]);

  const s = styles(theme);
  const busy = saving || submitting;

  const input = (label: string, value: string, onChangeText: (v: string) => void, placeholder: string, extra: Record<string, unknown> = {}) => (
    <TextInput
      accessibilityLabel={label}
      value={value}
      onChangeText={onChangeText}
      style={s.input}
      placeholderTextColor={theme.colors.textMuted}
      placeholder={placeholder}
      autoCapitalize="none"
      {...extra}
    />
  );
  const tlsLabel = t("admin.serverConfig.tlsMode", "TLS Mode");
  const passwordLabel = t("admin.serverConfig.smtpPassword", "Password");
  const showLabel = showPassword ? t("admin.serverConfig.hide", "Hide") : t("admin.serverConfig.show", "Show");

  return (
    <View>
      <Text style={s.hint}>{t("admin.serverConfig.smtpEnvHint")}</Text>

      <SettingRow label={t("admin.serverConfig.smtpHost", "SMTP Host")} info={t("admin.settingsHelp.smtpHost")} testID="setting-row-smtp-host">
        {input(t("admin.serverConfig.smtpHost", "SMTP Host"), host, setHost, "smtp.example.com")}
      </SettingRow>

      <SettingRow label={t("admin.serverConfig.port", "Port")} info={t("admin.settingsHelp.smtpPort")} testID="setting-row-smtp-port">
        {input(t("admin.serverConfig.port", "Port"), port, setPort, "587", { keyboardType: "numeric" })}
      </SettingRow>

      <SettingRow label={tlsLabel} info={t("admin.settingsHelp.smtpTls")} kind="wide" testID="setting-row-smtp-tls">
        <ChipSelect
          label={tlsLabel}
          value={tls}
          onChange={setTls}
          options={TLS_OPTIONS.map((o) => (o.value === "none" ? { ...o, label: t("admin.settingsLayout.tlsNone", o.label) } : o))}
        />
      </SettingRow>

      <SettingRow label={t("admin.serverConfig.smtpUsername", "Username")} info={t("admin.settingsHelp.smtpUsername")} testID="setting-row-smtp-username">
        {input(t("admin.serverConfig.smtpUsername", "Username"), username, setUsername, "user@example.com")}
      </SettingRow>

      <SettingRow label={passwordLabel} info={t("admin.settingsHelp.smtpPassword")} testID="setting-row-smtp-password">
        <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
          <TextInput
            value={password}
            onChangeText={setPassword}
            accessibilityLabel={passwordLabel}
            style={[s.input, { flex: 1 }]}
            placeholderTextColor={theme.colors.textMuted}
            placeholder="••••••••"
            secureTextEntry={!showPassword}
            autoCapitalize="none"
            autoCorrect={false}
          />
          <Pressable onPress={() => setShowPassword((v) => !v)} style={s.showBtn} hitSlop={8} accessibilityRole="button" accessibilityLabel={showLabel}>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontWeight: "600" }}>{showLabel}</Text>
          </Pressable>
        </View>
      </SettingRow>

      <SettingRow label={t("admin.serverConfig.fromAddress", "From Address")} info={t("admin.settingsHelp.smtpFrom")} testID="setting-row-smtp-from">
        {input(t("admin.serverConfig.fromAddress", "From Address"), from, setFrom, "Evey <noreply@example.com>")}
      </SettingRow>

      <View style={{ marginTop: 12 }}>
        <SaveButton onPress={handleSave} loading={busy} label={t("admin.serverConfig.saveSmtp", "Save SMTP Settings")} testID="smtp-save-btn" />
      </View>

      <Pressable
        onPress={handleTest}
        disabled={testing}
        style={[s.testBtn, { opacity: testing ? 0.5 : 1 }]}
        accessibilityRole="button"
        testID="smtp-test-btn"
      >
        {testing ? (
          <ActivityIndicator size="small" color={theme.colors.text} />
        ) : (
          <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 15 }}>
            {t("admin.serverConfig.sendTestEmail", "Send test email")}
          </Text>
        )}
      </Pressable>
      <Text style={s.hint}>{t("admin.serverConfig.testEmailHint")}</Text>
    </View>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    input: { backgroundColor: t.colors.surface, color: t.colors.text, borderRadius: t.radii.md, paddingHorizontal: 14, paddingVertical: 12, fontSize: 15 },
    showBtn: { paddingHorizontal: 10, paddingVertical: 10, borderRadius: 8, borderWidth: StyleSheet.hairlineWidth, borderColor: t.colors.border, backgroundColor: t.colors.surfaceRaised },
    testBtn: { borderRadius: t.radii.md, paddingVertical: 14, alignItems: "center", marginTop: 12, borderWidth: StyleSheet.hairlineWidth, borderColor: t.colors.border, backgroundColor: t.colors.surfaceRaised },
    hint: { color: t.colors.textMuted, fontSize: 12, marginTop: 8, marginBottom: 4 },
  });
