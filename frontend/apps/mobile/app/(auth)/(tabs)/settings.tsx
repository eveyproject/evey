import { useState, useCallback, useEffect, useRef } from "react";
import { View, Text, ScrollView, Pressable, Alert, TextInput, Switch, ActivityIndicator, StyleSheet, Linking } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import * as ImagePicker from "expo-image-picker";
import { router } from "expo-router";
import Constants from "expo-constants";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, useUserServiceUpdateProfile, useAdminSystemServiceGetSystemInfo, useUserServiceAuthorizeQuickConnect, useUserServiceUpdateUser, useUserServiceLogout, eveyFetchRaw, apiErrorMessage } from "@evey/api";
import { clearRefreshToken } from "../../../src/lib/refresh-token";
import { forceRefresh } from "../../../src/lib/auth-refresh";
import { useAuthStore, useLocaleStore, useServerListStore, toast, isDemoMode } from "@evey/store";
import { useBiometricAvailable, useBiometricEnabled } from "../../../src/hooks/useBiometric";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrlNullable } from "../../../src/hooks/useServerUrl";
import { supportedLanguages } from "../../../src/i18n";
import { ThemeSelector } from "../../../src/components/ThemeSelector";
import { wikiUrl } from "@evey/client";
import { usePluginSlotEntries } from "../../../src/features/plugins/PluginSlot";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";

function ChevronRight({ color }: { color: string }) {
  return <Text style={{ color, fontSize: 18 }}>{"›"}</Text>;
}

