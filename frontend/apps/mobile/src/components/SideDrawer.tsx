import { Text, Pressable, StyleSheet, Animated, Modal, ScrollView } from "react-native";
import { router, usePathname } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { useMediaRequestServiceGetRequestPolicy } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { useDrawer } from "../providers/DrawerProvider";
import { EveyWordmark } from "./EveyLogo";
import { PluginSlot } from "../features/plugins/PluginSlot";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

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
  { href: "/(auth)/(tabs)/requests", labelKey: "requests.title", icon: "mail-unread-outline", activeIcon: "mail-open" },
  { href: "/(auth)/(tabs)/music", labelKey: "common.nav.music", icon: "musical-notes-outline", activeIcon: "musical-notes" },
  { href: "/(auth)/syncplay", labelKey: "common.nav.syncplay", icon: "people-outline", activeIcon: "people" },
  { href: "/(auth)/downloads", labelKey: "downloads.title", icon: "download-outline", activeIcon: "download" },
];

export function SideDrawer() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { isOpen, close, translateX } = useDrawer();
  const pathname = usePathname();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const { data: requestPolicy } = useMediaRequestServiceGetRequestPolicy();
  const navItems = requestPolicy?.canRequest
    ? NAV_ITEMS
    : NAV_ITEMS.filter((item) => item.href !== "/(auth)/(tabs)/requests");

  if (!isOpen) return null;

  function navigate(href: string) {
    close();
    setTimeout(() => router.push(href as never), 50);
  }

  const isActive = (href: string) => {
    if (href === "/(auth)/(tabs)/") return pathname === "/" || pathname.endsWith("/index") || pathname === "/(auth)/(tabs)/";
    return pathname.includes(href.replace("/(auth)/(tabs)", ""));
  };

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
          <EveyWordmark height={24} color={theme.colors.text} />
        </Pressable>

        {/* Nav items - scrollable so they survive landscape's compressed
            height. The user/logout entries live in the topbar UserMenu now. */}
        <ScrollView style={styles.nav} contentContainerStyle={styles.navContent} showsVerticalScrollIndicator={false}>
          {navItems.map((item) => {
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
                  size={ICON_SIZE.lg}
                  color={active ? theme.colors.accent : theme.colors.textMuted}
                />
                <Text style={[styles.navLabel, { color: active ? theme.colors.accent : theme.colors.text }]}>
                  {t(item.labelKey)}
                </Text>
              </Pressable>
            );
          })}

          {/* Admin link - only if admin */}
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
                size={ICON_SIZE.lg}
                color={pathname.startsWith("/admin") ? theme.colors.accent : theme.colors.textMuted}
              />
              <Text style={[styles.navLabel, { color: pathname.startsWith("/admin") ? theme.colors.accent : theme.colors.text }]}>
                {t("common.nav.admin")}
              </Text>
            </Pressable>
          )}

          {/* Plugins mounting in the navigation tray bottom - same slot as
              web's Sidebar. */}
          <PluginSlot name="sidebar.bottom" />
        </ScrollView>
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
  nav: {
    flex: 1,
  },
  navContent: {
    paddingHorizontal: 8,
    paddingBottom: 24,
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
    fontSize: FONT_SIZE.sm,
    fontWeight: "500",
  },
});
