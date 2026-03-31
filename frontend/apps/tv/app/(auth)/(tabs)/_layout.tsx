import React, { useState, useCallback } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { Slot, useRouter, usePathname } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTheme } from "../../../src/providers/ThemeProvider";

interface NavItem {
  label: string;
  icon: keyof typeof Ionicons.glyphMap;
  path: string;
}

const NAV_ITEMS: NavItem[] = [
  { label: "Home", icon: "home", path: "/(auth)/(tabs)" },
  { label: "Search", icon: "search", path: "/(auth)/(tabs)/search" },
  { label: "Library", icon: "library", path: "/(auth)/(tabs)/library" },
  { label: "Favorites", icon: "heart", path: "/(auth)/(tabs)/favorites" },
  { label: "Settings", icon: "settings", path: "/(auth)/(tabs)/settings" },
];

export default function TabsLayout() {
  const theme = useTheme();
  const router = useRouter();
  const pathname = usePathname();
  const [expanded, setExpanded] = useState(false);
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
                accessibilityLabel={item.label}
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
                    {item.label}
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
