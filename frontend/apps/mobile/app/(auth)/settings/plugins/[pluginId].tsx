import { ScrollView, Text, StyleSheet } from "react-native";
import { Stack, useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../../src/providers/ThemeProvider";
import { HeaderBack } from "../../../../src/components/layout/HeaderBack";
import { PluginSlot, usePluginSlotEntries } from "../../../../src/features/plugins/PluginSlot";

// One screen per plugin that ships per-profile settings, reached from the
// Plugins section of the settings tab. Mirrors the admin extensions screen:
// the settings tab used to stack every plugin's WebView inline, with nothing
// naming which plugin owned which block.
export default function PluginUserSettingsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { pluginId } = useLocalSearchParams<{ pluginId: string }>();
  const id = String(pluginId ?? "");
  const entry = usePluginSlotEntries("settings.user.appearance").find((e) => e.pluginId === id);

  return (
    <ScrollView
      style={{ flex: 1, backgroundColor: theme.colors.bg }}
      contentContainerStyle={{ paddingBottom: 40 }}
    >
      <Stack.Screen
        options={{
          headerShown: true,
          title: entry?.name ?? t("auth.settings.tabs.plugins"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
          headerLeft: () => <HeaderBack />,
        }}
      />
      {id ? (
        <PluginSlot name="settings.user.appearance" pluginId={id} />
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
