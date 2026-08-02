import React, { useState, useCallback } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { Slot, useRouter, usePathname } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useNotificationServiceGetUnreadCount } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";

interface NavItem {
  labelKey: string;
  icon: keyof typeof Ionicons.glyphMap;
  path: string;
}

const NAV_ITEMS: NavItem[] = [
  { labelKey: "common.nav.home", icon: "home", path: "/(auth)/(tabs)" },
  { labelKey: "common.nav.search", icon: "search", path: "/(auth)/(tabs)/search" },
  { labelKey: "common.nav.libraries", icon: "library", path: "/(auth)/(tabs)/library" },
  { labelKey: "common.nav.music", icon: "musical-notes", path: "/(auth)/(tabs)/music" },
  { labelKey: "common.nav.favorites", icon: "heart", path: "/(auth)/(tabs)/favorites" },
  { labelKey: "common.nav.requests", icon: "mail-unread", path: "/(auth)/(tabs)/requests" },
  // A bell, like the web top bar. Notifications had no entry at all: you could
  // only reach them through the unread banner on the home, so once you had read
  // everything the screen became unreachable.
  { labelKey: "common.notifications.title", icon: "notifications", path: "/(auth)/notifications" },
  { labelKey: "common.nav.settings", icon: "settings", path: "/(auth)/(tabs)/settings" },
];

export default function TabsLayout() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const pathname = usePathname();
  const [expanded, setExpanded] = useState(false);
  // A failed count must never blank the navigation - the bar is how you leave
  // any screen. No badge is the honest degradation: the bell still works, it
  // just stops claiming there is something new.
  const { data: unread, isError: unreadFailed } = useNotificationServiceGetUnreadCount();
  const unreadCount = unreadFailed ? 0 : (unread?.count ?? 0);
  const [focusedIndex, setFocusedIndex] = useState(-1);

  const isActive = useCallback(
    (path: string) => {
      if (path === "/(auth)/(tabs)") return pathname === "/" || pathname === "";
      const segment = path.split("/").pop() ?? "";
      return pathname.includes(segment);
    },
    [pathname],
  );

  const handleNavFocus = useCallback((index: number) => {
    setFocusedIndex(index);
    setExpanded(true);
  }, []);

  const handleNavBlur = useCallback(() => {
    setFocusedIndex(-1);
    setExpanded(false);
  }, []);

  return (
    <View style={[styles.root, { backgroundColor: theme.colors.bg }]}>
      <View
        style={[
          styles.sidebar,
          {
            width: expanded ? 220 : 72,
            backgroundColor: theme.colors.surface,
            borderRightColor: theme.colors.border,
          },
        ]}
      >
        <View style={styles.navItems}>
          {NAV_ITEMS.map((item, index) => {
            const active = isActive(item.path);
            const focused = focusedIndex === index;
            return (
              <Pressable
                key={item.path}
                onPress={() => router.push(item.path as never)}
                onFocus={() => handleNavFocus(index)}
                onBlur={handleNavBlur}
                hasTVPreferredFocus={index === 0}
                accessible={true}
                accessibilityLabel={t(item.labelKey)}
                accessibilityRole="button"
                style={[
                  styles.navButton,
                  focused && { backgroundColor: theme.colors.accent + "33" },
                  active && { borderLeftColor: theme.colors.accent, borderLeftWidth: 3 },
                ]}
              >
                <View>
                  <Ionicons
                    name={item.icon}
                    size={28}
                    color={active || focused ? theme.colors.accent : theme.colors.textMuted}
                  />
                  {/* Unread marker on the bell. A dot rather than a number:
                      at three metres a count is unreadable, and the only
                      question that matters from the sofa is "is there
                      anything new". */}
                  {item.icon === "notifications" && unreadCount > 0 ? (
                    <View style={[styles.badge, { backgroundColor: theme.colors.accent, borderColor: theme.colors.bg }]} />
                  ) : null}
                </View>
                {expanded && (
                  <Text
                    style={[
                      styles.navLabel,
                      { color: active || focused ? theme.colors.accent : theme.colors.textMuted },
                    ]}
                  >
                    {t(item.labelKey)}
                  </Text>
                )}
              </Pressable>
            );
          })}
        </View>
      </View>

      <View style={styles.content}>
        <Slot />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  badge: { position: "absolute", top: -2, right: -2, width: 12, height: 12, borderRadius: 6, borderWidth: 2 },
  root: {
    flex: 1,
    flexDirection: "row",
  },
  sidebar: {
    borderRightWidth: 1,
    paddingTop: 40,
  },
  navItems: {
    flex: 1,
    justifyContent: "center",
    gap: 8,
  },
  navButton: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 16,
    paddingHorizontal: 20,
    borderLeftWidth: 3,
    borderLeftColor: "transparent",
  },
  navLabel: {
    fontSize: 18,
    fontWeight: "600",
    marginLeft: 16,
  },
  content: {
    flex: 1,
  },
});
