import { useEffect, useState, type ComponentProps } from "react";
import { View, Text, TextInput, Pressable, Switch, ActivityIndicator, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, apiErrorMessage, useLibraryServiceListLibraries } from "@evey/api";
import { toast, useDemoStore, demoLocks } from "@evey/store";
import { useTheme } from "../../../providers/ThemeProvider";
import { SettingRow } from "../settings/SettingRow";
import { SaveButton } from "../settings/SaveButton";
import { ChipSelect } from "./ChipSelect";
import { LoadFailed } from "../../LoadFailed";

// SsoSettingsCard configures OpenID Connect sign-in, same routes and keys as
// the web card: the server applies a save live and only returns the secret
// as the mask, which is sent back untouched to keep it.

export type SsoStatus = {
  state: "disabled" | "incomplete" | "ok" | "error";
  error?: string;
  checkedAt?: string;
  endSessionEndpoint?: string;
};

export type SsoSettings = {
  enabled: boolean;
  issuer: string;
  clientId: string;
  clientSecret: string;
  allowedEmailDomains: string[];
  buttonLabel: string;
  linkByEmail: boolean;
  allowSignup: boolean;
  allowSignupDefault: boolean;
  defaultLibraryIds: string[];
  redirectUri: string;
  postLogoutRedirectUri: string;
  source: "panel" | "env" | "none";
  status: SsoStatus;
};

const SSO_URL = "/api/v1/admin/sso";
export const SSO_QUERY_KEY = ["admin", "sso"];

