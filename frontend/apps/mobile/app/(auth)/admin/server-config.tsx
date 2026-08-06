import { useState, useEffect, useCallback } from "react";
import { View, Text, ScrollView, TextInput, Pressable, ActivityIndicator, Switch, RefreshControl, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useAdminSystemServiceGetServerConfig,
  useAdminSystemServiceUpdateServerConfig,
  getAdminSystemServiceGetServerConfigQueryKey,
  eveyFetch,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LocaleSettingsCard } from "../../../src/components/admin/server-config/LocaleSettingsCard";
import { MemoryProfileCard } from "../../../src/components/admin/server-config/MemoryProfileCard";
import { MkvgoCard } from "../../../src/components/admin/server-config/MkvgoCard";
import { SmtpSettingsCard } from "../../../src/components/admin/server-config/SmtpSettingsCard";
import { BackgroundJobsCard } from "../../../src/components/admin/server-config/BackgroundJobsCard";
import { ParserTokensCard } from "../../../src/components/admin/server-config/ParserTokensCard";
import type { ConfigEntry } from "../../../src/components/admin/server-config/types";

interface SettingValue<T> {
  value: T;
  default: T;
  source: "override" | "default";
}

interface ServerSettings {
  serverName: SettingValue<string>;
  maxConcurrentTranscodes: SettingValue<number>;
  hwAccelEnabled: SettingValue<boolean>;
  loginRateLimit: SettingValue<number>;
  globalRateLimit: SettingValue<number>;
  logLevel: SettingValue<string>;
  libraryWatcherEnabled: SettingValue<boolean>;
  periodicScanInterval: SettingValue<string>;
  skipPreviewEnabled: SettingValue<boolean>;
  auditRetentionDays: SettingValue<number>;
  metricsRetentionDays: SettingValue<number>;
  qoeRetentionDays: SettingValue<number>;
  playSessionRetentionDays: SettingValue<number>;
  transcodeCacheMaxGB: SettingValue<number>;
}

const SETTINGS_KEY = ["admin", "server-settings"];
const SERVER_CONFIG_ENTRIES_KEY = ["admin", "server-config-entries"];

const API_KEY_FIELDS = [
  { dbKey: "api.tmdb_key", labelKey: "admin.serverConfig.apiKeys.tmdb", labelFallback: "TMDB Key", hasDefault: true },
  { dbKey: "api.omdb_key", labelKey: "admin.serverConfig.apiKeys.omdb", labelFallback: "OMDb Key", hasDefault: false },
  { dbKey: "api.opensubtitles_key", labelKey: "admin.serverConfig.apiKeys.opensubtitles", labelFallback: "OpenSubtitles Key", hasDefault: false },
  { dbKey: "api.tidb_key", labelKey: "admin.serverConfig.apiKeys.tidb", labelFallback: "TIDB Key", hasDefault: false },
] as const;

const LOG_LEVELS = ["debug", "info", "warn", "error"] as const;

