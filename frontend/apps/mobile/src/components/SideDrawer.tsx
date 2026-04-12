import { View, Text, Pressable, StyleSheet, Animated, Modal } from "react-native";
import { router, usePathname } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useUserServiceGetCurrentUser, eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";
import { useDrawer } from "../providers/DrawerProvider";

const DRAWER_WIDTH = 260;

type NavItem = {
  href: string;
  labelKey: string;
  icon: keyof typeof Ionicons.glyphMap;
  activeIcon: keyof typeof Ionicons.glyphMap;
};

const NAV_ITEMS: NavItem[] = [
  { href: "/(auth)/(tabs)/", labelKey: "common.nav.home", icon: "home-outline", activeIcon: "home" },
  { href: "/(auth)/(tabs)/library", labelKey: "common.nav.libraries", icon: "library-outline", activeIcon: "library" },
  { href: "/(auth)/(tabs)/photos", labelKey: "common.nav.photos", icon: "images-outline", activeIcon: "images" },
  { href: "/(auth)/(tabs)/favorites", labelKey: "media.favorites.title", icon: "heart-outline", activeIcon: "heart" },
  { href: "/(auth)/(tabs)/music", labelKey: "common.nav.music", icon: "musical-notes-outline", activeIcon: "musical-notes" },
];

