import { useCallback, useState } from "react";
import { View, Text, Pressable, Modal, StyleSheet, ActivityIndicator } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { router } from "expo-router";
import * as SecureStore from "expo-secure-store";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { userServiceRefreshToken, userServiceLogout } from "@evey/api";
import { clearRefreshToken } from "../../lib/refresh-token";
import type { V1Profile } from "@evey/api";
import { useAuthStore, toast } from "@evey/store";
import { useTheme } from "../../providers/ThemeProvider";
import { useServerUrl } from "../../hooks/useServerUrl";
import { PinPromptDialog } from "../../features/profile/PinPromptDialog";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

const LAST_PROFILE_KEY = "evey:last_profile_id";

interface Props {
  profiles: V1Profile[];
  activeProfileId: string | null;
  size?: number;
}

type MenuItem = {
  key: string;
  label: string;
  icon: React.ComponentProps<typeof Ionicons>["name"];
  action: () => void;
};

export function ProfileSwitcher({ profiles, activeProfileId, size = 30 }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const accessToken = useAuthStore((s) => s.accessToken);
  const serverUrl = useServerUrl();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const logout = useAuthStore((s) => s.logout);
  const [menuOpen, setMenuOpen] = useState(false);
  const [switchOpen, setSwitchOpen] = useState(false);
  const [switching, setSwitching] = useState<string | null>(null);
  const [pinTarget, setPinTarget] = useState<V1Profile | null>(null);

  const active = profiles.find((p) => p.id === activeProfileId)
    ?? profiles.find((p) => p.isDefault)
    ?? profiles[0];

  const others = profiles.filter((p) => p.id && p.id !== active?.id);

  const doSwitch = useCallback(async (target: V1Profile, pinToken?: string): Promise<"ok" | "pin" | "fail"> => {
    if (!target.id) return "fail";
    setSwitching(target.id);
    try {
      const resp = await userServiceRefreshToken({ profileId: target.id, pinToken });
      if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      try {
        await SecureStore.setItemAsync(LAST_PROFILE_KEY, target.id);
      } catch { /* ignore */ }
      await qc.invalidateQueries();
      setSwitchOpen(false);
      setMenuOpen(false);
      return "ok";
    } catch (err) {
      // 403 = the server wants a parental PIN proof (gated source or PIN-carrying
      // target). The refresh token is not consumed, so a retry with a pinToken works.
      if (!pinToken && (err as { httpStatus?: number }).httpStatus === 403) return "pin";
      console.warn("profile switch failed", err);
      toast.error(err instanceof Error ? err.message : t("auth.profile.switchFailed", "Could not switch profile"));
      return "fail";
    } finally {
      setSwitching(null);
    }
  }, [qc, t]);

  const handleSwitch = useCallback(async (target: V1Profile) => {
    if (!target.id || switching) return;
    if (target.id === active?.id) return;
    const targetIsKids = target.isKids === true;
    const currentIsKids = active?.isKids === true;
    if (currentIsKids && !targetIsKids) {
      setPinTarget(target);
      return;
    }
    if ((await doSwitch(target)) === "pin") setPinTarget(target);
  }, [active, doSwitch, switching]);

  const go = useCallback((to: string) => {
    setMenuOpen(false);
    setTimeout(() => router.push(to as never), 50);
  }, []);

  const handleLogout = useCallback(() => {
    setMenuOpen(false);
    qc.clear();
    logout();
    // Best-effort revoke server-side; never block the UI on it.
    void userServiceLogout({}).catch(() => {});
    // Drop the persisted refresh token so the bootstrap refresh on cold
    // start doesn't silently re-authenticate the user. On a shared device a
    // failure here means the token survives on disk after the user believes
    // they signed out, so it must never be swallowed silently: log it and
    // tell the user their sign-out may be incomplete.
    void clearRefreshToken().catch((err) => {
      console.warn("failed to clear refresh token on logout", err);
      toast.error(t("auth.profile.logoutIncomplete", "Sign-out may be incomplete on this device. Restart the app to finish clearing your session."));
    });
    setTimeout(() => router.replace("/login"), 50);
  }, [qc, logout, t]);

  if (!active) return null;

  const activeAvatarUrl = active.avatarPath && active.id && serverUrl && accessToken
    ? `${serverUrl}/api/v1/profiles/${active.id}/avatar?token=${accessToken}`
    : null;
  const initial = (active.name ?? "?").charAt(0).toUpperCase();
  const s = styles(theme);

  const items: MenuItem[] = [
    { key: "settings", label: t("common.nav.settings", "Settings"), icon: "settings-outline", action: () => go("/(auth)/(tabs)/settings") },
    { key: "history", label: t("common.nav.watchHistory", "Watch History"), icon: "time-outline", action: () => go("/history") },
    { key: "quickconnect", label: t("common.nav.quickConnect", "Quick Connect"), icon: "qr-code-outline", action: () => go("/quick-connect") },
  ];
  if (isAdmin) items.push({ key: "admin", label: t("common.nav.admin", "Admin"), icon: "grid-outline", action: () => go("/admin") });

  return (
    <>
      <Pressable
        onPress={() => setMenuOpen(true)}
        style={[s.trigger, { width: size, height: size, borderRadius: size / 2, backgroundColor: theme.colors.accent }]}
        accessibilityRole="button"
        accessibilityLabel={t("common.nav.userMenu", "User menu")}
      >
        {activeAvatarUrl ? (
          <Image source={{ uri: activeAvatarUrl }} style={{ width: size, height: size, borderRadius: size / 2 }} contentFit="cover" />
        ) : (
          <Text style={{ color: theme.colors.onAccent, fontSize: Math.round(size * 0.45), fontWeight: "600" }}>
            {initial}
          </Text>
        )}
      </Pressable>

      {/* Dropdown menu */}
      <Modal visible={menuOpen} transparent animationType="fade" onRequestClose={() => setMenuOpen(false)}>
        <Pressable style={s.backdrop} onPress={() => setMenuOpen(false)} />
        <View style={[s.dropdown, { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border, borderRadius: theme.radii.md }]}>
          {others.length > 0 && (
            <>
              <Pressable
                onPress={() => { setMenuOpen(false); setTimeout(() => setSwitchOpen(true), 50); }}
                style={s.menuItem}
                accessibilityRole="button"
                accessibilityLabel={t("common.nav.switchProfile", "Switch profile")}
              >
                <Ionicons name="swap-horizontal-outline" size={ICON_SIZE.md} color={theme.colors.textMuted} />
                <Text style={[s.menuLabel, { color: theme.colors.text }]}>
                  {t("common.nav.switchProfile", "Switch profile")}
                </Text>
              </Pressable>
              <View style={[s.divider, { backgroundColor: theme.colors.border }]} />
            </>
          )}

          <Pressable
            onPress={() => { setMenuOpen(false); setTimeout(() => router.push("/(auth)/profile/manage"), 50); }}
            style={s.menuItem}
            accessibilityRole="button"
            accessibilityLabel={t("common.nav.manageProfiles", "Manage Profiles")}
          >
            <Ionicons name="people-outline" size={ICON_SIZE.md} color={theme.colors.textMuted} />
            <Text style={[s.menuLabel, { color: theme.colors.text }]}>
              {t("common.nav.manageProfiles", "Manage Profiles")}
            </Text>
          </Pressable>

          <View style={[s.divider, { backgroundColor: theme.colors.border }]} />

          {items.map((item) => (
            <Pressable
              key={item.key}
              onPress={item.action}
              style={s.menuItem}
              accessibilityRole="button"
              accessibilityLabel={item.label}
            >
              <Ionicons name={item.icon} size={ICON_SIZE.md} color={theme.colors.textMuted} />
              <Text style={[s.menuLabel, { color: theme.colors.text }]}>{item.label}</Text>
            </Pressable>
          ))}

          <View style={[s.divider, { backgroundColor: theme.colors.border }]} />
          <Pressable
            onPress={handleLogout}
            style={s.menuItem}
            accessibilityRole="button"
            accessibilityLabel={t("common.nav.signOut", "Sign out")}
          >
            <Ionicons name="log-out-outline" size={ICON_SIZE.md} color={theme.colors.error} />
            <Text style={[s.menuLabel, { color: theme.colors.error }]}>
              {t("common.nav.signOut", "Sign out")}
            </Text>
          </Pressable>
        </View>
      </Modal>

      {/* Switch profile modal */}
      <Modal visible={switchOpen} transparent animationType="fade" onRequestClose={() => setSwitchOpen(false)}>
        <View style={[s.sheetBackdrop, { backgroundColor: theme.colors.overlay }]}>
          <View style={[s.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
            <Text style={[s.sheetTitle, { color: theme.colors.text }]}>
              {t("common.nav.switchProfile", "Switch profile")}
            </Text>
            <View style={{ gap: theme.spacing.xs }}>
              {profiles.map((p) => {
                const isActive = p.id === active.id;
                const avatarUrl = p.avatarPath && p.id && serverUrl && accessToken
                  ? `${serverUrl}/api/v1/profiles/${p.id}/avatar?token=${accessToken}`
                  : null;
                const itemInitial = (p.name ?? "?").charAt(0).toUpperCase();
                return (
                  <Pressable
                    key={p.id}
                    onPress={() => void handleSwitch(p)}
                    disabled={switching !== null || isActive}
                    style={[s.item, { borderRadius: theme.radii.sm, opacity: isActive ? 0.85 : 1 }]}
                    accessibilityRole="button"
                    accessibilityLabel={p.name}
                    accessibilityState={{ selected: isActive }}
                  >
                    <View style={[s.itemAvatar, { backgroundColor: theme.colors.accent }]}>
                      {avatarUrl ? (
                        <Image source={{ uri: avatarUrl }} style={{ width: 36, height: 36, borderRadius: 18 }} contentFit="cover" />
                      ) : (
                        <Text style={{ color: theme.colors.onAccent, fontSize: FONT_SIZE.sm, fontWeight: "600" }}>{itemInitial}</Text>
                      )}
                    </View>
                    <Text style={[s.itemName, { color: theme.colors.text }]} numberOfLines={1}>
                      {p.name}
                    </Text>
                    {p.isKids && (
                      <View style={[s.kidsBadge, { backgroundColor: theme.colors.success }]}>
                        <Text style={[s.kidsBadgeText, { color: theme.colors.onAccent }]}>
                          {t("auth.profile.manage.kidsBadge", "Kids")}
                        </Text>
                      </View>
                    )}
                    {isActive && (
                      <Ionicons name="checkmark" size={ICON_SIZE.lg} color={theme.colors.accent} />
                    )}
                    {switching === p.id && (
                      <ActivityIndicator size="small" color={theme.colors.accent} />
                    )}
                  </Pressable>
                );
              })}
            </View>
            <Pressable
              onPress={() => setSwitchOpen(false)}
              style={[s.closeBtn, { backgroundColor: theme.colors.bg, borderRadius: theme.radii.md }]}
              accessibilityRole="button"
              accessibilityLabel={t("common.close", "Close")}
            >
              <Text style={[s.closeBtnText, { color: theme.colors.text }]}>{t("common.close", "Close")}</Text>
            </Pressable>
          </View>
        </View>
      </Modal>

      {pinTarget && (
        <PinPromptDialog
          target={pinTarget}
          fallback={active}
          onCancel={() => setPinTarget(null)}
          onVerified={async (target, pinToken) => {
            setPinTarget(null);
            await doSwitch(target, pinToken);
          }}
        />
      )}
    </>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    trigger: {
      alignItems: "center",
      justifyContent: "center",
      overflow: "hidden",
    },
    backdrop: {
      ...StyleSheet.absoluteFillObject,
    },
    dropdown: {
      position: "absolute",
      top: 90,
      right: 12,
      minWidth: 220,
      borderWidth: 1,
      paddingVertical: 4,
      shadowColor: t.colors.shadow,
      shadowOffset: { width: 0, height: 4 },
      shadowOpacity: 0.2,
      shadowRadius: 8,
      elevation: 8,
    },
    menuItem: {
      flexDirection: "row",
      alignItems: "center",
      gap: 10,
      paddingHorizontal: 14,
      paddingVertical: 11,
    },
    menuLabel: {
      fontSize: FONT_SIZE.sm,
      fontWeight: "500",
    },
    divider: {
      height: 1,
      marginVertical: 2,
    },
    sheetBackdrop: {
      flex: 1,
      justifyContent: "center",
      alignItems: "center",
      padding: t.spacing.lg,
    },
    sheet: {
      width: "100%",
      maxWidth: 420,
      padding: t.spacing.lg,
      gap: t.spacing.md,
    },
    sheetTitle: {
      fontSize: FONT_SIZE.base,
      fontWeight: "700",
      marginBottom: 4,
    },
    item: {
      flexDirection: "row",
      alignItems: "center",
      gap: t.spacing.sm + 2,
      paddingVertical: 8,
      paddingHorizontal: 8,
    },
    itemAvatar: {
      width: 36,
      height: 36,
      borderRadius: 18,
      alignItems: "center",
      justifyContent: "center",
      overflow: "hidden",
    },
    itemName: {
      flex: 1,
      fontSize: FONT_SIZE.sm,
    },
    kidsBadge: {
      paddingHorizontal: 6,
      paddingVertical: 2,
      borderRadius: 4,
    },
    kidsBadgeText: {
      fontSize: FONT_SIZE.xs,
      fontWeight: "700",
      textTransform: "uppercase",
    },
    closeBtn: {
      paddingVertical: 12,
      alignItems: "center",
      justifyContent: "center",
    },
    closeBtnText: {
      fontSize: FONT_SIZE.sm,
      fontWeight: "600",
    },
  });
