import React, { useState, useCallback, useEffect } from "react";
import { View, Text, Pressable, StyleSheet, DeviceEventEmitter } from "react-native";
import { Slot, useRouter, usePathname } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { EveyWordmark, EveyMark } from "../../../src/components/EveyLogo";
import { TopHeader } from "../../../src/components/TopHeader";
import { SearchOverlay } from "../../../src/components/SearchOverlay";

interface NavItem {
  labelKey: string;
  icon: keyof typeof Ionicons.glyphMap;
  path: string;
}

// Navigation only. Search, notifications and the profile moved to the header
// (TopHeader, owner's call 2026-09-19): they are actions, they belong where
// the eye looks for them, and two focus stops for one destination is one too
// many on a remote.
const NAV_ITEMS: NavItem[] = [
  { labelKey: "common.nav.home", icon: "home", path: "/(auth)/(tabs)" },
  { labelKey: "common.nav.libraries", icon: "library", path: "/(auth)/(tabs)/library" },
  { labelKey: "common.nav.music", icon: "musical-notes", path: "/(auth)/(tabs)/music" },
  { labelKey: "common.nav.favorites", icon: "heart", path: "/(auth)/(tabs)/favorites" },
  { labelKey: "common.nav.requests", icon: "mail-unread", path: "/(auth)/(tabs)/requests" },
  { labelKey: "common.nav.settings", icon: "settings", path: "/(auth)/(tabs)/settings" },
];

export default function TabsLayout() {
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const pathname = usePathname();
  const [expanded, setExpanded] = useState(false);
  const [focusedIndex, setFocusedIndex] = useState(-1);
  // The search lives here, above every tab screen: it is a dropdown of the
  // header, so it opens over whatever the viewer was looking at instead of
  // taking them to another page.
  const [searchOpen, setSearchOpen] = useState(false);

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

  // The remote's SEARCH button, forwarded by MainActivity (see
  // plugins/withSearchKey.js). Many televisions swallow it for the system
  // assistant and it never arrives; where it does, it opens the search.
  useEffect(() => {
    const sub = DeviceEventEmitter.addListener("eveyRemoteSearch", () => setSearchOpen(true));
    return () => sub.remove();
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
        {/* Brand, same shape as the web sidebar: the wordmark when the bar is
            expanded, the compact mark when it is not. Not focusable - on a
            remote every stop costs a press, and a logo does nothing. */}
        <View style={styles.brand}>
          {expanded ? (
            <EveyWordmark height={20} color={theme.colors.text} cutColor={theme.colors.surface} />
          ) : (
            <EveyMark height={24} cutColor={theme.colors.surface} />
          )}
        </View>
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
                <Ionicons
                  name={item.icon}
                  size={28}
                  color={active || focused ? theme.colors.accent : theme.colors.textMuted}
                />
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
        <TopHeader
          onSearch={() => setSearchOpen(true)}
          onNotifications={() => router.push("/(auth)/notifications" as never)}
          onProfile={() => router.push("/profile-select" as never)}
        />
        <Slot />
      </View>

      <SearchOverlay visible={searchOpen} onClose={() => setSearchOpen(false)} />
    </View>
  );
}

const styles = StyleSheet.create({
  brand: { height: 56, justifyContent: "center", alignItems: "center", marginBottom: 20 },
  root: {
    flex: 1,
    flexDirection: "row",
  },
  sidebar: {
    borderRightWidth: 1,
    paddingTop: 24,
  },
  // No vertical centring any more. With eight entries the list is taller than
  // the space left under the brand, and a centred box that overflows spills
  // BOTH ways - the first item climbed on top of the logo. Top-aligned, it
  // simply runs down the bar.
  navItems: {
    flex: 1,
    gap: 4,
  },
  navButton: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 9,
    paddingHorizontal: 20,
    borderLeftWidth: 3,
    borderLeftColor: "transparent",
  },
  navLabel: {
    fontSize: 17,
    fontWeight: "600",
    marginLeft: 16,
  },
  content: {
    flex: 1,
  },
});