export function SsoSettingsCard() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const saveLocked = useDemoStore((st) => demoLocks(st, "PUT /api/v1/admin/sso"));
  const testLocked = useDemoStore((st) => demoLocks(st, "POST /api/v1/admin/sso/test"));
  const { data, isLoading, isError, refetch } = useQuery<SsoSettings>({
    queryKey: SSO_QUERY_KEY,
    queryFn: () => eveyFetch<SsoSettings>({ url: SSO_URL, method: "GET" }),
  });
  const { data: librariesData } = useLibraryServiceListLibraries();
  const libraries = librariesData?.libraries ?? [];

  const [enabled, setEnabled] = useState(false);
  const [issuer, setIssuer] = useState("");
  const [clientId, setClientId] = useState("");
  const [clientSecret, setClientSecret] = useState("");
  const [domains, setDomains] = useState("");
  const [buttonLabel, setButtonLabel] = useState("");
  const [linkByEmail, setLinkByEmail] = useState(true);
  const [allowSignup, setAllowSignup] = useState(false);
  const [restrictLibraries, setRestrictLibraries] = useState(false);
  const [libraryIds, setLibraryIds] = useState<string[]>([]);
  const [testResult, setTestResult] = useState<{ ok: boolean; error?: string } | null>(null);

  // Seed from the server; the refetch after Save re-seeds the normalised values.
  useEffect(() => {
    if (!data) return;
    setEnabled(data.enabled);
    setIssuer(data.issuer);
    setClientId(data.clientId);
    setClientSecret(data.clientSecret);
    setDomains(data.allowedEmailDomains.join(", "));
    setButtonLabel(data.buttonLabel);
    setLinkByEmail(data.linkByEmail);
    setAllowSignup(data.allowSignup);
    setRestrictLibraries(data.defaultLibraryIds.length > 0);
    setLibraryIds(data.defaultLibraryIds);
  }, [data]);

  // The payload is built at press time so no stale render's values are sent.
  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) => eveyFetch<SsoSettings>({ url: SSO_URL, method: "PUT", data: body }),
    onSuccess: (next) => {
      qc.setQueryData(SSO_QUERY_KEY, next);
      // The login screen caches whether SSO is offered.
      void qc.invalidateQueries({ queryKey: ["oidc-config"] });
      setTestResult(null);
      toast.success(t("admin.sso.saved"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.sso.saveFailed"))),
  });

  const test = useMutation({
    mutationFn: (testIssuer: string) =>
      eveyFetch<{ ok: boolean; error?: string }>({ url: `${SSO_URL}/test`, method: "POST", data: { issuer: testIssuer } }),
    onSuccess: (res) => {
      setTestResult(res);
      void qc.invalidateQueries({ queryKey: SSO_QUERY_KEY });
    },
    onError: (err: Error) => setTestResult({ ok: false, error: apiErrorMessage(err, t, t("common.error")) }),
  });

  const s = styles(theme);

  if (isError && !data) return <LoadFailed inline onRetry={() => void refetch()} />;
  if (isLoading || !data) {
    return (
      <View style={{ paddingVertical: 24, alignItems: "center" }}>
        <ActivityIndicator size="small" color={theme.colors.accent} />
      </View>
    );
  }

  const selectionMissing = restrictLibraries && libraryIds.length === 0;
  const status = data.status;
  const checkedAt = status.checkedAt ? new Date(status.checkedAt).toLocaleString(i18n.language) : "";
  const statusColor = {
    ok: theme.colors.success,
    error: theme.colors.danger,
    incomplete: theme.colors.warning,
    disabled: theme.colors.textMuted,
  }[status.state];
  const switchColors = { trackColor: { false: theme.colors.border, true: theme.colors.accent } };

  const field = (
    label: string,
    info: string,
    value: string,
    onChange: (v: string) => void,
    id: string,
    opts: Partial<ComponentProps<typeof TextInput>> = {},
    kind: "field" | "wide" = "field",
  ) => (
    <SettingRow label={label} info={info} kind={kind} testID={`setting-row-${id}`}>
      <TextInput
        value={value}
        onChangeText={onChange}
        style={s.input}
        placeholderTextColor={theme.colors.textMuted}
        accessibilityLabel={label}
        autoCapitalize="none"
        autoCorrect={false}
        {...opts}
      />
    </SettingRow>
  );

  const switchRow = (label: string, info: string, value: boolean, onChange: (v: boolean) => void, testID: string, id: string) => (
    <SettingRow label={label} info={info} kind="toggle" testID={`setting-row-${id}`}>
      <Switch value={value} onValueChange={onChange} accessibilityLabel={label} testID={testID} {...switchColors} />
    </SettingRow>
  );

  const uri = (label: string, value: string) => (
    <View style={{ marginTop: 10 }}>
      <Text style={s.label}>{label}</Text>
      {/* No clipboard module in the app: the address is selectable instead. */}
      <Text selectable style={s.code}>
        {value}
      </Text>
    </View>
  );

  return (
    <View testID="sso-settings">
      <View style={{ flexDirection: "row", flexWrap: "wrap", alignItems: "center", gap: 8 }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{t("admin.sso.status")}</Text>
        <View style={[s.badge, { borderColor: statusColor }]}>
          <Text style={{ color: statusColor, fontSize: 12, fontWeight: "600" }} testID="sso-status">
            {t(`admin.sso.state.${status.state}`)}
          </Text>
        </View>
        {checkedAt ? <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("admin.sso.lastChecked", { time: checkedAt })}</Text> : null}
      </View>
      {status.error ? (
        <Text style={[s.hint, { color: theme.colors.danger }]} accessibilityRole="alert" testID="sso-status-error">
          {status.error}
        </Text>
      ) : null}
      {data.source === "env" ? <Text style={s.hint}>{t("admin.sso.sourceEnv")}</Text> : null}

      <View style={s.divider} />
      {switchRow(t("admin.sso.enabled"), t("admin.settingsHelp.ssoEnabled"), enabled, setEnabled, "sso-enabled", "sso-enabled")}
      {field(t("admin.sso.issuer"), t("admin.sso.issuerHint"), issuer, setIssuer, "sso-issuer", { placeholder: "https://auth.example.com/realms/home", keyboardType: "url" }, "wide")}
      {field(t("admin.sso.clientId"), t("admin.settingsHelp.ssoClientId"), clientId, setClientId, "sso-client-id")}
      {field(t("admin.sso.clientSecret"), t("admin.sso.clientSecretHint"), clientSecret, setClientSecret, "sso-client-secret", { secureTextEntry: true })}
      {field(t("admin.sso.allowedDomains"), t("admin.sso.allowedDomainsHint"), domains, setDomains, "sso-domains", { placeholder: "example.com" })}
      {field(t("admin.sso.buttonLabel"), t("admin.sso.buttonLabelHint"), buttonLabel, setButtonLabel, "sso-label", {
        placeholder: t("auth.login.signInWithSSO"),
        maxLength: 64,
        autoCapitalize: "sentences",
      })}
      {switchRow(t("admin.sso.linkByEmail"), t("admin.sso.linkByEmailHint"), linkByEmail, setLinkByEmail, "sso-link", "sso-link")}
      {switchRow(t("admin.sso.allowSignup"), t("admin.sso.allowSignupHint"), allowSignup, setAllowSignup, "sso-signup", "sso-signup")}
      {allowSignup ? (
        <SettingRow label={t("admin.sso.defaultAccess")} info={t("admin.settingsHelp.ssoDefaultAccess")} kind="wide" testID="setting-row-sso-default-access">
          <ChipSelect
            label={t("admin.sso.defaultAccess")}
            value={restrictLibraries ? "selected" : "all"}
            onChange={(v) => setRestrictLibraries(v === "selected")}
            options={[
              { value: "all", label: t("admin.sso.defaultAccessAll") },
              { value: "selected", label: t("admin.sso.defaultAccessSelected") },
            ]}
          />
          {restrictLibraries ? (
            <View style={s.libraryBox}>
              {libraries.map((lib) => {
                const id = lib.id ?? "";
                const checked = libraryIds.includes(id);
                return (
                  <Pressable
                    key={id}
                    onPress={() => setLibraryIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]))}
                    accessibilityRole="checkbox"
                    accessibilityState={{ checked }}
                    accessibilityLabel={lib.name ?? id}
                    style={s.libraryRow}
                  >
                    <View style={[s.checkbox, { backgroundColor: checked ? theme.colors.accent : "transparent", borderColor: checked ? theme.colors.accent : theme.colors.border }]}>
                      {checked ? <Text style={{ color: theme.colors.onAccent, fontSize: 12, fontWeight: "700" }}>✓</Text> : null}
                    </View>
                    <Text style={{ color: theme.colors.text, fontSize: 14, flex: 1 }} numberOfLines={1}>
                      {lib.name ?? id}
                    </Text>
                  </Pressable>
                );
              })}
            </View>
          ) : null}
          {selectionMissing ? <Text style={[s.hint, { color: theme.colors.danger }]}>{t("admin.sso.defaultAccessNone")}</Text> : null}
        </SettingRow>
      ) : null}

      <View style={s.divider} />
      <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }}>{t("admin.sso.registerTitle")}</Text>
      {uri(t("admin.sso.redirectUri"), data.redirectUri)}
      {uri(t("admin.sso.postLogoutUri"), data.postLogoutRedirectUri)}
      <Text style={s.hint}>{t("admin.sso.registerHint")}</Text>
      {status.state === "ok" ? (
        <Text style={s.hint}>{status.endSessionEndpoint ? t("admin.sso.logoutSupported") : t("admin.sso.logoutUnsupported")}</Text>
      ) : null}

      {testResult ? (
        <Text style={[s.hint, { marginTop: 16, color: testResult.ok ? theme.colors.textMuted : theme.colors.danger }]} testID="sso-test-result">
          {testResult.ok ? t("admin.sso.testOk") : t("admin.sso.testFailed", { error: testResult.error ?? "" })}
        </Text>
      ) : null}

      <SaveButton
        onPress={() =>
          save.mutate({
            enabled,
            issuer,
            clientId,
            clientSecret,
            allowedEmailDomains: domains.split(",").map((d) => d.trim()).filter(Boolean),
            buttonLabel,
            linkByEmail,
            allowSignup,
            defaultLibraryIds: restrictLibraries ? libraryIds : [],
          })
        }
        disabled={selectionMissing || saveLocked}
        loading={save.isPending}
        label={t("admin.sso.save")}
        testID="sso-save"
      />

      <Pressable
        onPress={() => test.mutate(issuer)}
        disabled={!issuer.trim() || test.isPending || testLocked}
        accessibilityHint={testLocked ? t("common.demo.locked") : undefined}
        style={[s.testBtn, { opacity: !issuer.trim() || test.isPending || testLocked ? 0.5 : 1 }]}
        accessibilityRole="button"
        testID="sso-test"
      >
        {test.isPending ? (
          <ActivityIndicator size="small" color={theme.colors.text} />
        ) : (
          <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 15 }}>{t("admin.sso.test")}</Text>
        )}
      </Pressable>
    </View>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    label: { color: t.colors.textMuted, fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginTop: 16, marginBottom: 6 },
    input: { backgroundColor: t.colors.surfaceRaised, borderWidth: StyleSheet.hairlineWidth, borderColor: t.colors.border, color: t.colors.text, borderRadius: t.radii.md, paddingHorizontal: 14, paddingVertical: 12, fontSize: 15 },
    hint: { color: t.colors.textMuted, fontSize: 12, marginTop: 6 },
    badge: { borderWidth: StyleSheet.hairlineWidth, borderRadius: 999, paddingHorizontal: 8, paddingVertical: 2 },
    divider: { height: StyleSheet.hairlineWidth, backgroundColor: t.colors.border, marginTop: 20, marginBottom: 4 },
    code: { backgroundColor: t.colors.surfaceRaised, color: t.colors.text, borderRadius: 6, paddingHorizontal: 8, paddingVertical: 6, fontSize: 12, fontFamily: "monospace" },
    libraryBox: { marginTop: 10, borderWidth: StyleSheet.hairlineWidth, borderColor: t.colors.border, borderRadius: t.radii.md, padding: 6, backgroundColor: t.colors.surfaceRaised },
    libraryRow: { flexDirection: "row", alignItems: "center", gap: 10, paddingHorizontal: 8, paddingVertical: 8 },
    checkbox: { width: 20, height: 20, borderRadius: 4, borderWidth: 1.5, alignItems: "center", justifyContent: "center" },
    testBtn: { borderRadius: t.radii.md, paddingVertical: 14, alignItems: "center", marginTop: 12, borderWidth: StyleSheet.hairlineWidth, borderColor: t.colors.border, backgroundColor: t.colors.surfaceRaised },
  });
