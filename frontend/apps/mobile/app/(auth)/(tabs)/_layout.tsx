// The "(tabs)" segment name is a historical leftover from an earlier bottom-tab
// navigation design. The route group was kept to avoid breaking deep links and
// expo-router's generated routes, but the actual navigation UI is the
// SideDrawer (see src/components/SideDrawer.tsx) opened via BurgerButton
// below; this layout itself renders a plain Stack, not a Tabs navigator.
import { View, Text, Pressable, StyleSheet } from "react-native";
import { Stack, router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useNotificationServiceGetUnreadCount } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useDrawer } from "../../../src/providers/DrawerProvider";
import { HeaderSearch } from "../../../src/components/HeaderSearch";
import { EveyMark } from "../../../src/components/EveyLogo";
import { UserMenu } from "../../../src/components/layout/UserMenu";

// The brand lives with the menu, never on a page: closed it is the compact mark
// that opens the drawer, open it is the wordmark at the drawer's head - the
// same two states the web sidebar shows collapsed and expanded.
function BurgerButton() {
  const { toggle } = useDrawer();
  return (
    <Pressable onPress={toggle} style={{ marginLeft: 0, padding: 4 }} accessibilityRole="button" accessibilityLabel="Open menu">
      <EveyMark height={26} />
    </Pressable>
  );
}

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
    headerLeft: () => <BurgerButton />,
    headerTitle: () => <HeaderSearch />,
    headerRight: () => <HeaderRight theme={theme} />,
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
      <Stack.Screen name="requests" options={sharedOptions} />
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
  badgeText: { fontSize: 9, fontWeight: "700" },
});
