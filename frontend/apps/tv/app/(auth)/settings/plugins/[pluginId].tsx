import { View, Text, ScrollView, StyleSheet } from "react-native";
import { Stack, router, useLocalSearchParams } from "expo-router";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../../src/providers/ThemeProvider";
import { FocusableCard } from "../../../../src/components/FocusableCard";
import { PluginSlot, usePluginSlotEntries } from "../../../../src/features/plugins/PluginSlot";

// One screen per plugin that ships per-profile settings, reached from the
// Plugins section of the settings tab. Mirrors mobile's per-plugin settings
// screen: the settings tab used to stack every plugin's WebView inline, with
// nothing naming which plugin owned which block.
export default function PluginUserSettingsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { pluginId } = useLocalSearchParams<{ pluginId: string }>();
  const id = String(pluginId ?? "");
  const entry = usePluginSlotEntries("settings.user.appearance").find((e) => e.pluginId === id);

  const header = (
    <Stack.Screen options={{ title: entry?.name ?? t("auth.settings.tabs.plugins") }} />
  );

  if (!id) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        {header}
        <Text style={[styles.empty, { color: theme.colors.textMuted }]}>
          {t("admin.plugins.notFound", "This plugin is not installed, enabled, or has no admin page.")}
        </Text>
        <FocusableCard
          onPress={() => router.back()}
          hasTVPreferredFocus
          style={[styles.backBtn, { backgroundColor: theme.colors.surface }]}
          accessibilityLabel={t("common.back", "Back")}
        >
          <Text style={{ color: theme.colors.text, fontSize: 18 }}>{t("common.back", "Back")}</Text>
        </FocusableCard>
      </View>
    );
  }

  return (
    <ScrollView
      style={[styles.container, { backgroundColor: theme.colors.bg }]}
      contentContainerStyle={{ padding: 32 }}
    >
      {header}
      <Text style={[styles.title, { color: theme.colors.text }]}>{entry?.name ?? t("auth.settings.tabs.plugins")}</Text>
      <PluginSlot name="settings.user.appearance" pluginId={id} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  title: { fontSize: 30, fontWeight: "700", marginBottom: 24 },
  center: { flex: 1, justifyContent: "center", alignItems: "center", padding: 48 },
  empty: { fontSize: 18, textAlign: "center", marginBottom: 24 },
  backBtn: { paddingVertical: 9, paddingHorizontal: 24 },
});
