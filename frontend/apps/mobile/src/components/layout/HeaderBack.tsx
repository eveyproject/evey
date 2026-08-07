import { Pressable } from "react-native";
import { router } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";

// Explicit back chevron for admin DETAIL screens (user edit, media segments,
// plugin extension): the admin navigator's default headerLeft is the drawer
// burger, and these flows must return to their caller instead.
export function HeaderBack() {
  const theme = useTheme();
  const { t } = useTranslation();
  return (
    <Pressable
      onPress={() => router.back()}
      hitSlop={8}
      style={{ padding: 4 }}
      accessibilityRole="button"
      accessibilityLabel={t("common.back", "Back")}
    >
      <Ionicons name="chevron-back" size={24} color={theme.colors.text} />
    </Pressable>
  );
}
