import { useState, useCallback, useEffect, useRef } from "react";
import { View, Text, ScrollView, Pressable, Alert, TextInput, Switch, ActivityIndicator, StyleSheet } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import * as ImagePicker from "expo-image-picker";
import { router } from "expo-router";
import Constants from "expo-constants";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, useUserServiceUpdateProfile, useAdminSystemServiceGetSystemInfo, useUserServiceAuthorizeQuickConnect, useUserServiceUpdateUser, useUserServiceLogout, eveyFetch, eveyFetchRaw } from "@evey/api";
import { clearRefreshToken } from "../../../src/lib/refresh-token";
import { useAuthStore, useLocaleStore, useServerListStore, toast } from "@evey/store";
import { useBiometricAvailable, useBiometricEnabled } from "../../../src/hooks/useBiometric";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrlNullable } from "../../../src/hooks/useServerUrl";
import { supportedLanguages } from "../../../src/i18n";
import { ThemeSelector } from "../../../src/components/ThemeSelector";

function ChevronRight({ color }: { color: string }) {
  return <Text style={{ color, fontSize: 18 }}>{"›"}</Text>;
}

export default function SettingsScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const logout = useAuthStore((s) => s.logout);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const setLocale = useLocaleStore((s) => s.setLocale);
  const serverUrl = useServerUrlNullable();
  const serverCount = useServerListStore((s) => Object.keys(s.servers).length);

  const [qcCode, setQcCode] = useState("");
  const [qcSuccess, setQcSuccess] = useState(false);
  const updateProfile = useUserServiceUpdateProfile();
  const { available: biometricAvailable } = useBiometricAvailable();
  const { enabled: biometricEnabled, toggle: toggleBiometric } = useBiometricEnabled();

  // Account editing
  const [editingAccount, setEditingAccount] = useState(false);
  const [editUsername, setEditUsername] = useState("");
  const [editEmail, setEditEmail] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");

  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const defaultProfile = profilesResp?.profiles?.find((p: any) => p.isDefault) ?? profilesResp?.profiles?.[0];
  // The profile actually in use, which is not necessarily the default one -
  // somebody switched to it from profile-select. Writing language or avatar
  // onto `defaultProfile` changed a profile the user was not even looking at.
  const activeProfileId = useAuthStore((s) => s.profileId);
  const activeProfile =
    profilesResp?.profiles?.find((p: { id?: string }) => p.id === activeProfileId) ?? defaultProfile;
  const { data: systemInfo } = useAdminSystemServiceGetSystemInfo();
  const [avatarPreview, setAvatarPreview] = useState<string | null>(null);
  const accessToken = useAuthStore((s) => s.accessToken);

  const updateUser = useUserServiceUpdateUser({
    mutation: {
      onSuccess: () => {
        setNewPassword("");
        setConfirmPassword("");
        setEditingAccount(false);
        toast.success(t("auth.settings.account.profileUpdated") || "Profile updated");
        qc.invalidateQueries({ queryKey: ["/api/v1/users/me"] });
      },
      onError: (err: Error) => {
        toast.error(err.message || "Update failed");
      },
    },
  });

  const qcSuccessTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => {
    if (qcSuccessTimerRef.current) clearTimeout(qcSuccessTimerRef.current);
  }, []);

  const authorizeQc = useUserServiceAuthorizeQuickConnect({
    mutation: {
      onSuccess: () => {
        setQcSuccess(true);
        setQcCode("");
        if (qcSuccessTimerRef.current) clearTimeout(qcSuccessTimerRef.current);
        qcSuccessTimerRef.current = setTimeout(() => setQcSuccess(false), 3000);
      },
      onError: () => Alert.alert(t("auth.settings.quickConnect.invalidCode")),
    },
  });

  const startEditAccount = useCallback(() => {
    setEditUsername(user?.username ?? "");
    setEditEmail(user?.email ?? "");
    setEditingAccount(true);
  }, [user]);

  const handleSaveAccount = useCallback(async () => {
    if (!user?.id) return;
    const data: Record<string, string> = {};
    if (editUsername.trim() !== user.username) data.username = editUsername.trim();
    if (editEmail.trim() !== (user.email ?? "")) data.email = editEmail.trim();
    if (newPassword) {
      if (newPassword !== confirmPassword) {
        Alert.alert(t("auth.settings.account.passwordMismatch"));
        return;
      }
      if (newPassword.length < 8) {
        Alert.alert(t("auth.settings.account.passwordMin"));
        return;
      }
      data.password = newPassword;
    }
    if (Object.keys(data).length === 0) {
      setEditingAccount(false);
      return;
    }
    try {
      await updateUser.mutateAsync({ id: user.id, data });
    } catch (err) {
      const msg = err instanceof Error ? err.message : t("auth.settings.account.updateFailed", "Update failed");
      toast.error(msg);
    }
  }, [user, editUsername, editEmail, newPassword, confirmPassword, updateUser, t]);

  const handleAvatarPick = useCallback(async () => {
    if (!activeProfile?.id) return;
    const result = await ImagePicker.launchImageLibraryAsync({
      mediaTypes: ImagePicker.MediaTypeOptions.Images,
      allowsEditing: true,
      aspect: [1, 1],
      quality: 0.8,
    });
    if (result.canceled || !result.assets?.[0]) return;
    const asset = result.assets[0];
    const formData = new FormData();
    formData.append("avatar", {
      uri: asset.uri,
      type: asset.mimeType ?? "image/jpeg",
      name: `avatar.${asset.mimeType?.split("/")[1] ?? "jpg"}`,
    } as any);
    try {
      await eveyFetchRaw({ url: `/api/v1/profiles/${activeProfile.id}/avatar`, method: "POST", body: formData as any });
      setAvatarPreview(asset.uri);
      if (user?.id) void qc.invalidateQueries({ queryKey: [`/api/v1/users/${user.id}/profiles`] });
      toast.success(t("auth.profile.avatarUpdated"));
    } catch {
      toast.error(t("auth.profile.avatarFailed"));
    }
  }, [activeProfile]);

  function handleAuthorizeQc() {
    const trimmed = qcCode.trim().toUpperCase();
    if (!trimmed) return;
    authorizeQc.mutate({ data: { code: trimmed } });
  }

  function handleManageServers() {
    router.push("/(auth)/manage-servers");
  }

  const logoutMutation = useUserServiceLogout();
  async function handleSignOut() {
    Alert.alert(t("mobile.settings.signOut"), t("mobile.settings.signOutConfirm"), [
      { text: t("common.cancel"), style: "cancel" },
      {
        text: t("mobile.settings.signOut"),
        style: "destructive",
        onPress: async () => {
          // Best-effort revoke server-side; never block the UI on it.
          logoutMutation.mutate({ data: {} });
          // Drop the persisted refresh token so a cold-start refresh on
          // bootstrap doesn't silently re-authenticate the user.
          await clearRefreshToken().catch(() => {});
          logout();
          router.replace("/login");
        },
      },
    ]);
  }

  async function handleLanguageChange(code: string) {
    // 1. Update UI
    setLocale(code);
    void i18n.changeLanguage(code);
    // 2. Update profile on server
    if (activeProfile?.id) {
      try {
        await updateProfile.mutateAsync({ id: activeProfile.id, data: { locale: code } });
        // 3. Refresh JWT to get new locale claim
        const resp = await eveyFetch<{ accessToken?: string }>({ url: "/api/v1/auth/refresh", method: "POST", data: {} });
        if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      } catch (err) {
        // UI is already updated locally; warn the user that the server side
        // didn't pick up the change so they don't get a silent revert next
        // login.
        const msg = err instanceof Error ? err.message : t("common.languageSyncFailed", "Language saved locally but couldn't sync to server");
        toast.error(msg);
      }
    }
  }

  const initial = user?.username?.charAt(0).toUpperCase() ?? "U";
  const memberSince = user?.createdAt
    ? new Date(user.createdAt).toLocaleDateString(undefined, { year: "numeric", month: "long" })
    : null;
  const appVersion = Constants.expoConfig?.version ?? "1.0.0";
  const currentLang = i18n.language;
  const s = sStyles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={{ paddingBottom: theme.spacing.xl }}>

      {/* Profile card */}
      <View style={[s.profileCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: theme.spacing.md, marginTop: theme.spacing.lg }]}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: theme.spacing.md }}>
          <Pressable onPress={handleAvatarPick} style={[s.avatar, { backgroundColor: theme.colors.accent }]} accessible={true} accessibilityLabel={t("auth.profile.avatarUpdated") || "Change avatar"} accessibilityRole="button">
            {avatarPreview ? (
              <Image
                source={{ uri: avatarPreview }}
                style={{ width: 56, height: 56, borderRadius: 28 }}
                contentFit="cover"
              />
            ) : defaultProfile?.avatarPath && serverUrl && accessToken ? (
              <Image
                source={{ uri: `${serverUrl}/api/v1/profiles/${defaultProfile?.id}/avatar?token=${accessToken}` }}
                style={{ width: 56, height: 56, borderRadius: 28 }}
                contentFit="cover"
              />
            ) : (
              <Text style={s.avatarText}>{initial}</Text>
            )}
            <View style={s.avatarOverlay}>
              {/* avatarOverlay's background is theme.colors.overlay, a scrim that stays
                  near-black on every theme, so a fixed white icon always has contrast here */}
              <Ionicons name="camera" size={16} color="#fff" />
            </View>
          </Pressable>
          <View style={{ flex: 1 }}>
            <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
              <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700" }}>{user?.username ?? "..."}</Text>
              {isAdmin && (
                <View style={{ backgroundColor: theme.colors.accent, borderRadius: 4, paddingHorizontal: 5, paddingVertical: 1 }}>
                  <Text style={{ color: theme.colors.onAccent, fontSize: 9, fontWeight: "700" }}>ADMIN</Text>
                </View>
              )}
            </View>
            <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{user?.email || "-"}</Text>
            {memberSince && <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 2 }}>{memberSince}</Text>}
          </View>
          <Pressable onPress={editingAccount ? () => setEditingAccount(false) : startEditAccount} hitSlop={8} accessible={true} accessibilityLabel={editingAccount ? t("common.cancel") : t("common.edit") || "Edit"} accessibilityRole="button">
            <Ionicons name={editingAccount ? "close" : "pencil"} size={18} color={theme.colors.accent} />
          </Pressable>
        </View>

        {/* Inline edit */}
        {editingAccount && (
          <View style={{ marginTop: theme.spacing.md, gap: theme.spacing.sm }}>
            <TextInput
              placeholder={t("auth.login.username")}
              placeholderTextColor={theme.colors.textMuted}
              value={editUsername}
              onChangeText={setEditUsername}
              autoCapitalize="none"
              style={s.editInput}
            />
            <TextInput
              placeholder={t("auth.profile.email")}
              placeholderTextColor={theme.colors.textMuted}
              value={editEmail}
              onChangeText={setEditEmail}
              keyboardType="email-address"
              autoCapitalize="none"
              style={s.editInput}
            />
            <View style={{ borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border, paddingTop: theme.spacing.sm, marginTop: theme.spacing.xs }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: theme.spacing.sm }}>
                {t("auth.settings.account.changePasswordDesc")}
              </Text>
              <TextInput
                placeholder={t("auth.settings.account.newPassword")}
                placeholderTextColor={theme.colors.textMuted}
                value={newPassword}
                onChangeText={setNewPassword}
                secureTextEntry
                style={s.editInput}
              />
              <View style={{ height: theme.spacing.sm }} />
              <TextInput
                placeholder={t("auth.settings.account.confirmPassword")}
                placeholderTextColor={theme.colors.textMuted}
                value={confirmPassword}
                onChangeText={setConfirmPassword}
                secureTextEntry
                style={s.editInput}
              />
            </View>
            <Pressable
              onPress={handleSaveAccount}
              disabled={updateUser.isPending}
              style={[s.saveBtn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: updateUser.isPending ? 0.5 : 1 }]}
              accessible={true}
              accessibilityLabel={t("common.save") || "Save"}
              accessibilityRole="button"
            >
              {updateUser.isPending ? (
                <ActivityIndicator color={theme.colors.onAccent} size="small" />
              ) : (
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 14 }}>{t("common.save") || "Save"}</Text>
              )}
            </Pressable>
          </View>
        )}
      </View>

      {/* Account shortcuts */}
      <Text style={s.sectionTitle}>{t("auth.settings.tabs.account")}</Text>
      <View style={s.section}>
        <Pressable style={s.row} onPress={() => router.push("/history")} accessible={true} accessibilityLabel={t("common.watchHistory.title")} accessibilityRole="button"><Text style={s.label}>{t("common.watchHistory.title")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
        <Pressable style={s.row} onPress={() => router.push("/settings/stats")} accessible={true} accessibilityLabel={t("common.userStats.title")} accessibilityRole="button"><Text style={s.label}>{t("common.userStats.title")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
        <Pressable style={s.row} onPress={() => router.push("/notifications")} accessible={true} accessibilityLabel={t("common.notifications.title")} accessibilityRole="button"><Text style={s.label}>{t("common.notifications.title")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
        <Pressable style={s.row} onPress={() => router.push("/profile-select")} accessible={true} accessibilityLabel={t("mobile.settings.switchProfile")} accessibilityRole="button"><Text style={s.label}>{t("mobile.settings.switchProfile")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
        <Pressable style={s.row} onPress={() => router.push("/profile/manage")} accessible={true} accessibilityLabel={t("auth.profile.manage.title", "Manage profiles")} accessibilityRole="button"><Text style={s.label}>{t("auth.profile.manage.title", "Manage profiles")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
        <Pressable style={s.row} onPress={handleSignOut} accessible={true} accessibilityLabel={t("mobile.settings.signOut")} accessibilityRole="button"><Text style={s.labelDestructive}>{t("mobile.settings.signOut")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
      </View>

      {/* Appearance */}
      <Text style={s.sectionTitle}>{t("auth.settings.tabs.appearance")}</Text>
      <ThemeSelector />

      {/* Language */}
      <Text style={s.sectionTitle}>{t("common.language")}</Text>
      <View style={s.section}>
        {supportedLanguages.map((lang) => (
          <Pressable key={lang.code} style={s.row} onPress={() => handleLanguageChange(lang.code)} accessible={true} accessibilityLabel={lang.label} accessibilityRole="radio">
            <Text style={s.label}>{lang.label}</Text>
            {currentLang === lang.code && <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Region */}
      <Text style={s.sectionTitle}>{t("common.region.title")}</Text>
      <View style={s.section}>
        <Pressable style={s.row} onPress={() => router.push("/settings/region")} accessible={true} accessibilityLabel={t("common.region.title")} accessibilityRole="button">
          <Text style={s.label}>{t("common.region.title")}</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
      </View>

      {/* Playback */}
      <Text style={s.sectionTitle}>{t("auth.settings.tabs.playback")}</Text>
      <View style={s.section}>
        <Pressable style={s.row} onPress={() => router.push("/settings/playback")} accessible={true} accessibilityLabel={t("auth.settings.playback.bitrateTitle")} accessibilityRole="button"><Text style={s.label}>{t("auth.settings.playback.bitrateTitle")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
      </View>

      {/* Notifications */}
      <Text style={s.sectionTitle}>{t("auth.settings.tabs.notifications")}</Text>
      <View style={s.section}>
        <Pressable style={s.row} onPress={() => router.push("/settings/notifications")} accessible={true} accessibilityLabel={t("auth.settings.tabs.notifications")} accessibilityRole="button"><Text style={s.label}>{t("auth.settings.tabs.notifications")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
      </View>

      {/* Parental */}
      <Text style={s.sectionTitle}>{t("auth.settings.tabs.parental")}</Text>
      <View style={s.section}>
        <Pressable style={s.row} onPress={() => router.push("/settings/parental")} accessible={true} accessibilityLabel={t("auth.settings.tabs.parental")} accessibilityRole="button"><Text style={s.label}>{t("auth.settings.tabs.parental")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
      </View>

      {/* Security */}
      <Text style={s.sectionTitle}>{t("auth.settings.tabs.security") || "Security"}</Text>
      <View style={s.section}>
        {biometricAvailable && (
          <View style={[s.row, { justifyContent: "space-between" }]}>
            <Text style={s.label}>{t("mobile.settings.biometricLogin") || "Biometric login"}</Text>
            <Switch value={biometricEnabled} onValueChange={toggleBiometric} trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }} />
          </View>
        )}
        <Pressable style={s.row} onPress={() => router.push("/admin/api-keys")} accessible={true} accessibilityLabel="API Keys" accessibilityRole="button"><Text style={s.label}>API Keys</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
        <Pressable style={s.row} onPress={() => router.push("/settings/devices")} accessible={true} accessibilityLabel={t("auth.devices.title")} accessibilityRole="button"><Text style={s.label}>{t("auth.devices.title")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
      </View>

      {/* Quick Connect */}
      <Text style={s.sectionTitle}>{t("auth.settings.quickConnect.title")}</Text>
      <View style={s.section}>
        <View style={{ paddingHorizontal: theme.spacing.md, paddingTop: 14 }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginBottom: theme.spacing.sm }}>
            {t("auth.settings.quickConnect.description")}
          </Text>
        </View>
        <Pressable style={s.row} onPress={() => router.push("/qr-scan")} accessible={true} accessibilityLabel={t("auth.settings.quickConnect.scanQr")} accessibilityRole="button">
          <Text style={s.label}>{t("auth.settings.quickConnect.scanQr")}</Text>
          <ChevronRight color={theme.colors.textMuted} />
        </Pressable>
        <View style={[s.row, { gap: theme.spacing.sm }]}>
          <TextInput
            placeholder="A1B2C3"
            placeholderTextColor={theme.colors.textMuted}
            value={qcCode}
            onChangeText={(v) => setQcCode(v.toUpperCase())}
            autoCapitalize="characters"
            autoCorrect={false}
            maxLength={6}
            style={s.qcInput}
          />
          <Pressable
            onPress={handleAuthorizeQc}
            disabled={authorizeQc.isPending || qcCode.trim().length < 4}
            style={{ backgroundColor: qcCode.trim().length < 4 ? theme.colors.border : theme.colors.accent, borderRadius: theme.radii.md, paddingVertical: theme.spacing.sm, paddingHorizontal: theme.spacing.md, alignItems: "center" }}
            accessible={true}
            accessibilityLabel={t("auth.settings.quickConnect.authorize")}
            accessibilityRole="button"
          >
            {authorizeQc.isPending ? <ActivityIndicator color={theme.colors.onAccent} size="small" /> : <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 14 }}>{t("auth.settings.quickConnect.authorize")}</Text>}
          </Pressable>
        </View>
        {qcSuccess && (
          <View style={{ paddingHorizontal: theme.spacing.md, paddingBottom: 14 }}>
            <Text style={{ color: theme.colors.accent, fontSize: 13 }}>{t("auth.settings.quickConnect.authorized")}</Text>
          </View>
        )}
      </View>

      {/* Server */}
      <Text style={s.sectionTitle}>{t("mobile.settings.server")}</Text>
      <View style={s.section}>
        <View style={s.row}><Text style={s.label}>URL</Text><Text style={s.value} numberOfLines={1}>{serverUrl ?? "..."}</Text></View>
        <View style={s.row}><Text style={s.label}>{t("mobile.settings.serverVersion")}</Text><Text style={s.value}>{systemInfo?.version ?? "\u2014"}</Text></View>
        <Pressable style={s.row} onPress={handleManageServers} accessible={true} accessibilityLabel={t("mobile.servers.title", "Servers")} accessibilityRole="button">
          <Text style={s.label}>{t("mobile.servers.title", "Servers")}</Text>
          <View style={{ flexDirection: "row", alignItems: "center", gap: 6 }}>
            <Text style={s.value}>{t("mobile.servers.manageSubtitle", "{{count}} saved", { count: serverCount })}</Text>
            <ChevronRight color={theme.colors.textMuted} />
          </View>
        </Pressable>
      </View>

      {/* About */}
      <Text style={s.sectionTitle}>{t("mobile.settings.about")}</Text>
      <View style={s.section}>
        <View style={s.row}><Text style={s.label}>{t("mobile.settings.appVersion")}</Text><Text style={s.value}>{appVersion}</Text></View>
      </View>
    </ScrollView>
  );
}

const sStyles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    container: { flex: 1, backgroundColor: t.colors.bg },
    profileCard: { padding: t.spacing.md },
    avatar: { width: 56, height: 56, borderRadius: 28, justifyContent: "center", alignItems: "center" },
    avatarText: { color: t.colors.onAccent, fontSize: 24, fontWeight: "700" },
    // fixed dark scrim (not derived from theme) so the camera icon reads
    // consistently regardless of the underlying avatar photo's colours
    avatarOverlay: { position: "absolute" as const, bottom: 0, right: 0, width: 22, height: 22, borderRadius: 11, backgroundColor: t.colors.overlay, justifyContent: "center", alignItems: "center" },
    editInput: {
      backgroundColor: t.colors.bg,
      color: t.colors.text,
      borderRadius: t.radii.md,
      paddingHorizontal: t.spacing.sm + 4,
      paddingVertical: t.spacing.sm,
      fontSize: 15,
      borderWidth: 1,
      borderColor: t.colors.border,
    },
    saveBtn: { paddingVertical: t.spacing.sm + 2, alignItems: "center" },
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
    label: { color: t.colors.text, fontSize: 16 },
    labelDestructive: { color: t.colors.error, fontSize: 16 },
    value: { color: t.colors.textMuted, fontSize: 15, flexShrink: 1, marginLeft: t.spacing.md },
    qcInput: {
      flex: 1,
      backgroundColor: t.colors.bg,
      color: t.colors.text,
      borderRadius: t.radii.md,
      padding: t.spacing.sm,
      fontFamily: "monospace",
      letterSpacing: 4,
      fontSize: 18,
      textAlign: "center",
      borderWidth: 1,
      borderColor: t.colors.border,
    },
  });
