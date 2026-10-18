import { useRef } from "react";
import { View, Pressable, StyleSheet, ScrollView } from "react-native";
import { router, usePathname } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useAuthStore } from "@evey/store";
import { useMediaRequestServiceGetRequestPolicy } from "@evey/api";
import { ICON_SIZE } from "@evey/theme";
import { useTheme } from "../../providers/ThemeProvider";
import { useDrawer } from "../../providers/DrawerProvider";
import { EveyMark } from "../EveyLogo";
import { usePluginSlotEntries } from "../../features/plugins/PluginSlot";
import {
  NAV_ITEMS,
  ADMIN_SECTIONS,
  isAdminLinkActive,
  isMainLinkActive,
  type NavItem,
} from "../SideDrawer";

export const RAIL_WIDTH = 52;

// Permanent icon rail - the mobile twin of the web sidebar's collapsed state:
// the icons never leave the screen, and the brand mark on top expands the full
// drawer (icons + labels) over it. Mounted beside the (tabs) and admin stacks.
export function NavRail({ mode }: { mode: "main" | "admin" }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const { open } = useDrawer();
  const pathname = usePathname();
  const insets = useSafeAreaInsets();
  const logoRef = useRef<View>(null);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const { data: requestPolicy } = useMediaRequestServiceGetRequestPolicy();
  const pluginAdminEntries = usePluginSlotEntries("admin.nav");

  const mainItems = requestPolicy?.canRequest
    ? NAV_ITEMS
    : NAV_ITEMS.filter((item) => item.href !== "/(auth)/(tabs)/requests");

  function railIcon(
    key: string,
    icon: keyof typeof Ionicons.glyphMap,
    label: string,
    active: boolean,
    onPress: () => void,
  ) {
    return (
      <Pressable
        key={key}
        onPress={onPress}
        style={[styles.iconBtn, active && { backgroundColor: theme.colors.surfaceRaised, borderRadius: 8 }]}
        accessible={true}
        accessibilityLabel={label}
        accessibilityRole="menuitem"
      >
        <Ionicons name={icon} size={ICON_SIZE.lg} color={active ? theme.colors.accent : theme.colors.textMuted} />
      </Pressable>
    );
  }

  return (
    <View
      style={[
        styles.rail,
        {
          width: RAIL_WIDTH,
          backgroundColor: theme.colors.surface,
          borderRightColor: theme.colors.border,
          paddingTop: insets.top + 8,
          paddingBottom: insets.bottom + 8,
        },
      ]}
    >
      <Pressable
        ref={logoRef}
        onPress={() => open(logoRef.current)}
        style={styles.logoBtn}
        accessible={true}
        accessibilityLabel={t("common.nav.expandSidebar", "Expand sidebar")}
        accessibilityRole="button"
      >
        <EveyMark height={24} />
      </Pressable>
      <ScrollView contentContainerStyle={styles.icons} showsVerticalScrollIndicator={false}>
        {mode === "admin" ? (
          <>
            {railIcon("back-to-app", "arrow-back-outline", t("common.nav.backToApp"), false, () =>
              router.push("/(auth)/(tabs)/" as never))}
            {ADMIN_SECTIONS.flatMap((s) => s.items).map((item: NavItem) => {
              const active = isAdminLinkActive(pathname, item.href);
              return railIcon(item.href, active ? item.activeIcon : item.icon, t(item.labelKey), active, () =>
                router.push(item.href as never));
            })}
            {pluginAdminEntries.map((entry) => {
              const href = `/admin/extensions/${entry.pluginId}`;
              const active = isAdminLinkActive(pathname, href);
              return railIcon(href, active ? "extension-puzzle" : "extension-puzzle-outline", entry.name, active, () =>
                router.push(href as never));
            })}
          </>
        ) : (
          <>
            {mainItems.map((item: NavItem) => {
              const active = isMainLinkActive(pathname, item.href);
              return railIcon(item.href, active ? item.activeIcon : item.icon, t(item.labelKey), active, () =>
                router.push(item.href as never));
            })}
            {isAdmin &&
              railIcon("admin", "settings-outline", t("common.nav.admin"), false, () =>
                router.push("/admin" as never))}
          </>
        )}
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  rail: {
    borderRightWidth: StyleSheet.hairlineWidth,
    alignItems: "stretch",
  },
  logoBtn: {
    alignItems: "center",
    paddingVertical: 10,
  },
  icons: {
    paddingHorizontal: 4,
    paddingBottom: 12,
    gap: 2,
  },
  iconBtn: {
    height: 44,
    alignItems: "center",
    justifyContent: "center",
  },
});
