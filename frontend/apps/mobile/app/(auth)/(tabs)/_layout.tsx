import { View, Text, Pressable, StyleSheet, Modal } from "react-native";
import { Image } from "expo-image";
import { Stack, router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNotificationServiceGetUnreadCount, useUserServiceGetCurrentUser, useUserServiceListProfiles, eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useDrawer } from "../../../src/providers/DrawerProvider";
import { HeaderSearch } from "../../../src/components/HeaderSearch";
import { useServerUrl } from "../../../src/hooks/useServerUrl";

function BurgerButton({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { toggle } = useDrawer();
  return (
    <Pressable onPress={toggle} style={{ marginLeft: 16, padding: 4 }} accessibilityRole="button" accessibilityLabel="Open menu">
      <Ionicons name="menu" size={24} color={theme.colors.text} />
    </Pressable>
  );
}

function UserMenu({ theme, initial, avatarUrl }: { theme: ReturnType<typeof useTheme>; initial: string; avatarUrl: string | null }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const logout = useAuthStore((s) => s.logout);
  const isAdmin = useAuthStore((s) => s.isAdmin);

  function go(path: string) {
    setOpen(false);
    setTimeout(() => router.push(path as never), 50);
  }

  function handleLogout() {
    setOpen(false);
    logout();
    void eveyFetch<void>({ url: "/api/v1/auth/logout", method: "POST", data: {} }).catch(() => undefined);
    setTimeout(() => router.replace("/login"), 50);
  }

  return (
    <>
      <Pressable
        onPress={() => setOpen(true)}
        style={[styles.avatar, { backgroundColor: theme.colors.accent, overflow: "hidden" }]}
        accessibilityRole="button"
        accessibilityLabel={t("common.nav.userMenu")}
      >
        {avatarUrl ? (
          <Image source={{ uri: avatarUrl }} style={{ width: 30, height: 30, borderRadius: 15 }} contentFit="cover" />
        ) : (
          <Text style={{ color: "#fff", fontSize: 13, fontWeight: "600" }}>{initial}</Text>
        )}
      </Pressable>

      <Modal transparent visible={open} animationType="fade" onRequestClose={() => setOpen(false)}>
        <Pressable style={styles.menuBackdrop} onPress={() => setOpen(false)} />
        <View style={[styles.menuDropdown, { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border }]}>
          {[
            { label: t("common.nav.settings"), icon: "settings-outline", path: "/(auth)/(tabs)/settings" },
            { label: t("common.nav.watchHistory"), icon: "time-outline", path: "/history" },
            { label: t("common.nav.quickConnect"), icon: "qr-code-outline", path: "/quick-connect" },
            ...(isAdmin ? [{ label: t("common.nav.admin"), icon: "grid-outline", path: "/admin" }] : []),
          ].map((item) => (
            <Pressable key={item.path} onPress={() => go(item.path)} style={styles.menuItem}>
              <Ionicons name={item.icon as never} size={16} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.text, fontSize: 14 }}>{item.label}</Text>
            </Pressable>
          ))}
          <View style={[styles.menuDivider, { backgroundColor: theme.colors.border }]} />
          <Pressable onPress={handleLogout} style={styles.menuItem}>
            <Ionicons name="log-out-outline" size={16} color="#e57373" />
            <Text style={{ color: "#e57373", fontSize: 14 }}>{t("common.nav.signOut")}</Text>
          </Pressable>
        </View>
      </Modal>
    </>
  );
}

function HeaderRight({ theme, serverUrl }: { theme: ReturnType<typeof useTheme>; serverUrl: string }) {
  const { data: notifData } = useNotificationServiceGetUnreadCount();
  const { data: currentUser } = useUserServiceGetCurrentUser();
  const accessToken = useAuthStore((s) => s.accessToken);
  const { data: profilesResp } = useUserServiceListProfiles(currentUser?.id ?? "", { query: { enabled: !!currentUser?.id } });
  const defaultProfile = profilesResp?.profiles?.find((p) => p.isDefault) ?? profilesResp?.profiles?.[0];
  const count = notifData?.count ?? 0;
  const initial = currentUser?.username?.charAt(0).toUpperCase() ?? "U";
  const avatarUrl = defaultProfile?.avatarPath && defaultProfile?.id && serverUrl && accessToken
    ? `${serverUrl}/api/v1/profiles/${defaultProfile.id}/avatar?token=${accessToken}`
    : null;

  return (
    <View style={{ flexDirection: "row", alignItems: "center", gap: 4, marginRight: 8 }}>
      <Pressable onPress={() => router.push("/notifications")} style={{ padding: 6 }}>
        <Ionicons name="notifications-outline" size={22} color={theme.colors.text} />
        {count > 0 && (
          <View style={[styles.badge, { backgroundColor: theme.colors.accent }]}>
            <Text style={styles.badgeText}>{count > 9 ? "9+" : count}</Text>
          </View>
        )}
      </Pressable>
      <UserMenu theme={theme} initial={initial} avatarUrl={avatarUrl} />
    </View>
  );
}

export default function TabsLayout() {
  const theme = useTheme();
  const serverUrl = useServerUrl();

  const sharedOptions = {
    headerStyle: { backgroundColor: theme.colors.surface },
    headerShadowVisible: false,
    headerLeft: () => <BurgerButton theme={theme} />,
    headerTitle: () => <HeaderSearch />,
    headerRight: () => <HeaderRight theme={theme} serverUrl={serverUrl} />,
    headerTitleContainerStyle: { flex: 1, paddingHorizontal: 8 },
  };

  return (
    <Stack>
      <Stack.Screen name="index" options={sharedOptions} />
      <Stack.Screen name="search" options={sharedOptions} />
      <Stack.Screen name="library" options={sharedOptions} />
      <Stack.Screen name="photos" options={sharedOptions} />
      <Stack.Screen name="music" options={sharedOptions} />
      <Stack.Screen name="favorites" options={sharedOptions} />
      <Stack.Screen name="settings" options={sharedOptions} />
    </Stack>
  );
}

const styles = StyleSheet.create({
  badge: {
    position: "absolute",
    top: 2,
    right: 2,
    minWidth: 15,
    height: 15,
    borderRadius: 8,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 2,
  },
  badgeText: { color: "#fff", fontSize: 9, fontWeight: "700" },
  avatar: {
    width: 30,
    height: 30,
    borderRadius: 15,
    alignItems: "center",
    justifyContent: "center",
  },
  menuBackdrop: {
    ...StyleSheet.absoluteFillObject,
  },
  menuDropdown: {
    position: "absolute",
    top: 100,
    right: 12,
    minWidth: 200,
    borderRadius: 10,
    borderWidth: 1,
    overflow: "hidden",
    elevation: 8,
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.2,
    shadowRadius: 8,
  },
  menuItem: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    paddingHorizontal: 14,
    paddingVertical: 11,
  },
  menuDivider: {
    height: 1,
    marginVertical: 2,
  },
});
