import { Text, Pressable, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { usePathname } from "expo-router";
import { useTheme } from "../providers/ThemeProvider";
import { useAppUpdate } from "../hooks/useAppUpdate";
import { openStorePage } from "../lib/appUpdate";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

// Strip offered when the server was released with a newer phone build than
// this one (see lib/appUpdate.ts): the Play Store would not install it on
// its own, so the strip opens its page. Never over the player, whose chrome
// auto-hides; nothing at all when this build is current.
export function UpdateBanner() {
  const update = useAppUpdate();
  const theme = useTheme();
  const { t } = useTranslation();
  const pathname = usePathname();
  if (!update || pathname?.startsWith("/player")) return null;
  return (
    <Pressable
      onPress={() => void openStorePage()}
      accessibilityRole="button"
      accessibilityLabel={t("mobile.appUpdate.openStore", { version: update.version, defaultValue: `Update Evey to ${update.version} on the Play Store` })}
      style={[styles.banner, { backgroundColor: theme.colors.accent + "33" }]}
      testID="update-banner"
    >
      <Ionicons name="arrow-up-circle" size={ICON_SIZE.sm} color={theme.colors.accent} />
      <Text style={[styles.label, { color: theme.colors.text }]} numberOfLines={1}>
        {t("mobile.appUpdate.available", { version: update.version, defaultValue: `Update available: ${update.version}` })}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  banner: {
    alignSelf: "stretch",
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: 6,
    paddingHorizontal: 16,
    paddingVertical: 8,
  },
  label: {
    fontSize: FONT_SIZE.xs,
    fontWeight: "600",
    textAlign: "center",
  },
});
