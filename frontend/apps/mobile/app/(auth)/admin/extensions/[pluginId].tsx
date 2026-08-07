import { ScrollView, Text, StyleSheet } from "react-native";
import { Stack, useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../../src/providers/ThemeProvider";
import { HeaderBack } from "../../../../src/components/layout/HeaderBack";
import { PluginSlot, usePluginSlotEntries } from "../../../../src/features/plugins/PluginSlot";

// Dedicated full-screen route for a plugin's admin.nav module - mobile parity
// with web's /admin/extensions/<id>. The dashboard menu row navigates here
// instead of embedding the WebView inline, so each plugin gets a real screen.
export default function PluginExtensionScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { pluginId } = useLocalSearchParams<{ pluginId: string }>();
  const id = String(pluginId ?? "");
  const entry = usePluginSlotEntries("admin.nav").find((e) => e.pluginId === id);

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
    >
      <Stack.Screen
        options={{
          headerShown: true,
          title: entry?.name ?? t("admin.nav.plugins", "Plugin"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
          headerLeft: () => <HeaderBack />,
        }}
      />
      {id ? (
        <PluginSlot name="admin.nav" pluginId={id} />
      ) : (
        <Text style={[styles.empty, { color: theme.colors.textMuted }]}>
          {t("admin.plugins.notFound", "This plugin is not installed, enabled, or has no admin page.")}
        </Text>
      )}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  empty: { padding: 24, fontSize: 14, textAlign: "center" },
});
