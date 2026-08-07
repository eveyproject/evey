// The "(tabs)" segment name is a historical leftover from an earlier bottom-tab
// navigation design. The route group was kept to avoid breaking deep links and
// expo-router's generated routes, but the actual navigation UI is the NavRail
// (permanent icon column, web sidebar collapsed state) plus the SideDrawer it
// expands to; this layout itself renders a plain Stack, not a Tabs navigator.
import { View, Text, Pressable, StyleSheet } from "react-native";
import { Stack, router } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useNotificationServiceGetUnreadCount } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { HeaderSearch } from "../../../src/components/HeaderSearch";
import { NavRail } from "../../../src/components/layout/NavRail";
import { UserMenu } from "../../../src/components/layout/UserMenu";

function HeaderRight({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { t } = useTranslation();
  const { data: notifData } = useNotificationServiceGetUnreadCount();
  const count = notifData?.count ?? 0;

  return (
    <View style={{ flexDirection: "row", alignItems: "center", gap: 4, marginRight: 2 }}>
      <Pressable
        onPress={() => router.push("/notifications")}
        style={{ padding: 6 }}
        accessibilityRole="button"
        accessibilityLabel={t("common.notifications.title")}
      >
        <Ionicons name="notifications-outline" size={22} color={theme.colors.text} />
        {count > 0 && (
          <View style={[styles.badge, { backgroundColor: theme.colors.accent }]}>
            <Text style={[styles.badgeText, { color: theme.colors.onAccent }]}>{count > 9 ? "9+" : count}</Text>
          </View>
        )}
      </Pressable>
      <UserMenu />
    </View>
  );
}

export default function TabsLayout() {
  const theme = useTheme();

  const sharedOptions = {
    headerStyle: { backgroundColor: theme.colors.surface },
    headerShadowVisible: false,
    // The rail carries the brand and the navigation - the header keeps only
    // search and the user cluster, like the web topbar next to the sidebar.
    headerLeft: () => null,
    headerTitle: () => <HeaderSearch />,
    headerRight: () => <HeaderRight theme={theme} />,
    headerTitleContainerStyle: { flex: 1, paddingHorizontal: 8 },
  };

  return (
    <View style={{ flex: 1, flexDirection: "row", backgroundColor: theme.colors.surface }}>
      <NavRail mode="main" />
      <View style={{ flex: 1 }}>
        <Stack>
          <Stack.Screen name="index" options={sharedOptions} />
          <Stack.Screen name="search" options={sharedOptions} />
          <Stack.Screen name="library" options={sharedOptions} />
          <Stack.Screen name="photos" options={sharedOptions} />
          <Stack.Screen name="music" options={sharedOptions} />
          <Stack.Screen name="favorites" options={sharedOptions} />
          <Stack.Screen name="requests" options={sharedOptions} />
          <Stack.Screen name="settings" options={sharedOptions} />
        </Stack>
      </View>
    </View>
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
  badgeText: { fontSize: 9, fontWeight: "700" },
});
