import { useState, useEffect, useCallback } from "react";
import { View, Text, ScrollView, TextInput, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useAdminSystemServiceGetServerConfig,
  useAdminSystemServiceUpdateServerConfig,
  getAdminSystemServiceGetServerConfigQueryKey,
} from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";

export default function AdminServerConfigScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data: config, isLoading } = useAdminSystemServiceGetServerConfig();
  const [serverName, setServerName] = useState("");
  const [baseUrl, setBaseUrl] = useState("");

  useEffect(() => {
    if (config) {
      setServerName((config as any).serverName ?? "");
      setBaseUrl((config as any).baseUrl ?? "");
    }
  }, [config]);

  const updateConfig = useAdminSystemServiceUpdateServerConfig({
    mutation: {
      onSuccess: async () => {
        toast.success("Config saved");
        await qc.invalidateQueries({ queryKey: getAdminSystemServiceGetServerConfigQueryKey() });
      },
      onError: () => toast.error("Failed to save"),
    },
  });

  const handleSave = useCallback(async () => {
    try {
      await updateConfig.mutateAsync({ data: { serverName: serverName.trim(), baseUrl: baseUrl.trim() } as any });
    } catch {}
  }, [serverName, baseUrl, updateConfig]);

  const s = formStyles(theme);

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ paddingBottom: 40 }}>
      <Stack.Screen options={{
        headerShown: true,
        title: "Server Config",
        headerStyle: { backgroundColor: theme.colors.surface },
        headerTintColor: theme.colors.text,
      }} />

      {isLoading ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 60 }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      ) : (
        <View style={{ padding: 16 }}>
          <Text style={s.label}>Server Name</Text>
          <TextInput value={serverName} onChangeText={setServerName} style={s.input} placeholderTextColor={theme.colors.textMuted} placeholder="My Server" />

          <Text style={s.label}>Base URL</Text>
          <TextInput value={baseUrl} onChangeText={setBaseUrl} style={s.input} placeholderTextColor={theme.colors.textMuted} placeholder="https://example.com" autoCapitalize="none" keyboardType="url" />

          <Pressable
            onPress={handleSave}
            disabled={updateConfig.isPending}
            style={[s.saveBtn, { opacity: updateConfig.isPending ? 0.5 : 1 }]}
          >
            <Text style={{ color: "#fff", fontWeight: "600", fontSize: 15 }}>Save</Text>
          </Pressable>
        </View>
      )}
    </ScrollView>
  );
}

const formStyles = (t: ReturnType<typeof useTheme>) => StyleSheet.create({
  label: { color: t.colors.textMuted, fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginTop: 16, marginBottom: 6 },
  input: { backgroundColor: t.colors.surface, color: t.colors.text, borderRadius: t.radii.md, paddingHorizontal: 14, paddingVertical: 12, fontSize: 15 },
  saveBtn: { backgroundColor: t.colors.accent, borderRadius: t.radii.md, paddingVertical: 14, alignItems: "center", marginTop: 24 },
});
