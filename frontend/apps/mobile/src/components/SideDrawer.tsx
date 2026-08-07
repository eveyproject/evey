import { View, Text, Pressable, StyleSheet, Animated, Modal, ScrollView } from "react-native";
import { router, usePathname } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useAuthStore } from "@evey/store";
import { useMediaRequestServiceGetRequestPolicy } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { useDrawer } from "../providers/DrawerProvider";
import { EveyWordmark } from "./EveyLogo";
import { PluginSlot, usePluginSlotEntries } from "../features/plugins/PluginSlot";
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

// Mirror of web's AdminNav sections (features/admin/AdminNav.tsx), mapped to
// the existing mobile admin routes. Shown in place of the main nav while the
// user is inside /admin - one tap to any admin area, "back to app" to leave.
const ADMIN_SECTIONS: { titleKey: string; items: NavItem[] }[] = [
  {
    titleKey: "admin.nav.dashboardSystem",
    items: [
      { href: "/admin", labelKey: "admin.nav.dashboard", icon: "grid-outline", activeIcon: "grid" },
      { href: "/admin/sessions", labelKey: "admin.nav.nowPlaying", icon: "play-circle-outline", activeIcon: "play-circle" },
      { href: "/admin/remote-access", labelKey: "admin.nav.remoteAccess", icon: "globe-outline", activeIcon: "globe" },
      { href: "/admin/server-config", labelKey: "admin.nav.serverConfig", icon: "options-outline", activeIcon: "options" },
      { href: "/admin/transcoding", labelKey: "admin.nav.transcodingConfig", icon: "film-outline", activeIcon: "film" },
      { href: "/admin/system-health", labelKey: "admin.nav.systemHealth", icon: "hardware-chip-outline", activeIcon: "hardware-chip" },
    ],
  },
  {
    titleKey: "admin.nav.media",
    items: [
      { href: "/admin/libraries", labelKey: "admin.nav.libraries", icon: "library-outline", activeIcon: "library" },
      { href: "/admin/transcode-cache", labelKey: "admin.nav.transcodeCache", icon: "server-outline", activeIcon: "server" },
      { href: "/admin/artworks", labelKey: "admin.nav.artworks", icon: "image-outline", activeIcon: "image" },
      { href: "/admin/seek-index", labelKey: "admin.nav.seekIndex", icon: "search-outline", activeIcon: "search" },
    ],
  },
  {
    titleKey: "admin.nav.usersAccess",
    items: [
      { href: "/admin/users", labelKey: "admin.nav.users", icon: "people-outline", activeIcon: "people" },
      { href: "/admin/media-requests", labelKey: "admin.nav.mediaRequests", icon: "file-tray-full-outline", activeIcon: "file-tray-full" },
    ],
  },
  {
    titleKey: "admin.nav.integrations",
    items: [
      { href: "/admin/plugins", labelKey: "admin.nav.plugins", icon: "extension-puzzle-outline", activeIcon: "extension-puzzle" },
      { href: "/admin/webhooks", labelKey: "admin.nav.webhooks", icon: "git-network-outline", activeIcon: "git-network" },
      { href: "/admin/api-keys", labelKey: "admin.nav.apiKeys", icon: "key-outline", activeIcon: "key" },
    ],
  },
  {
    titleKey: "admin.nav.monitoring",
    items: [
      { href: "/admin/analytics", labelKey: "admin.nav.analytics", icon: "trending-up-outline", activeIcon: "trending-up" },
      { href: "/admin/activity", labelKey: "admin.nav.activity", icon: "bar-chart-outline", activeIcon: "bar-chart" },
      { href: "/admin/logs", labelKey: "admin.nav.logs", icon: "document-text-outline", activeIcon: "document-text" },
      { href: "/admin/qoe", labelKey: "admin.nav.qoe", icon: "speedometer-outline", activeIcon: "speedometer" },
    ],
  },
  {
    titleKey: "admin.nav.maintenance",
    items: [
      { href: "/admin/backup", labelKey: "admin.nav.backupRestore", icon: "archive-outline", activeIcon: "archive" },
    ],
  },
];

function isAdminLinkActive(pathname: string, href: string): boolean {
  if (href === "/admin") return pathname === "/admin" || pathname === "/admin/";
  return pathname === href || pathname.startsWith(href + "/");
}