export function SideDrawer() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { isOpen, close, translateX } = useDrawer();
  const pathname = usePathname();
  const { data: currentUser } = useUserServiceGetCurrentUser();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const logout = useAuthStore((s) => s.logout);

  if (!isOpen) return null;

  function navigate(href: string) {
    close();
    setTimeout(() => router.push(href as never), 50);
  }

  function handleLogout() {
    close();
    logout();
    void eveyFetch<void>({ url: "/api/v1/auth/logout", method: "POST", data: {} }).catch((err) => console.warn("caught", err));
    setTimeout(() => router.replace("/login"), 50);
  }

  const isActive = (href: string) => {
    if (href === "/(auth)/(tabs)/") return pathname === "/" || pathname.endsWith("/index") || pathname === "/(auth)/(tabs)/";
    return pathname.includes(href.replace("/(auth)/(tabs)", ""));
  };

  const initial = currentUser?.username?.charAt(0).toUpperCase() ?? "U";

  return (
    <Modal transparent animationType="none" visible={isOpen} onRequestClose={close}>
      <Pressable style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]} onPress={close} accessible accessibilityLabel={t("common.close", "Close")} accessibilityRole="button" />
      <Animated.View
        style={[
          styles.drawer,
          {
            width: DRAWER_WIDTH,
            backgroundColor: theme.colors.surface,
            borderRightColor: theme.colors.border,
            transform: [{ translateX }],
          },
        ]}
      >
        {/* Logo */}
        <Pressable onPress={() => navigate("/(auth)/(tabs)/")} style={styles.logoRow} accessible={true} accessibilityLabel="Evey home" accessibilityRole="button">
          <Text style={[styles.logo, { color: theme.colors.accent }]}>Evey</Text>
        </Pressable>

        {/* Nav items */}
        <View style={styles.nav}>
          {NAV_ITEMS.map((item) => {
            const active = isActive(item.href);
            return (
              <Pressable
                key={item.href}
                onPress={() => navigate(item.href)}
                style={[
                  styles.navItem,
                  { borderLeftColor: active ? theme.colors.accent : "transparent" },
                  active && { backgroundColor: theme.colors.surfaceRaised },
                ]}
                accessible={true}
                accessibilityLabel={t(item.labelKey)}
                accessibilityRole="menuitem"
              >
                <Ionicons
                  name={active ? item.activeIcon : item.icon}
                  size={18}
                  color={active ? theme.colors.accent : theme.colors.textMuted}
                />
                <Text style={[styles.navLabel, { color: active ? theme.colors.accent : theme.colors.text }]}>
                  {t(item.labelKey)}
                </Text>
              </Pressable>
            );
          })}

          {/* Admin link — only if admin */}
          {isAdmin && (
            <Pressable
              onPress={() => navigate("/admin")}
              style={[
                styles.navItem,
                { borderLeftColor: pathname.startsWith("/admin") ? theme.colors.accent : "transparent" },
                pathname.startsWith("/admin") && { backgroundColor: theme.colors.surfaceRaised },
              ]}
              accessible={true}
              accessibilityLabel={t("common.nav.admin")}
              accessibilityRole="menuitem"
            >
              <Ionicons
                name={pathname.startsWith("/admin") ? "settings" : "settings-outline"}
                size={18}
                color={pathname.startsWith("/admin") ? theme.colors.accent : theme.colors.textMuted}
              />
              <Text style={[styles.navLabel, { color: pathname.startsWith("/admin") ? theme.colors.accent : theme.colors.text }]}>
                {t("common.nav.admin")}
              </Text>
            </Pressable>
          )}
        </View>

        {/* User section at bottom */}
        <View style={[styles.userSection, { borderTopColor: theme.colors.border }]}>
          <Pressable onPress={() => navigate("/(auth)/(tabs)/settings")} style={styles.userRow} accessible={true} accessibilityLabel={currentUser?.username ?? "Settings"} accessibilityRole="button">
            <View style={[styles.avatar, { backgroundColor: theme.colors.accent }]}>
              <Text style={[styles.avatarText, { color: theme.colors.text }]}>{initial}</Text>
            </View>
            <View style={{ flex: 1 }}>
              <Text style={[styles.username, { color: theme.colors.text }]} numberOfLines={1}>
                {currentUser?.username ?? "…"}
              </Text>
              {isAdmin && (
                <Text style={[styles.adminBadge, { color: theme.colors.accent }]}>Admin</Text>
              )}
            </View>
            <Ionicons name="chevron-forward" size={16} color={theme.colors.textMuted} />
          </Pressable>
          <Pressable onPress={handleLogout} style={styles.logoutRow} accessible={true} accessibilityLabel={t("common.nav.signOut", "Sign out")} accessibilityRole="button">
            <Ionicons name="log-out-outline" size={18} color={theme.colors.error} />
            <Text style={[styles.navLabel, { color: theme.colors.error }]}>{t("common.nav.signOut", "Sign out")}</Text>
          </Pressable>
        </View>
      </Animated.View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    ...StyleSheet.absoluteFillObject,
  },
  drawer: {
    position: "absolute",
    top: 0,
    left: 0,
    bottom: 0,
    borderRightWidth: 1,
    flexDirection: "column",
  },
  logoRow: {
    paddingHorizontal: 20,
    paddingTop: 52,
    paddingBottom: 20,
  },
  logo: {
    fontSize: 22,
    fontWeight: "800",
    letterSpacing: -0.5,
  },
  nav: {
    flex: 1,
    paddingHorizontal: 8,
  },
  navItem: {
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
    paddingHorizontal: 14,
    paddingVertical: 11,
    borderRadius: 8,
    borderLeftWidth: 2,
    marginBottom: 2,
  },
  navLabel: {
    fontSize: 14,
    fontWeight: "500",
  },
  userSection: {
    borderTopWidth: 1,
    paddingHorizontal: 8,
    paddingVertical: 8,
    paddingBottom: 24,
  },
  userRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    paddingHorizontal: 14,
    paddingVertical: 10,
    borderRadius: 8,
  },
  avatar: {
    width: 32,
    height: 32,
    borderRadius: 16,
    alignItems: "center",
    justifyContent: "center",
  },
  avatarText: { fontSize: 14, fontWeight: "600" },
  username: { fontSize: 14, fontWeight: "500" },
  adminBadge: { fontSize: 11, fontWeight: "600", marginTop: 1 },
  logoutRow: {
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
    paddingHorizontal: 14,
    paddingVertical: 10,
    borderRadius: 8,
    marginTop: 2,
  },
});