export default function AdminServerConfigScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);

  const { data: config, isLoading: configLoading } = useAdminSystemServiceGetServerConfig();
  const [serverName, setServerName] = useState("");
  const [baseUrl, setBaseUrl] = useState("");

  useEffect(() => {
    if (config?.values) {
      setServerName(config.values["server_name"] ?? "");
      setBaseUrl(config.values["base_url"] ?? "");
    }
  }, [config]);

  const updateConfig = useAdminSystemServiceUpdateServerConfig({
    mutation: {
      onSuccess: async () => {
        toast.success(t("admin.serverConfig.configUpdated", "Config saved"));
        await qc.invalidateQueries({ queryKey: getAdminSystemServiceGetServerConfigQueryKey() });
      },
      onError: () => toast.error(t("admin.serverConfig.failedUpdate", "Failed to save")),
    },
  });

  const handleSaveBasic = useCallback(async () => {
    try {
      await updateConfig.mutateAsync({ data: { values: { server_name: serverName.trim(), base_url: baseUrl.trim() } } });
    } catch {
      // Not silent: updateConfig's mutation.onError above already surfaces a
      // toast. This just swallows the rejection mutateAsync re-throws so it
      // doesn't also warn as an unhandled promise.
    }
  }, [serverName, baseUrl, updateConfig]);

  const { data: settings, isLoading: settingsLoading, refetch } = useQuery<ServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });

  const [maxTranscodes, setMaxTranscodes] = useState("");
  const [hwAccel, setHwAccel] = useState(false);
  const [loginRate, setLoginRate] = useState("");
  const [globalRate, setGlobalRate] = useState("");
  const [logLevel, setLogLevel] = useState("");
  const [libraryWatcher, setLibraryWatcher] = useState(true);
  const [scanInterval, setScanInterval] = useState("");
  const [skipPreview, setSkipPreview] = useState(false);
  const [auditDays, setAuditDays] = useState("");
  const [metricsDays, setMetricsDays] = useState("");
  const [qoeDays, setQoeDays] = useState("");
  const [sessionDays, setSessionDays] = useState("");
  const [cacheGB, setCacheGB] = useState("");
  const [settingsDirty, setSettingsDirty] = useState(false);

  useEffect(() => {
    if (settings) {
      setMaxTranscodes(settings.maxConcurrentTranscodes.source === "override" ? String(settings.maxConcurrentTranscodes.value) : "");
      setHwAccel(settings.hwAccelEnabled.value);
      setLoginRate(settings.loginRateLimit.source === "override" ? String(settings.loginRateLimit.value) : "");
      setGlobalRate(settings.globalRateLimit.source === "override" ? String(settings.globalRateLimit.value) : "");
      setLogLevel(settings.logLevel?.source === "override" ? String(settings.logLevel.value) : "");
      setLibraryWatcher(settings.libraryWatcherEnabled?.value ?? true);
      setScanInterval(settings.periodicScanInterval?.source === "override" ? String(settings.periodicScanInterval.value) : "");
      setSkipPreview(settings.skipPreviewEnabled?.value ?? false);
      setAuditDays(settings.auditRetentionDays?.source === "override" ? String(settings.auditRetentionDays.value) : "");
      setMetricsDays(settings.metricsRetentionDays?.source === "override" ? String(settings.metricsRetentionDays.value) : "");
      setQoeDays(settings.qoeRetentionDays?.source === "override" ? String(settings.qoeRetentionDays.value) : "");
      setSessionDays(settings.playSessionRetentionDays?.source === "override" ? String(settings.playSessionRetentionDays.value) : "");
      setCacheGB(settings.transcodeCacheMaxGB?.source === "override" ? String(settings.transcodeCacheMaxGB.value) : "");
      setSettingsDirty(false);
    }
  }, [settings]);

  const saveMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: body }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: SETTINGS_KEY });
      toast.success(t("admin.serverSettings.saved"));
      setSettingsDirty(false);
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const resetMutation = useMutation({
    mutationFn: (field: string) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: { [field]: null } }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: SETTINGS_KEY });
      toast.success(t("admin.serverSettings.saved"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const handleSaveSettings = useCallback(async () => {
    try {
      const body: Record<string, unknown> = {};
      if (maxTranscodes) body.maxConcurrentTranscodes = parseInt(maxTranscodes, 10);
      // Drive the unified HW mode (auto/off) rather than the legacy on/off flag,
      // so the mobile toggle stays consistent with the web backend picker and
      // the persisted transcoding.hw_mode override. On = auto (best available),
      // off = software only. Backend pinning (nvenc/vaapi/qsv) is web-only.
      body.hwMode = hwAccel ? "auto" : "off";
      if (loginRate) body.loginRateLimit = parseInt(loginRate, 10);
      if (globalRate) body.globalRateLimit = parseInt(globalRate, 10);
      if (logLevel) body.logLevel = logLevel;
      body.libraryWatcherEnabled = libraryWatcher;
      if (scanInterval) body.periodicScanInterval = scanInterval;
      body.skipPreviewEnabled = skipPreview;
      if (auditDays) body.auditRetentionDays = parseInt(auditDays, 10);
      if (metricsDays) body.metricsRetentionDays = parseInt(metricsDays, 10);
      if (qoeDays) body.qoeRetentionDays = parseInt(qoeDays, 10);
      if (sessionDays) body.playSessionRetentionDays = parseInt(sessionDays, 10);
      if (cacheGB) body.transcodeCacheMaxGB = parseInt(cacheGB, 10);
      await saveMutation.mutateAsync(body);
    } catch {
      // Not silent: saveMutation.onError above already surfaces a toast.
      // This just swallows the rejection mutateAsync re-throws so it doesn't
      // also warn as an unhandled promise.
    }
  }, [
    maxTranscodes, hwAccel, loginRate, globalRate, logLevel, libraryWatcher,
    scanInterval, skipPreview, auditDays, metricsDays, qoeDays, sessionDays,
    cacheGB, saveMutation,
  ]);

  const { data: configEntries, refetch: refetchEntries } = useQuery<ConfigEntry[]>({
    queryKey: SERVER_CONFIG_ENTRIES_KEY,
    queryFn: async () => {
      const res = await eveyFetch<{ entries: ConfigEntry[] }>({ url: "/api/v1/admin/server-config", method: "GET" });
      return res.entries ?? [];
    },
  });

  function getEntryValue(key: string): string {
    return configEntries?.find((e) => e.key === key)?.value ?? "";
  }

  const [apiKeyDrafts, setApiKeyDrafts] = useState<Record<string, string>>({});
  const [apiKeyShown, setApiKeyShown] = useState<Record<string, boolean>>({});

  const saveConfigEntry = useMutation({
    mutationFn: ({ key, value }: { key: string; value: string }) =>
      eveyFetch({ url: `/api/v1/admin/server-config/${key}`, method: "PUT", data: { value } }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: SERVER_CONFIG_ENTRIES_KEY });
      toast.success(t("admin.serverConfig.configUpdated", "Config saved"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const deleteConfigEntry = useMutation({
    mutationFn: (key: string) =>
      eveyFetch({ url: `/api/v1/admin/server-config/${key}`, method: "DELETE" }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: SERVER_CONFIG_ENTRIES_KEY });
      toast.success(t("admin.serverConfig.configUpdated", "Config saved"));
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const introEnabled = getEntryValue("intro_providers.enabled") === "true";
  const tidbKey = getEntryValue("api.tidb_key");
  const tidbSubmitEnabled = getEntryValue("intro_providers.tidb_submit_enabled") === "true";
  const trickplayEnabled = getEntryValue("trickplay.enabled") !== "false";
  const trickplayAdaptiveCpu = getEntryValue("trickplay.adaptive_cpu") !== "false";

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await Promise.all([refetch(), refetchEntries(), qc.invalidateQueries({ queryKey: getAdminSystemServiceGetServerConfigQueryKey() })]);
    setRefreshing(false);
  }, [refetch, refetchEntries, qc]);

  function DefaultHint({ setting, field }: { setting: SettingValue<unknown>; field: string }) {
    if (setting.source === "default") {
      return (
        <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 2 }}>
          {t("admin.serverSettings.usingDefault")} {String(setting.default)}
        </Text>
      );
    }
    return (
      <Pressable onPress={() => resetMutation.mutate(field)} hitSlop={8}>
        <Text style={{ color: theme.colors.accent, fontSize: 11, marginTop: 2 }}>
          {t("admin.serverSettings.reset")}
        </Text>
      </Pressable>
    );
  }

  const s = formStyles(theme);
  const isLoading = configLoading || settingsLoading;

  return (
    <ScrollView
      keyboardShouldPersistTaps="handled"
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
    >
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.serverConfig.title", "Server Config"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      {isLoading ? (
        <View style={{ alignItems: "center", paddingTop: 60 }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <View style={{ padding: 16 }}>
          <Text style={s.sectionTitle}>{t("admin.serverConfig.title", "Server Config")}</Text>

          <Text style={s.label}>{t("admin.serverSettings.serverName", "Server Name")}</Text>
          <TextInput value={serverName} onChangeText={setServerName} style={s.input} placeholderTextColor={theme.colors.textMuted} placeholder="My Server" />

          <Text style={s.label}>Base URL</Text>
          <TextInput value={baseUrl} onChangeText={setBaseUrl} style={s.input} placeholderTextColor={theme.colors.textMuted} placeholder="https://example.com" autoCapitalize="none" keyboardType="url" />

          <Pressable
            onPress={handleSaveBasic}
            disabled={updateConfig.isPending}
            style={[s.saveBtn, { opacity: updateConfig.isPending ? 0.5 : 1 }]}
          >
            <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>{t("common.save")}</Text>
          </Pressable>

          {/* API Keys section */}
          <>
            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
            <Text style={s.sectionTitle}>{t("admin.serverConfig.apiKeys.title", "API Keys")}</Text>

            {API_KEY_FIELDS.map((field) => {
              const current = getEntryValue(field.dbKey);
              const draft = apiKeyDrafts[field.dbKey] ?? current;
              const isShown = apiKeyShown[field.dbKey] ?? false;
              return (
                <View key={field.dbKey}>
                  <View style={{ flexDirection: "row", alignItems: "center", flexWrap: "wrap", gap: 6, marginTop: 16, marginBottom: 6 }}>
                    <Text style={[s.label, { marginTop: 0, marginBottom: 0 }]}>{t(field.labelKey, field.labelFallback)}</Text>
                    {field.hasDefault && !current && (
                      <View style={[staticStyles.badge, { backgroundColor: theme.colors.surfaceRaised }]}>
                        <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>
                          {t("admin.serverConfig.apiKeys.usingDefault", "Clé par défaut")}
                        </Text>
                      </View>
                    )}
                    {!!current && (
                      <View style={[staticStyles.badge, { backgroundColor: theme.colors.surfaceRaised }]}>
                        <Text style={{ color: theme.colors.accent, fontSize: 10 }}>
                          {t("admin.serverConfig.apiKeys.usingCustom", "Clé personnalisée")}
                        </Text>
                      </View>
                    )}
                  </View>
                  <View style={{ flexDirection: "row", alignItems: "center", gap: 8 }}>
                    <TextInput
                      value={draft}
                      onChangeText={(v) => setApiKeyDrafts((p) => ({ ...p, [field.dbKey]: v }))}
                      style={[s.input, { flex: 1 }]}
                      placeholderTextColor={theme.colors.textMuted}
                      placeholder={field.labelFallback}
                      autoComplete="off"
                      autoCapitalize="none"
                      autoCorrect={false}
                      secureTextEntry={!isShown}
                    />
                    <Pressable
                      onPress={() => setApiKeyShown((p) => ({ ...p, [field.dbKey]: !p[field.dbKey] }))}
                      style={[staticStyles.showBtn, { borderColor: theme.colors.border, backgroundColor: theme.colors.surfaceRaised }]}
                      hitSlop={8}
                      accessibilityRole="button"
                      accessibilityLabel={isShown ? t("admin.serverConfig.hide", "Masquer") : t("admin.serverConfig.show", "Afficher")}
                    >
                      <Text style={{ color: theme.colors.textMuted, fontSize: 12, fontWeight: "600" }}>
                        {isShown ? t("admin.serverConfig.hide", "Masquer") : t("admin.serverConfig.show", "Afficher")}
                      </Text>
                    </Pressable>
                  </View>
                  <View style={{ flexDirection: "row", gap: 8, marginTop: 8 }}>
                    <Pressable
                      onPress={() => saveConfigEntry.mutate({ key: field.dbKey, value: draft })}
                      disabled={saveConfigEntry.isPending}
                      style={[s.saveBtn, { flex: 1, marginTop: 0, opacity: saveConfigEntry.isPending ? 0.5 : 1 }]}
                    >
                      <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 14 }}>{t("common.save")}</Text>
                    </Pressable>
                    {current && (
                      <Pressable
                        onPress={() => {
                          deleteConfigEntry.mutate(field.dbKey);
                          setApiKeyDrafts((p) => ({ ...p, [field.dbKey]: "" }));
                        }}
                        disabled={deleteConfigEntry.isPending}
                        style={[s.saveBtn, { flex: 1, marginTop: 0, backgroundColor: theme.colors.border, opacity: deleteConfigEntry.isPending ? 0.5 : 1 }]}
                      >
                        <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: 14 }}>{t("admin.serverSettings.reset", "Reset")}</Text>
                      </Pressable>
                    )}
                  </View>
                </View>
              );
            })}
          </>

          {/* Intro Providers section */}
          <>
            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
            <Text style={s.sectionTitle}>{t("admin.intro.providersTitle", "Intro Providers")}</Text>

            <View style={staticStyles.switchRow}>
              <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1, marginRight: 12 }}>
                {t("admin.intro.enabled", "Enable community intro/recap providers")}
              </Text>
              <Switch
                value={introEnabled}
                onValueChange={(v) => saveConfigEntry.mutate({ key: "intro_providers.enabled", value: v ? "true" : "false" })}
                trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
                disabled={saveConfigEntry.isPending}
              />
            </View>

            {introEnabled && tidbKey ? (
              <View style={staticStyles.switchRow}>
                <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1, marginRight: 12 }}>
                  {t("admin.intro.tidbSubmit", "Submit detected segments to TIDB")}
                </Text>
                <Switch
                  value={tidbSubmitEnabled}
                  onValueChange={(v) => saveConfigEntry.mutate({ key: "intro_providers.tidb_submit_enabled", value: v ? "true" : "false" })}
                  trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
                  disabled={saveConfigEntry.isPending}
                />
              </View>
            ) : null}

            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 8 }}>
              {t("admin.intro.restartHint", "Restart server for changes to take effect.")}
            </Text>
          </>

          {/* Trickplay section */}
          <>
            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
            <Text style={s.sectionTitle}>{t("admin.serverConfig.trickplayTitle", "Trickplay")}</Text>

            <View style={staticStyles.switchRow}>
              <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1, marginRight: 12 }}>
                {t("admin.serverConfig.trickplayEnabled", "Générer les vignettes trickplay")}
              </Text>
              <Switch
                value={trickplayEnabled}
                onValueChange={(v) => saveConfigEntry.mutate({ key: "trickplay.enabled", value: v ? "true" : "false" })}
                trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
                disabled={saveConfigEntry.isPending}
              />
            </View>

            <View style={staticStyles.switchRow}>
              <Text style={{ color: theme.colors.text, fontSize: 15, flex: 1, marginRight: 12 }}>
                {t("admin.serverConfig.trickplayAdaptiveCpu", "Adapter la charge CPU dynamiquement")}
              </Text>
              <Switch
                value={trickplayAdaptiveCpu}
                onValueChange={(v) => saveConfigEntry.mutate({ key: "trickplay.adaptive_cpu", value: v ? "true" : "false" })}
                trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
                disabled={saveConfigEntry.isPending || !trickplayEnabled}
              />
            </View>

            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 8 }}>
              {t("admin.serverConfig.trickplayAdaptiveCpuDesc", "Réduit automatiquement la cadence de génération quand le CPU est chargé.")}
            </Text>
          </>

          {/* Language & Region section */}
          <>
            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
            <Text style={s.sectionTitle}>{t("admin.serverConfig.languageRegion", "Language & Region")}</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: 12 }}>
              {t(
                "admin.serverConfig.languageRegionDesc",
                "Server language and region settings. Affects default UI language, default classification region and metadata language.",
              )}
            </Text>
            <LocaleSettingsCard
              getEntryValue={getEntryValue}
              saveEntry={saveConfigEntry.mutate}
              saving={saveConfigEntry.isPending}
            />
          </>

          {/* Memory profile section */}
          <>
            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
            <Text style={s.sectionTitle}>{t("admin.serverConfig.memoryProfileTitle", "Memory Profile")}</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: 12 }}>
              {t(
                "admin.serverConfig.memoryProfileSubtitle",
                "Controls how aggressively idle plugin processes are suspended to save RAM.",
              )}
            </Text>
            <MemoryProfileCard
              getEntryValue={getEntryValue}
              saveEntry={saveConfigEntry.mutate}
              saving={saveConfigEntry.isPending}
            />
          </>

          {/* mkvgo section */}
          <>
            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
            <Text style={s.sectionTitle}>{t("admin.serverConfig.mkvgoTitle", "mkvgo")}</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: 12 }}>
              {t("admin.serverConfig.mkvgoSubtitle", "Media inspection and repair via the mkvgo library.")}
            </Text>
            <MkvgoCard
              getEntryValue={getEntryValue}
              saveEntry={saveConfigEntry.mutate}
              saving={saveConfigEntry.isPending}
            />
          </>

          {/* SMTP section */}
          <>
            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
            <Text style={s.sectionTitle}>{t("admin.serverConfig.emailSmtp", "Email / SMTP")}</Text>
            <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: 12 }}>
              {t("admin.serverConfig.emailSmtpDesc", "Configure outgoing email for notifications and invitations.")}
            </Text>
            <SmtpSettingsCard
              entries={configEntries}
              saveEntry={saveConfigEntry.mutate}
              saving={saveConfigEntry.isPending}
            />
          </>

          {/* Background jobs section */}
          <>
            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
            <Text style={s.sectionTitle}>{t("admin.serverConfig.backgroundJobs.title", "Background Jobs")}</Text>
            <BackgroundJobsCard
              entries={configEntries}
              saveEntry={saveConfigEntry.mutate}
              deleteEntry={deleteConfigEntry.mutate}
              saving={saveConfigEntry.isPending || deleteConfigEntry.isPending}
            />
          </>

          {/* Parser tokens section */}
          <>
            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
            <Text style={s.sectionTitle}>{t("admin.serverConfig.parserTokensTitle", "Filename Parser Tokens")}</Text>
            <ParserTokensCard
              entries={configEntries}
              saveEntry={saveConfigEntry.mutate}
              saving={saveConfigEntry.isPending}
            />
          </>

          {settings ? (
            <>
              <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
              <Text style={s.sectionTitle}>{t("admin.serverSettings.title", "Server Settings")}</Text>

              <Text style={s.label}>{t("admin.serverSettings.maxConcurrentTranscodes")}</Text>
              <TextInput
                value={maxTranscodes}
                onChangeText={(v) => { setMaxTranscodes(v); setSettingsDirty(true); }}
                style={s.input}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={String(settings.maxConcurrentTranscodes.default ?? 2)}
                keyboardType="numeric"
              />
              <DefaultHint setting={settings.maxConcurrentTranscodes} field="maxConcurrentTranscodes" />

              <Text style={s.label}>{t("admin.serverSettings.hwAccelEnabled")}</Text>
              <View style={staticStyles.switchRow}>
                <Text style={{ color: theme.colors.text, fontSize: 15 }}>
                  {hwAccel ? t("common.enabled", "Enabled") : t("common.disabled", "Disabled")}
                </Text>
                <Switch
                  value={hwAccel}
                  onValueChange={(v) => { setHwAccel(v); setSettingsDirty(true); }}
                  trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
                />
              </View>
              <DefaultHint setting={settings.hwAccelEnabled} field="hwAccelEnabled" />

              <Text style={s.label}>{t("admin.serverSettings.loginRateLimit")}</Text>
              <TextInput
                value={loginRate}
                onChangeText={(v) => { setLoginRate(v); setSettingsDirty(true); }}
                style={s.input}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={String(settings.loginRateLimit.default ?? 10)}
                keyboardType="numeric"
              />
              <DefaultHint setting={settings.loginRateLimit} field="loginRateLimit" />

              <Text style={s.label}>{t("admin.serverSettings.globalRateLimit")}</Text>
              <TextInput
                value={globalRate}
                onChangeText={(v) => { setGlobalRate(v); setSettingsDirty(true); }}
                style={s.input}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={String(settings.globalRateLimit.default ?? 300)}
                keyboardType="numeric"
              />
              <DefaultHint setting={settings.globalRateLimit} field="globalRateLimit" />

              <Text style={s.label}>{t("admin.serverSettings.logLevel", "Niveau de log")}</Text>
              <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
                <Pressable
                  onPress={() => { setLogLevel(""); setSettingsDirty(true); }}
                  style={[staticStyles.chip, { borderColor: theme.colors.border, backgroundColor: logLevel === "" ? theme.colors.accent : theme.colors.surface }]}
                >
                  <Text style={{ color: logLevel === "" ? theme.colors.onAccent : theme.colors.textMuted, fontSize: 13 }}>
                    {t("admin.serverConfig.envDefault", "Défaut env")} ({settings.logLevel?.default ?? "info"})
                  </Text>
                </Pressable>
                {LOG_LEVELS.map((lvl) => (
                  <Pressable
                    key={lvl}
                    onPress={() => { setLogLevel(lvl); setSettingsDirty(true); }}
                    style={[staticStyles.chip, { borderColor: theme.colors.border, backgroundColor: logLevel === lvl ? theme.colors.accent : theme.colors.surface }]}
                  >
                    <Text style={{ color: logLevel === lvl ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>{lvl}</Text>
                  </Pressable>
                ))}
              </View>
              {settings.logLevel && <DefaultHint setting={settings.logLevel} field="logLevel" />}

              <Text style={s.label}>{t("admin.serverSettings.libraryWatcher", "Surveillance des bibliothèques")}</Text>
              <View style={staticStyles.switchRow}>
                <Text style={{ color: theme.colors.textMuted, fontSize: 12, flex: 1, marginRight: 12 }}>
                  {t("admin.serverSettings.restartHint", "Nécessite un redémarrage du serveur.")}
                </Text>
                <Switch
                  value={libraryWatcher}
                  onValueChange={(v) => { setLibraryWatcher(v); setSettingsDirty(true); }}
                  trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
                />
              </View>
              {settings.libraryWatcherEnabled && <DefaultHint setting={settings.libraryWatcherEnabled} field="libraryWatcherEnabled" />}

              <Text style={s.label}>
                {t("admin.serverSettings.periodicScanInterval", "Intervalle de scan périodique")} {t("admin.serverSettings.restartHint", "Nécessite un redémarrage du serveur.")}
              </Text>
              <TextInput
                value={scanInterval}
                onChangeText={(v) => { setScanInterval(v); setSettingsDirty(true); }}
                style={s.input}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={String(settings.periodicScanInterval?.default ?? "2m")}
                autoCapitalize="none"
              />
              {settings.periodicScanInterval && <DefaultHint setting={settings.periodicScanInterval} field="periodicScanInterval" />}

              <Text style={s.label}>{t("admin.serverSettings.skipPreview", "Ignorer la génération de prévisualisations")}</Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: 6 }}>
                {t("admin.serverSettings.skipPreviewInfo", "Désactive la génération des vignettes trickplay pendant le scan pour économiser du CPU.")}
              </Text>
              <View style={staticStyles.switchRow}>
                <Text style={{ color: theme.colors.text, fontSize: 15 }}>
                  {skipPreview ? t("common.enabled", "Activé") : t("common.disabled", "Désactivé")}
                </Text>
                <Switch
                  value={skipPreview}
                  onValueChange={(v) => { setSkipPreview(v); setSettingsDirty(true); }}
                  trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
                />
              </View>
              {settings.skipPreviewEnabled && <DefaultHint setting={settings.skipPreviewEnabled} field="skipPreviewEnabled" />}

              <Text style={s.label}>
                {t("admin.serverSettings.auditRetentionDays", "Rétention des logs d'audit (jours, 0 = illimité)")} {t("admin.serverSettings.retentionSweepHint", "(s'applique à la prochaine purge quotidienne)")}
              </Text>
              <TextInput
                value={auditDays}
                onChangeText={(v) => { setAuditDays(v); setSettingsDirty(true); }}
                style={s.input}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={String(settings.auditRetentionDays?.default ?? 0)}
                keyboardType="numeric"
              />
              {settings.auditRetentionDays && <DefaultHint setting={settings.auditRetentionDays} field="auditRetentionDays" />}

              <Text style={s.label}>
                {t("admin.serverSettings.metricsRetentionDays", "Rétention des métriques serveur (jours)")} {t("admin.serverSettings.retentionSweepHint", "(s'applique à la prochaine purge quotidienne)")}
              </Text>
              <TextInput
                value={metricsDays}
                onChangeText={(v) => { setMetricsDays(v); setSettingsDirty(true); }}
                style={s.input}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={String(settings.metricsRetentionDays?.default ?? 90)}
                keyboardType="numeric"
              />
              {settings.metricsRetentionDays && <DefaultHint setting={settings.metricsRetentionDays} field="metricsRetentionDays" />}

              <Text style={s.label}>
                {t("admin.serverSettings.qoeRetentionDays", "Rétention de la qualité de lecture (QoE, jours)")} {t("admin.serverSettings.retentionSweepHint", "(s'applique à la prochaine purge quotidienne)")}
              </Text>
              <TextInput
                value={qoeDays}
                onChangeText={(v) => { setQoeDays(v); setSettingsDirty(true); }}
                style={s.input}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={String(settings.qoeRetentionDays?.default ?? 30)}
                keyboardType="numeric"
              />
              {settings.qoeRetentionDays && <DefaultHint setting={settings.qoeRetentionDays} field="qoeRetentionDays" />}

              <Text style={s.label}>
                {t("admin.serverSettings.playSessionRetentionDays", "Rétention de l'historique de lecture (jours, 0 = illimité)")} {t("admin.serverSettings.retentionSweepHint", "(s'applique à la prochaine purge quotidienne)")}
              </Text>
              <TextInput
                value={sessionDays}
                onChangeText={(v) => { setSessionDays(v); setSettingsDirty(true); }}
                style={s.input}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={String(settings.playSessionRetentionDays?.default ?? 0)}
                keyboardType="numeric"
              />
              {settings.playSessionRetentionDays && <DefaultHint setting={settings.playSessionRetentionDays} field="playSessionRetentionDays" />}

              <Text style={s.label}>
                {t("admin.serverSettings.transcodeCacheMaxGB", "Budget du cache de transcodage (Go)")} {t("admin.serverSettings.restartHint", "Nécessite un redémarrage du serveur.")}
              </Text>
              <TextInput
                value={cacheGB}
                onChangeText={(v) => { setCacheGB(v); setSettingsDirty(true); }}
                style={s.input}
                placeholderTextColor={theme.colors.textMuted}
                placeholder={String(settings.transcodeCacheMaxGB?.default ?? 50)}
                keyboardType="numeric"
              />
              {settings.transcodeCacheMaxGB && <DefaultHint setting={settings.transcodeCacheMaxGB} field="transcodeCacheMaxGB" />}

              <Pressable
                onPress={handleSaveSettings}
                disabled={!settingsDirty || saveMutation.isPending}
                style={[s.saveBtn, { opacity: !settingsDirty || saveMutation.isPending ? 0.5 : 1 }]}
              >
                {saveMutation.isPending ? (
                  <ActivityIndicator size="small" color={theme.colors.onAccent} />
                ) : (
                  <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 15 }}>{t("common.save")}</Text>
                )}
              </Pressable>
            </>
          ) : null}
        </View>
      )}
    </ScrollView>
  );
}

const formStyles = (t: ReturnType<typeof useTheme>) => StyleSheet.create({
  sectionTitle: { color: t.colors.text, fontSize: 18, fontWeight: "700", marginBottom: 8 },
  label: { color: t.colors.textMuted, fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginTop: 16, marginBottom: 6 },
  input: { backgroundColor: t.colors.surface, color: t.colors.text, borderRadius: t.radii.md, paddingHorizontal: 14, paddingVertical: 12, fontSize: 15 },
  saveBtn: { backgroundColor: t.colors.accent, borderRadius: t.radii.md, paddingVertical: 14, alignItems: "center", marginTop: 24 },
});

const staticStyles = StyleSheet.create({
  switchRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", paddingVertical: 8 },
  badge: { paddingHorizontal: 6, paddingVertical: 2, borderRadius: 4 },
  showBtn: { paddingHorizontal: 10, paddingVertical: 10, borderRadius: 8, borderWidth: StyleSheet.hairlineWidth },
  chip: { paddingHorizontal: 12, paddingVertical: 8, borderRadius: 8, borderWidth: StyleSheet.hairlineWidth },
});
