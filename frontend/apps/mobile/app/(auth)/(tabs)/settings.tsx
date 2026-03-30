import { useEffect, useState } from "react";
import { View, Text, ScrollView, Pressable, Alert, StyleSheet } from "react-native";
import { router } from "expo-router";
import Constants from "expo-constants";
import { useUserServiceGetCurrentUser, useAdminServiceGetSystemInfo } from "@evey/api";
import { useAuthStore, useThemeStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { getServerUrl, clearServerUrl } from "../../../src/lib/server-config";

const THEME_OPTIONS = [
  { label: "Dark", value: "default" },
  { label: "Light", value: "light-default" },
  { label: "AMOLED", value: "amoled" },
] as const;

function ChevronRight({ color }: { color: string }) {
  return <Text style={{ color, fontSize: 18 }}>{"›"}</Text>;
}

export default function SettingsScreen() {
  const theme = useTheme();
  const logout = useAuthStore((s) => s.logout);
  const currentTheme = useThemeStore((s) => s.theme);
  const setTheme = useThemeStore((s) => s.setTheme);
  const [serverUrl, setServerUrl] = useState<string | null>(null);

  const { data: user } = useUserServiceGetCurrentUser();
  const { data: systemInfo } = useAdminServiceGetSystemInfo();

  useEffect(() => {
    getServerUrl().then(setServerUrl);
  }, []);

  async function handleChangeServer() {
    Alert.alert("Change server", "Disconnect from this server?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Disconnect",
        style: "destructive",
        onPress: async () => {
          logout();
          await clearServerUrl();
          router.replace("/server-connect");
        },
      },
    ]);
  }

  async function handleSignOut() {
    Alert.alert("Sign out", "Are you sure?", [
      { text: "Cancel", style: "cancel" },
      {
        text: "Sign out",
        style: "destructive",
        onPress: () => {
          logout();
          router.replace("/login");
        },
      },
    ]);
  }

  const appVersion = Constants.expoConfig?.version ?? "1.0.0";

  const s = styles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={{ paddingBottom: theme.spacing.xl }}>
      {/* Server */}
      <Text style={s.sectionTitle}>Server</Text>
      <View style={s.section}>
        <View style={s.row}>
          <Text style={s.label}>URL</Text>
          <Text style={s.value} numberOfLines={1}>{serverUrl ?? "..."}</Text>
        </View>
        <Pressable style={s.row} onPress={handleChangeServer}>
          <Text style={s.labelDestructive}>Change server</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
      </View>

      {/* Account */}
      <Text style={s.sectionTitle}>Account</Text>
      <View style={s.section}>
        <View style={s.row}>
          <Text style={s.label}>Username</Text>
          <Text style={s.value}>{user?.username ?? "..."}</Text>
        </View>
        <Pressable style={s.row} onPress={handleSignOut}>
          <Text style={s.labelDestructive}>Sign out</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
      </View>

      {/* Appearance */}
      <Text style={s.sectionTitle}>Appearance</Text>
      <View style={s.section}>
        {THEME_OPTIONS.map((opt) => (
          <Pressable key={opt.value} style={s.row} onPress={() => setTheme(opt.value)}>
            <Text style={s.label}>{opt.label}</Text>
            {currentTheme === opt.value && (
              <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>✓</Text>
            )}
          </Pressable>
        ))}
      </View>

      {/* About */}
      <Text style={s.sectionTitle}>About</Text>
      <View style={s.section}>
        <View style={s.row}>
          <Text style={s.label}>App version</Text>
          <Text style={s.value}>{appVersion}</Text>
        </View>
        <View style={s.row}>
          <Text style={s.label}>Server version</Text>
          <Text style={s.value}>{systemInfo?.version ?? "—"}</Text>
        </View>
      </View>
    </ScrollView>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    container: {
      flex: 1,
      backgroundColor: t.colors.bg,
    },
    sectionTitle: {
      color: t.colors.textMuted,
      fontSize: 13,
      fontWeight: "600",
      textTransform: "uppercase",
      marginTop: t.spacing.lg,
      marginBottom: t.spacing.xs,
      marginHorizontal: t.spacing.md,
    },
    section: {
      backgroundColor: t.colors.surface,
      borderRadius: t.radii.md,
      marginHorizontal: t.spacing.md,
      overflow: "hidden",
    },
    row: {
      flexDirection: "row",
      alignItems: "center",
      justifyContent: "space-between",
      paddingVertical: 14,
      paddingHorizontal: t.spacing.md,
      borderBottomWidth: StyleSheet.hairlineWidth,
      borderBottomColor: t.colors.border,
    },
    label: {
      color: t.colors.text,
      fontSize: 16,
    },
    labelDestructive: {
      color: "#ef4444",
      fontSize: 16,
    },
    value: {
      color: t.colors.textMuted,
      fontSize: 15,
      flexShrink: 1,
      marginLeft: t.spacing.md,
    },
  });