export default function SettingsScreen() {
  const theme = useTheme();
  // Plugins mounting the only user-settings slot the server allows.
  const pluginEntries = usePluginSlotEntries("settings.user.appearance");
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
  const [currentPassword, setCurrentPassword] = useState("");

  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const defaultProfile = profilesResp?.profiles?.find((p: any) => p.isDefault) ?? profilesResp?.profiles?.[0];
  // The profile actually in use, which is not necessarily the default one -
  // somebody switched to it from profile-select. Writing language or avatar
  // onto `defaultProfile` changed a profile the user was not even looking at.
  const activeProfileId = useAuthStore((s) => s.profileId);
  const activeProfile =
    profilesResp?.profiles?.find((p: { id?: string }) => p.id === activeProfileId) ?? defaultProfile;
  // Admin RPC: for anyone else it answered PermissionDenied on every visit
  // (a WARN per non-admin in the server log) and the row read "-" anyway.
  const { data: systemInfo } = useAdminSystemServiceGetSystemInfo({ query: { enabled: isAdmin } });
  const [avatarPreview, setAvatarPreview] = useState<string | null>(null);
  const accessToken = useAuthStore((s) => s.accessToken);
  // The avatar route takes the token as a header, never in the URL, where
  // it lands in caches and logs.
  const authHeaders = useAuthHeaders();

  const updateUser = useUserServiceUpdateUser({
    mutation: {
      onSuccess: () => {
        setNewPassword("");
        setConfirmPassword("");
        setCurrentPassword("");
        setEditingAccount(false);
        toast.success(t("auth.settings.account.profileUpdated") || "Profile updated");
        qc.invalidateQueries({ queryKey: ["/api/v1/users/me"] });
      },
      onError: (err: Error) => {
        toast.error(apiErrorMessage(err, t, t("auth.settings.account.updateFailed", "Update failed")));
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
    setCurrentPassword("");
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
    // Username, e-mail and password are credentials: the server re-checks the current password.
    if (!currentPassword) {
      Alert.alert(t("errors.user.current_password_required"));
      return;
    }
    try {
      await updateUser.mutateAsync({ id: user.id, data: { ...data, currentPassword } });
    } catch (err) {
      toast.error(apiErrorMessage(err as Error, t, t("auth.settings.account.updateFailed", "Update failed")));
    }
  }, [user, editUsername, editEmail, newPassword, confirmPassword, currentPassword, updateUser, t]);

  // An avatar upload can take seconds on a phone link, and nothing on screen
  // said one was in flight: repeated taps fired concurrent POSTs on the same
  // profile, and the last one to land won. Every other mutation on this screen
  // already guards its own pending state.
  const [avatarUploading, setAvatarUploading] = useState(false);
  const handleAvatarPick = useCallback(async () => {
    if (!activeProfile?.id || avatarUploading) return;
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
    setAvatarUploading(true);
    try {
      await eveyFetchRaw({ url: `/api/v1/profiles/${activeProfile.id}/avatar`, method: "POST", body: formData as any });
      setAvatarPreview(asset.uri);
      if (user?.id) void qc.invalidateQueries({ queryKey: [`/api/v1/users/${user.id}/profiles`] });
      toast.success(t("auth.profile.avatarUpdated"));
    } catch {
      toast.error(t("auth.profile.avatarFailed"));
    } finally {
      setAvatarUploading(false);
    }
  }, [activeProfile, avatarUploading, user?.id, qc, t]);

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
    // 2. Update profile on server (not on the demo: the profile is shared, the
    // language stays on this phone)
    if (activeProfile?.id && !isDemoMode()) {
      try {
        await updateProfile.mutateAsync({ id: activeProfile.id, data: { locale: code } });
        // 3. Refresh JWT to get the new locale claim - through the one path that
        // writes the ROTATED refresh token back to SecureStore. A bare
        // /auth/refresh rotated it server-side (cookie jar) and kept the old one
        // on disk, so the next silent refresh replayed it into a forced logout.
        await forceRefresh();
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
  const tmdbAttribution = t("common.attribution.tmdb", "This product uses the TMDB API but is not endorsed or certified by TMDB.");
  const currentLang = i18n.language;
  const s = sStyles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={{ paddingBottom: theme.spacing.xl }}>

      {/* Profile card */}
      <View style={[s.profileCard, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md, marginHorizontal: theme.spacing.md, marginTop: theme.spacing.lg }]}>
        <View style={{ flexDirection: "row", alignItems: "center", gap: theme.spacing.md }}>
          <Pressable onPress={handleAvatarPick} disabled={avatarUploading} style={[s.avatar, { backgroundColor: theme.colors.accent, opacity: avatarUploading ? 0.6 : 1 }]} accessible={true} accessibilityLabel={t("auth.profile.avatarUpdated") || "Change avatar"} accessibilityRole="button" accessibilityState={{ disabled: avatarUploading, busy: avatarUploading }}>
            {avatarPreview ? (
              <Image
                source={{ uri: avatarPreview }}
                style={{ width: 56, height: 56, borderRadius: 28 }}
                contentFit="cover"
              />
            ) : activeProfile?.avatarPath && activeProfile.id && serverUrl && accessToken ? (
              // The ACTIVE profile, the one the upload above writes to. Reading
              // defaultProfile here meant anyone on a non-default profile saw an
              // initial instead of the avatar they had just set.
              <Image
                source={{ uri: `${serverUrl}/api/v1/profiles/${activeProfile.id}/avatar`, headers: authHeaders }}
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
              {/* A long username used to take its full intrinsic width (RN Text
                  does not shrink by default) and push the badge out of the card. */}
              <Text style={{ color: theme.colors.text, fontSize: 18, fontWeight: "700", flexShrink: 1 }} numberOfLines={1}>{user?.username ?? "..."}</Text>
              {isAdmin && (
                <View style={{ backgroundColor: theme.colors.accent, borderRadius: 4, paddingHorizontal: 5, paddingVertical: 1, flexShrink: 0 }}>
                  <Text style={{ color: theme.colors.onAccent, fontSize: 9, fontWeight: "700" }}>ADMIN</Text>
                </View>
              )}
            </View>
            {/* An address is one unbreakable token: ellipsize in the middle so
                both the mailbox and the domain stay readable. */}
            <Text style={{ color: theme.colors.textMuted, fontSize: 13 }} numberOfLines={1} ellipsizeMode="middle">{user?.email || "-"}</Text>
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
            <View style={{ borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border, paddingTop: theme.spacing.sm, marginTop: theme.spacing.xs }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginBottom: theme.spacing.sm }}>
                {t("auth.settings.account.currentPasswordHint")}
              </Text>
              <TextInput
                placeholder={t("auth.settings.account.currentPassword")}
                placeholderTextColor={theme.colors.textMuted}
                value={currentPassword}
                onChangeText={setCurrentPassword}
                secureTextEntry
                autoComplete="current-password"
                textContentType="password"
                accessibilityLabel={t("auth.settings.account.currentPassword")}
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
        <Pressable style={s.row} onPress={() => router.push("/profile-select?pick=1" as never)} accessible={true} accessibilityLabel={t("mobile.settings.switchProfile")} accessibilityRole="button"><Text style={s.label}>{t("mobile.settings.switchProfile")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
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
          <Pressable key={lang.code} style={s.row} onPress={() => handleLanguageChange(lang.code)} accessible={true} accessibilityLabel={lang.label} accessibilityRole="radio" accessibilityState={{ selected: currentLang === lang.code }}>
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

      {/* Plugins that ship their own per-profile settings. They used to be
          injected inline here, one WebView under another with nothing saying
          which plugin owned what. Now each gets a menu row and a screen of
          its own; no plugin installed means no section at all. */}
      {pluginEntries.length > 0 && (
        <>
          <Text style={s.sectionTitle}>{t("auth.settings.tabs.plugins")}</Text>
          <View style={s.section}>
            {pluginEntries.map((entry) => (
              <Pressable
                key={entry.pluginId}
                style={s.row}
                onPress={() => router.push(`/settings/plugins/${entry.pluginId}`)}
                accessible={true}
                accessibilityLabel={entry.name}
                accessibilityRole="button"
              >
                <Text style={s.label} numberOfLines={2}>{entry.name}</Text>
                <ChevronRight color={theme.colors.textMuted} />
              </Pressable>
            ))}
          </View>
        </>
      )}

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
        <Pressable style={s.row} onPress={() => router.push("/admin/api-keys")} accessible={true} accessibilityLabel={t("auth.apiKeys.title")} accessibilityRole="button"><Text style={s.label}>{t("auth.apiKeys.title")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
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
        {isAdmin && (
          <View style={s.row}><Text style={s.label}>{t("mobile.settings.serverVersion")}</Text><Text style={s.value}>{systemInfo?.version ?? "-"}</Text></View>
        )}
        <Pressable style={s.row} onPress={handleManageServers} accessible={true} accessibilityLabel={t("mobile.servers.title", "Servers")} accessibilityRole="button">
          <Text style={s.label}>{t("mobile.servers.title", "Servers")}</Text>
          <View style={{ flexDirection: "row", alignItems: "center", gap: 6, flexShrink: 0 }}>
            <Text style={s.value}>{t("mobile.servers.manageSubtitle", "{{count}} saved", { count: serverCount })}</Text>
            <ChevronRight color={theme.colors.textMuted} />
          </View>
        </Pressable>
      </View>

      {/* About */}
      <Text style={s.sectionTitle}>{t("mobile.settings.about")}</Text>
      <View style={s.section}>
        <View style={s.row}><Text style={s.label}>{t("mobile.settings.appVersion")}</Text><Text style={s.value}>{appVersion}</Text></View>
        {/* The store listing and the app both have to point at the policy. */}
        <Pressable style={s.row} onPress={() => { Linking.openURL(wikiUrl(i18n.language, "privacy/")).catch(() => {}); }} accessible={true} accessibilityLabel={t("mobile.settings.privacyPolicy")} accessibilityRole="link"><Text style={s.label}>{t("mobile.settings.privacyPolicy")}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
        <Pressable style={s.row} onPress={() => { Linking.openURL("https://www.themoviedb.org/").catch(() => {}); }} accessible={true} accessibilityRole="link"><Text style={[s.label, { flex: 1 }]}>{tmdbAttribution}</Text><ChevronRight color={theme.colors.textMuted} /></Pressable>
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
    // flexShrink: 1 on the labels, not just on `value`: without it a long
    // translated label ("Débit maximum en streaming") took the whole row and
    // the value or the chevron was clipped away by section's overflow:hidden.
    label: { color: t.colors.text, fontSize: 16, flexShrink: 1 },
    labelDestructive: { color: t.colors.error, fontSize: 16, flexShrink: 1 },
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
