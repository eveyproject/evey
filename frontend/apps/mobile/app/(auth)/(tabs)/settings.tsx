import { useState } from "react";
import { View, Text, ScrollView, Pressable, Alert, TextInput, ActivityIndicator, StyleSheet } from "react-native";
import { router } from "expo-router";
import Constants from "expo-constants";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, useAdminServiceGetSystemInfo, useUserServiceAuthorizeQuickConnect, useUserServiceUpdateUser } from "@evey/api";
import { useAuthStore, useThemeStore, useLocaleStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrlNullable } from "../../../src/hooks/useServerUrl";
import { clearServerUrl } from "../../../src/lib/server-config";
import { supportedLanguages } from "../../../src/i18n";

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
  const { t, i18n } = useTranslation();
  const logout = useAuthStore((s) => s.logout);
  const currentTheme = useThemeStore((s) => s.theme);
  const setTheme = useThemeStore((s) => s.setTheme);
  const setLocale = useLocaleStore((s) => s.setLocale);
  const serverUrl = useServerUrlNullable();

  const [qcCode, setQcCode] = useState("");
  const [qcSuccess, setQcSuccess] = useState(false);
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const { data: user } = useUserServiceGetCurrentUser();
  const { data: systemInfo } = useAdminServiceGetSystemInfo();

  const updateUser = useUserServiceUpdateUser({
    mutation: {
      onSuccess: () => {
        setNewPassword("");
        setConfirmPassword("");
        Alert.alert(t("auth.settings.account.passwordUpdated", "Password updated"));
      },
      onError: (err: Error) => {
        Alert.alert(t("auth.settings.account.passwordFailed", "Failed"), err.message);
      },
    },
  });

  const authorizeQc = useUserServiceAuthorizeQuickConnect({
    mutation: {
      onSuccess: () => {
        setQcSuccess(true);
        setQcCode("");
        setTimeout(() => setQcSuccess(false), 3000);
      },
      onError: () => {
        Alert.alert(t("auth.settings.quickConnect.invalidCode"));
      },
    },
  });

  function handleAuthorizeQc() {
    const trimmed = qcCode.trim().toUpperCase();
    if (!trimmed) return;
    authorizeQc.mutate({ data: { code: trimmed } });
  }

  async function handleChangeServer() {
    Alert.alert(t("mobile.settings.changeServer"), t("mobile.settings.disconnectConfirm"), [
      { text: t("common.cancel"), style: "cancel" },
      {
        text: t("mobile.settings.disconnect"),
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
    Alert.alert(t("mobile.settings.signOut"), t("mobile.settings.signOutConfirm"), [
      { text: t("common.cancel"), style: "cancel" },
      {
        text: t("mobile.settings.signOut"),
        style: "destructive",
        onPress: () => {
          logout();
          router.replace("/login");
        },
      },
    ]);
  }

  function handleLanguageChange(code: string) {
    setLocale(code);
    i18n.changeLanguage(code);
  }

  const appVersion = Constants.expoConfig?.version ?? "1.0.0";
  const currentLang = i18n.language;

  const s = styles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={{ paddingBottom: theme.spacing.xl }}>
      {/* Server */}
      <Text style={s.sectionTitle}>{t("mobile.settings.server")}</Text>
      <View style={s.section}>
        <View style={s.row}>
          <Text style={s.label}>URL</Text>
          <Text style={s.value} numberOfLines={1}>{serverUrl ?? "..."}</Text>
        </View>
        <Pressable style={s.row} onPress={handleChangeServer} accessible={true} accessibilityLabel={t("mobile.settings.changeServer")} accessibilityRole="button">
          <Text style={s.labelDestructive}>{t("mobile.settings.changeServer")}</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
      </View>

      {/* Account */}
      <Text style={s.sectionTitle}>{t("auth.settings.tabs.account")}</Text>
      <View style={s.section}>
        <View style={s.row}>
          <Text style={s.label}>{t("auth.login.username")}</Text>
          <Text style={s.value}>{user?.username ?? "..."}</Text>
        </View>
        <Pressable style={s.row} onPress={() => router.push("/history")} accessible={true} accessibilityLabel={t("auth.watchHistory.title", "Watch History")} accessibilityRole="button">
          <Text style={s.label}>{t("auth.watchHistory.title", "Watch History")}</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
        <Pressable style={s.row} onPress={() => router.push("/profile-select")} accessible={true} accessibilityLabel={t("mobile.settings.switchProfile", "Switch Profile")} accessibilityRole="button">
          <Text style={s.label}>{t("mobile.settings.switchProfile", "Switch Profile")}</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
        <Pressable style={s.row} onPress={() => router.push("/settings/parental")} accessible={true} accessibilityLabel={t("auth.settings.tabs.parental", "Parental Controls")} accessibilityRole="button">
          <Text style={s.label}>{t("auth.settings.tabs.parental", "Parental Controls")}</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
        <Pressable style={s.row} onPress={handleSignOut} accessible={true} accessibilityLabel={t("mobile.settings.signOut")} accessibilityRole="button">
          <Text style={s.labelDestructive}>{t("mobile.settings.signOut")}</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
      </View>

      {/* Change Password */}
      <Text style={s.sectionTitle}>{t("auth.settings.account.changePassword", "Change Password")}</Text>
      <View style={s.section}>
        <View style={{ paddingHorizontal: theme.spacing.md, paddingTop: 14 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginBottom: theme.spacing.sm }}>
            {t("auth.settings.account.changePasswordDesc", "Set a new password for your account")}
          </Text>
        </View>
        <View style={{ paddingHorizontal: theme.spacing.md, paddingBottom: 14, gap: theme.spacing.sm }}>
          <TextInput
            placeholder={t("auth.settings.account.newPassword", "New password")}
            placeholderTextColor={theme.colors.textMuted}
            value={newPassword}
            onChangeText={setNewPassword}
            secureTextEntry
            autoComplete="new-password"
            style={{
              backgroundColor: theme.colors.bg,
              color: theme.colors.text,
              borderRadius: theme.radii.md,
              padding: theme.spacing.sm,
              borderWidth: 1,
              borderColor: theme.colors.border,
            }}
          />
          <TextInput
            placeholder={t("auth.settings.account.confirmPassword", "Confirm password")}
            placeholderTextColor={theme.colors.textMuted}
            value={confirmPassword}
            onChangeText={setConfirmPassword}
            secureTextEntry
            autoComplete="new-password"
            style={{
              backgroundColor: theme.colors.bg,
              color: theme.colors.text,
              borderRadius: theme.radii.md,
              padding: theme.spacing.sm,
              borderWidth: 1,
              borderColor: theme.colors.border,
            }}
          />
          <Pressable
            onPress={() => {
              if (!user?.id) return;
              if (newPassword !== confirmPassword) {
                Alert.alert(t("auth.settings.account.passwordMismatch", "Passwords do not match"));
                return;
              }
              if (newPassword.length < 8) {
                Alert.alert(t("auth.settings.account.passwordMin", "Password must be at least 8 characters"));
                return;
              }
              updateUser.mutate({ id: user.id, data: { password: newPassword } });
            }}
            disabled={updateUser.isPending || newPassword.length < 8}
            style={{
              backgroundColor: newPassword.length < 8 ? theme.colors.border : theme.colors.accent,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              alignItems: "center",
            }}
          >
            {updateUser.isPending ? (
              <ActivityIndicator color="#fff" size="small" />
            ) : (
              <Text style={{ color: "#fff", fontWeight: "600", fontSize: 14 }}>
                {t("auth.settings.account.updatePassword", "Update Password")}
              </Text>
            )}
          </Pressable>
        </View>
      </View>

      {/* Language */}
      <Text style={s.sectionTitle}>{t("common.language")}</Text>
      <View style={s.section}>
        {supportedLanguages.map((lang) => (
          <Pressable key={lang.code} style={s.row} onPress={() => handleLanguageChange(lang.code)} accessible={true} accessibilityLabel={lang.label} accessibilityRole="button">
            <Text style={s.label}>{lang.label}</Text>
            {currentLang === lang.code && (
              <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>✓</Text>
            )}
          </Pressable>
        ))}
      </View>

      {/* Appearance */}
      <Text style={s.sectionTitle}>{t("auth.settings.tabs.appearance")}</Text>
      <View style={s.section}>
        {THEME_OPTIONS.map((opt) => (
          <Pressable key={opt.value} style={s.row} onPress={() => setTheme(opt.value)} accessible={true} accessibilityLabel={`${opt.label} theme`} accessibilityRole="button">
            <Text style={s.label}>{opt.label}</Text>
            {currentTheme === opt.value && (
              <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>✓</Text>
            )}
          </Pressable>
        ))}
      </View>

      {/* Quick Connect */}
      <Text style={s.sectionTitle}>{t("auth.settings.quickConnect.title")}</Text>
      <View style={s.section}>
        <View style={{ paddingHorizontal: theme.spacing.md, paddingTop: 14 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginBottom: theme.spacing.sm }}>
            {t("auth.settings.quickConnect.description")}
          </Text>
        </View>
        <View style={[s.row, { gap: theme.spacing.sm }]}>
          <TextInput
            placeholder="A1B2C3"
            placeholderTextColor={theme.colors.textMuted}
            value={qcCode}
            onChangeText={(v) => setQcCode(v.toUpperCase())}
            autoCapitalize="characters"
            autoCorrect={false}
            maxLength={6}
            style={{
              flex: 1,
              backgroundColor: theme.colors.bg,
              color: theme.colors.text,
              borderRadius: theme.radii.md,
              padding: theme.spacing.sm,
              fontFamily: "monospace",
              letterSpacing: 4,
              fontSize: 18,
              textAlign: "center",
              borderWidth: 1,
              borderColor: theme.colors.border,
            }}
            accessible={true}
            accessibilityLabel={t("auth.settings.quickConnect.codeLabel")}
          />
          <Pressable
            onPress={handleAuthorizeQc}
            disabled={authorizeQc.isPending || qcCode.trim().length < 4}
            accessible={true}
            accessibilityLabel={t("auth.settings.quickConnect.authorize")}
            accessibilityRole="button"
            style={{
              backgroundColor: qcCode.trim().length < 4 ? theme.colors.border : theme.colors.accent,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            {authorizeQc.isPending ? (
              <ActivityIndicator color={theme.colors.text} size="small" />
            ) : (
              <Text style={{ color: "#fff", fontWeight: "600", fontSize: 14 }}>
                {t("auth.settings.quickConnect.authorize")}
              </Text>
            )}
          </Pressable>
        </View>
        {qcSuccess && (
          <View style={{ paddingHorizontal: theme.spacing.md, paddingBottom: 14 }}>
            <Text style={{ color: theme.colors.accent, fontSize: 13 }}>
              {t("auth.settings.quickConnect.authorized")}
            </Text>
          </View>
        )}
      </View>

      {/* About */}
      <Text style={s.sectionTitle}>{t("mobile.settings.about")}</Text>
      <View style={s.section}>
        <View style={s.row}>
          <Text style={s.label}>{t("mobile.settings.appVersion")}</Text>
          <Text style={s.value}>{appVersion}</Text>
        </View>
        <View style={s.row}>
          <Text style={s.label}>{t("mobile.settings.serverVersion")}</Text>
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
