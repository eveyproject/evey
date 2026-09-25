import { View, Text, Pressable, Linking, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { UPDATE_COMMAND, changesUrl, latestLabel, useUpdateStatus } from "@evey/client";
import { useTheme } from "../../providers/ThemeProvider";

// The web dashboard's update banner: shown only when the registry publishes a
// newer image. Evey never updates itself; the command is selectable text, a
// long press copies it.
export function UpdateBanner() {
  const theme = useTheme();
  const { t } = useTranslation();
  const { data } = useUpdateStatus();
  if (!data || data.state !== "update_available") return null;
  const link = changesUrl(data);

  return (
    <View
      style={[styles.card, { backgroundColor: theme.colors.surface, borderColor: theme.colors.accent, borderRadius: theme.radii.md }]}
      testID="update-banner"
    >
      <View style={styles.titleRow}>
        <Ionicons name="arrow-up-circle-outline" size={18} color={theme.colors.accent} />
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600", flexShrink: 1 }}>
          {t("admin.update.available", { latest: latestLabel(data) })}
        </Text>
      </View>
      <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("admin.update.current", { current: data.currentVersion })}</Text>
      <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 4 }}>{t("admin.update.howTo")}</Text>
      <Text selectable style={[styles.command, { color: theme.colors.text, backgroundColor: theme.colors.bg }]}>
        {UPDATE_COMMAND}
      </Text>
      {link ? (
        <Pressable onPress={() => void Linking.openURL(link)} accessibilityRole="link" hitSlop={8}>
          <Text style={{ color: theme.colors.accent, fontSize: 12, marginTop: 4 }}>{t("admin.update.changes")}</Text>
        </Pressable>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { marginHorizontal: 16, marginTop: 16, padding: 14, borderWidth: 1, gap: 2 },
  titleRow: { flexDirection: "row", alignItems: "center", gap: 6 },
  command: { fontFamily: "monospace", fontSize: 12, paddingHorizontal: 8, paddingVertical: 6, borderRadius: 4, marginTop: 4 },
});
