import { View, Text, Pressable } from "react-native";
import { Tabs, router } from "expo-router";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useNotificationServiceGetUnreadCount } from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";

function NotificationBellButton() {
  const theme = useTheme();
  const { data } = useNotificationServiceGetUnreadCount();
  const count = data?.count ?? 0;

  return (
    <Pressable
      onPress={() => router.push("/notifications")}
      style={{ marginRight: theme.spacing.md }}
      accessible
      accessibilityLabel="Notifications"
      accessibilityRole="button"
    >
      <Ionicons name="notifications-outline" size={22} color={theme.colors.text} />
      {count > 0 && (
        <View style={{
          position: "absolute",
          top: -4,
          right: -6,
          minWidth: 16,
          height: 16,
          borderRadius: 8,
          backgroundColor: theme.colors.accent,
          alignItems: "center",
          justifyContent: "center",
          paddingHorizontal: 3,
        }}>
          <Text style={{ color: "#fff", fontSize: 10, fontWeight: "700" }}>
            {count > 9 ? "9+" : count}
          </Text>
        </View>
      )}
    </Pressable>
  );
}

export default function TabsLayout() {
  const theme = useTheme();
  const { t } = useTranslation();

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarStyle: {
          backgroundColor: theme.colors.surface,
          borderTopColor: theme.colors.border,
        },
        tabBarActiveTintColor: theme.colors.accent,
        tabBarInactiveTintColor: theme.colors.textMuted,
      }}
    >
      <Tabs.Screen
        name="index"
        options={{
          title: t("common.nav.home"),
          headerShown: true,
          headerStyle: { backgroundColor: theme.colors.bg },
          headerTitleStyle: { color: theme.colors.text },
          headerRight: () => <NotificationBellButton />,
          tabBarIcon: ({ color, size }) => <Ionicons name="home" size={size} color={color} />,
        }}
      />
      <Tabs.Screen name="search" options={{ title: t("common.nav.search"), tabBarIcon: ({ color, size }) => <Ionicons name="search" size={size} color={color} /> }} />
      <Tabs.Screen name="library" options={{ title: t("media.library.title"), tabBarIcon: ({ color, size }) => <Ionicons name="library" size={size} color={color} /> }} />
      <Tabs.Screen name="favorites" options={{ title: t("media.favorites.title"), tabBarIcon: ({ color, size }) => <Ionicons name="heart" size={size} color={color} /> }} />
      <Tabs.Screen name="settings" options={{ title: t("common.settings"), tabBarIcon: ({ color, size }) => <Ionicons name="settings" size={size} color={color} /> }} />
    </Tabs>
  );
}
