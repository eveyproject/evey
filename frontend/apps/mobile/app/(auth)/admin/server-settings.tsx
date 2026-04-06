import { useState, useEffect, useCallback } from "react";
import { View, Text, ScrollView, TextInput, Pressable, ActivityIndicator, Switch, RefreshControl, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
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

const SETTINGS_KEY = ["admin", "server-settings"];

function useServerSettings() {
  return useQuery<ServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });
}

export default function AdminServerSettingsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [refreshing, setRefreshing] = useState(false);

  const { data, isLoading, refetch } = useServerSettings();

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
      setDirty(false);
    }
  }, [data]);

  const saveMutation = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: body }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: SETTINGS_KEY });
      toast.success(t("admin.serverSettings.saved"));
      setDirty(false);
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

  const handleSave = useCallback(async () => {
    try {
      const body: Record<string, unknown> = {};
      if (serverName) body.serverName = serverName;
      if (maxTranscodes) body.maxConcurrentTranscodes = parseInt(maxTranscodes, 10);
      body.hwAccelEnabled = hwAccel;
      if (loginRate) body.loginRateLimit = parseInt(loginRate, 10);
      if (globalRate) body.globalRateLimit = parseInt(globalRate, 10);
      await saveMutation.mutateAsync(body);
    } catch { /* handled by onError */ }
  }, [serverName, maxTranscodes, hwAccel, loginRate, globalRate, saveMutation]);

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    await refetch();
    setRefreshing(false);
  }, [refetch]);

  const s = formStyles(theme);

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

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
      refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
    >
      <Stack.Screen options={{
        headerShown: true,
        title: t("admin.serverSettings.title"),
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      {isLoading ? (
        <View style={{ alignItems: "center", paddingTop: 60 }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : data ? (
        <View style={{ padding: 16 }}>
          {/* Server Name */}
          <Text style={s.label}>{t("admin.serverSettings.serverName")}</Text>
          <TextInput
            value={serverName}
            onChangeText={(v) => { setServerName(v); setDirty(true); }}
            style={s.input}
            placeholderTextColor={theme.colors.textMuted}
            placeholder={data.serverName.default ?? "Evey"}
          />
          <DefaultHint setting={data.serverName} field="serverName" />

          {/* Max Concurrent Transcodes */}
          <Text style={s.label}>{t("admin.serverSettings.maxConcurrentTranscodes")}</Text>
          <TextInput
            value={maxTranscodes}
            onChangeText={(v) => { setMaxTranscodes(v); setDirty(true); }}
            style={s.input}
            placeholderTextColor={theme.colors.textMuted}
            placeholder={String(data.maxConcurrentTranscodes.default ?? 2)}
            keyboardType="numeric"
          />
          <DefaultHint setting={data.maxConcurrentTranscodes} field="maxConcurrentTranscodes" />

          {/* Hardware Acceleration */}
          <Text style={s.label}>{t("admin.serverSettings.hwAccelEnabled")}</Text>
          <View style={styles.switchRow}>
            <Text style={{ color: theme.colors.text, fontSize: 15 }}>
              {hwAccel ? t("common.enabled", "Enabled") : t("common.disabled", "Disabled")}
            </Text>
            <Switch
              value={hwAccel}
              onValueChange={(v) => { setHwAccel(v); setDirty(true); }}
              trackColor={{ false: theme.colors.border, true: theme.colors.accent }}
            />
          </View>
          <DefaultHint setting={data.hwAccelEnabled} field="hwAccelEnabled" />

          {/* Login Rate Limit */}
          <Text style={s.label}>{t("admin.serverSettings.loginRateLimit")}</Text>
          <TextInput
            value={loginRate}
            onChangeText={(v) => { setLoginRate(v); setDirty(true); }}
            style={s.input}
            placeholderTextColor={theme.colors.textMuted}
            placeholder={String(data.loginRateLimit.default ?? 10)}
            keyboardType="numeric"
          />
          <DefaultHint setting={data.loginRateLimit} field="loginRateLimit" />

          {/* Global Rate Limit */}
          <Text style={s.label}>{t("admin.serverSettings.globalRateLimit")}</Text>
          <TextInput
            value={globalRate}
            onChangeText={(v) => { setGlobalRate(v); setDirty(true); }}
            style={s.input}
            placeholderTextColor={theme.colors.textMuted}
            placeholder={String(data.globalRateLimit.default ?? 300)}
            keyboardType="numeric"
          />
          <DefaultHint setting={data.globalRateLimit} field="globalRateLimit" />

          {/* Save */}
          <Pressable
            onPress={handleSave}
            disabled={!dirty || saveMutation.isPending}
            style={[s.saveBtn, { opacity: !dirty || saveMutation.isPending ? 0.5 : 1 }]}
          >
            {saveMutation.isPending ? (
              <ActivityIndicator size="small" color="#fff" />
            ) : (
              <Text style={{ color: "#fff", fontWeight: "600", fontSize: 15 }}>{t("common.save")}</Text>
            )}
          </Pressable>
        </View>
      ) : null}
    </ScrollView>
  );
}

const formStyles = (t: ReturnType<typeof useTheme>) => StyleSheet.create({
  label: { color: t.colors.textMuted, fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginTop: 16, marginBottom: 6 },
  input: { backgroundColor: t.colors.surface, color: t.colors.text, borderRadius: t.radii.md, paddingHorizontal: 14, paddingVertical: 12, fontSize: 15 },
  saveBtn: { backgroundColor: t.colors.accent, borderRadius: t.radii.md, paddingVertical: 14, alignItems: "center", marginTop: 24 },
});

const styles = StyleSheet.create({
  switchRow: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", paddingVertical: 8 },
});
