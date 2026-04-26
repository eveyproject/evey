import { View, Text, Pressable, StyleSheet } from "react-native";
import { Stack, router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useNotificationServiceGetUnreadCount } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useDrawer } from "../../../src/providers/DrawerProvider";
import { HeaderSearch } from "../../../src/components/HeaderSearch";
import { UserMenu } from "../../../src/components/layout/UserMenu";

function BurgerButton({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { toggle } = useDrawer();
  return (
    <Pressable onPress={toggle} style={{ marginLeft: 16, padding: 4 }} accessibilityRole="button" accessibilityLabel="Open menu">
      <Ionicons name="menu" size={24} color={theme.colors.text} />
    </Pressable>
  );
}

function HeaderRight({ theme }: { theme: ReturnType<typeof useTheme> }) {
  const { data: notifData } = useNotificationServiceGetUnreadCount();
  const count = notifData?.count ?? 0;

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
      <UserMenu />
    </View>
  );
}

export default function TabsLayout() {
  const theme = useTheme();

  const sharedOptions = {
    headerStyle: { backgroundColor: theme.colors.surface },
    headerShadowVisible: false,
    headerLeft: () => <BurgerButton theme={theme} />,
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
});