export function SideDrawer() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { isOpen, close, translateX } = useDrawer();
  const pathname = usePathname();
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const { data: requestPolicy } = useMediaRequestServiceGetRequestPolicy();
  const pluginAdminEntries = usePluginSlotEntries("admin.nav");
  const navItems = requestPolicy?.canRequest
    ? NAV_ITEMS
    : NAV_ITEMS.filter((item) => item.href !== "/(auth)/(tabs)/requests");
  const isOnAdmin = pathname.startsWith("/admin");

  if (!isOpen) return null;

  function navigate(href: string) {
    close();
    setTimeout(() => router.push(href as never), 50);
  }

  const isActive = (href: string) => {
    if (href === "/(auth)/(tabs)/") return pathname === "/" || pathname.endsWith("/index") || pathname === "/(auth)/(tabs)/";
    return pathname.includes(href.replace("/(auth)/(tabs)", ""));
  };

  function drawerItem(item: NavItem, active: boolean) {
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
  }

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
            height. The user/logout entries live in the topbar UserMenu now.
            Inside /admin the tray swaps to the admin sections + "back to app",
            mirroring the web sidebar's two modes. */}
        <ScrollView style={styles.nav} contentContainerStyle={styles.navContent} showsVerticalScrollIndicator={false}>
          {isOnAdmin && isAdmin ? (
            <>
              <Pressable
                onPress={() => navigate("/(auth)/(tabs)/")}
                style={[styles.navItem, { borderLeftColor: "transparent" }]}
                accessible={true}
                accessibilityLabel={t("common.nav.backToApp")}
                accessibilityRole="menuitem"
              >
                <Ionicons name="arrow-back-outline" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
                <Text style={[styles.navLabel, { color: theme.colors.text }]}>{t("common.nav.backToApp")}</Text>
              </Pressable>
              {ADMIN_SECTIONS.map((section) => (
                <View key={section.titleKey}>
                  <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
                    {t(section.titleKey).toUpperCase()}
                  </Text>
                  {section.items.map((item) => drawerItem(item, isAdminLinkActive(pathname, item.href)))}
                </View>
              ))}
              {pluginAdminEntries.length > 0 && (
                <View>
                  <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
                    {t("admin.nav.plugins").toUpperCase()}
                  </Text>
                  {pluginAdminEntries.map((entry) => {
                    const href = `/admin/extensions/${entry.pluginId}`;
                    const active = isAdminLinkActive(pathname, href);
                    return (
                      <Pressable
                        key={entry.pluginId}
                        onPress={() => navigate(href)}
                        style={[
                          styles.navItem,
                          { borderLeftColor: active ? theme.colors.accent : "transparent" },
                          active && { backgroundColor: theme.colors.surfaceRaised },
                        ]}
                        accessible={true}
                        accessibilityLabel={entry.name}
                        accessibilityRole="menuitem"
                      >
                        <Ionicons
                          name={active ? "extension-puzzle" : "extension-puzzle-outline"}
                          size={ICON_SIZE.lg}
                          color={active ? theme.colors.accent : theme.colors.textMuted}
                        />
                        <Text style={[styles.navLabel, { color: active ? theme.colors.accent : theme.colors.text }]}>
                          {entry.name}
                        </Text>
                      </Pressable>
                    );
                  })}
                </View>
              )}
            </>
          ) : (
            <>
              {navItems.map((item) => drawerItem(item, isActive(item.href)))}

              {/* Admin link - only if admin */}
              {isAdmin && (
                <Pressable
                  onPress={() => navigate("/admin")}
                  style={[styles.navItem, { borderLeftColor: "transparent" }]}
                  accessible={true}
                  accessibilityLabel={t("common.nav.admin")}
                  accessibilityRole="menuitem"
                >
                  <Ionicons name="settings-outline" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
                  <Text style={[styles.navLabel, { color: theme.colors.text }]}>
                    {t("common.nav.admin")}
                  </Text>
                </Pressable>
              )}
            </>
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
  sectionTitle: {
    fontSize: 11,
    fontWeight: "600",
    letterSpacing: 1,
    paddingHorizontal: 14,
    paddingTop: 14,
    paddingBottom: 4,
  },
});
