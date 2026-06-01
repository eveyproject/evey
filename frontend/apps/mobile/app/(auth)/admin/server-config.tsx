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
}

interface ConfigEntry {
  key: string;
  value: string;
}

const SETTINGS_KEY = ["admin", "server-settings"];
const SERVER_CONFIG_ENTRIES_KEY = ["admin", "server-config-entries"];

const API_KEY_FIELDS = [
  { dbKey: "api.tmdb_key", labelKey: "admin.serverConfig.apiKeys.tmdb", labelFallback: "TMDB Key" },
  { dbKey: "api.omdb_key", labelKey: "admin.serverConfig.apiKeys.omdb", labelFallback: "OMDb Key" },
  { dbKey: "api.opensubtitles_key", labelKey: "admin.serverConfig.apiKeys.opensubtitles", labelFallback: "OpenSubtitles Key" },
  { dbKey: "api.tidb_key", labelKey: "admin.serverConfig.apiKeys.tidb", labelFallback: "TIDB Key" },
] as const;

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
    } catch {}
  }, [serverName, baseUrl, updateConfig]);

  const { data: settings, isLoading: settingsLoading, refetch } = useQuery<ServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });

  const [maxTranscodes, setMaxTranscodes] = useState("");
  const [hwAccel, setHwAccel] = useState(false);
  const [loginRate, setLoginRate] = useState("");
  const [globalRate, setGlobalRate] = useState("");
  const [settingsDirty, setSettingsDirty] = useState(false);

  useEffect(() => {
    if (settings) {
      setMaxTranscodes(settings.maxConcurrentTranscodes.source === "override" ? String(settings.maxConcurrentTranscodes.value) : "");
      setHwAccel(settings.hwAccelEnabled.value);
      setLoginRate(settings.loginRateLimit.source === "override" ? String(settings.loginRateLimit.value) : "");
      setGlobalRate(settings.globalRateLimit.source === "override" ? String(settings.globalRateLimit.value) : "");
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
      await saveMutation.mutateAsync(body);
    } catch {}
  }, [maxTranscodes, hwAccel, loginRate, globalRate, saveMutation]);

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
            <Text style={{ color: "#fff", fontWeight: "600", fontSize: 15 }}>{t("common.save")}</Text>
          </Pressable>

          {/* API Keys section */}
          <>
            <View style={{ height: 1, backgroundColor: theme.colors.border, marginVertical: 24 }} />
            <Text style={s.sectionTitle}>{t("admin.serverConfig.apiKeys.title", "API Keys")}</Text>

            {API_KEY_FIELDS.map((field) => {
              const current = getEntryValue(field.dbKey);
              const draft = apiKeyDrafts[field.dbKey] ?? current;
              return (
                <View key={field.dbKey}>
                  <Text style={s.label}>{t(field.labelKey, field.labelFallback)}</Text>
                  <TextInput
                    value={draft}
                    onChangeText={(v) => setApiKeyDrafts((p) => ({ ...p, [field.dbKey]: v }))}
                    style={s.input}
                    placeholderTextColor={theme.colors.textMuted}
                    placeholder={field.labelFallback}
                    autoComplete="off"
                    autoCapitalize="none"
                    autoCorrect={false}
                  />
                  <View style={{ flexDirection: "row", gap: 8, marginTop: 8 }}>
                    <Pressable
                      onPress={() => saveConfigEntry.mutate({ key: field.dbKey, value: draft })}
                      disabled={saveConfigEntry.isPending}
                      style={[s.saveBtn, { flex: 1, marginTop: 0, opacity: saveConfigEntry.isPending ? 0.5 : 1 }]}
                    >
                      <Text style={{ color: "#fff", fontWeight: "600", fontSize: 14 }}>{t("common.save")}</Text>
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

              <Pressable
                onPress={handleSaveSettings}
                disabled={!settingsDirty || saveMutation.isPending}
                style={[s.saveBtn, { opacity: !settingsDirty || saveMutation.isPending ? 0.5 : 1 }]}
              >
                {saveMutation.isPending ? (
                  <ActivityIndicator size="small" color="#fff" />
                ) : (
                  <Text style={{ color: "#fff", fontWeight: "600", fontSize: 15 }}>{t("common.save")}</Text>
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
});
