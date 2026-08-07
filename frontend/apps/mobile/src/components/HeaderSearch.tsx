import { Text, Pressable, StyleSheet } from "react-native";
import { router } from "expo-router";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

// The header title lives inside the NATIVE toolbar, and both Android and iOS
// clip a toolbar's children to the bar's bounds - so the old in-header result
// dropdown (absolutely positioned below the bar) could never render on device:
// typing looked dead until the user submitted and landed on the search screen.
// The header field is therefore a TRIGGER, not an input: pressing it opens the
// search screen (which searches live as you type) and focuses its field.
export function HeaderSearch() {
  const theme = useTheme();
  const { t } = useTranslation();

  const openSearch = () => {
    router.navigate({
      pathname: "/(auth)/(tabs)/search",
      // Fresh value each press so the search screen re-focuses its input even
      // when it is already the visible screen.
      params: { focus: String(Date.now()) },
    });
  };

  return (
    <Pressable
      onPress={openSearch}
      style={[styles.inputRow, { backgroundColor: theme.colors.surfaceRaised, borderColor: theme.colors.border }]}
      accessible
      accessibilityRole="button"
      accessibilityLabel={t("media.search.placeholder", "Search movies, series…")}
    >
      <Ionicons name="search-outline" size={ICON_SIZE.md} color={theme.colors.textMuted} style={{ marginRight: 6 }} />
      <Text style={[styles.placeholder, { color: theme.colors.textMuted }]} numberOfLines={1}>
        {t("media.search.placeholder", "Search movies, series…")}
      </Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  inputRow: {
    flexDirection: "row",
    alignItems: "center",
    borderRadius: 8,
    borderWidth: 1,
    paddingHorizontal: 10,
    paddingVertical: 8,
  },
  placeholder: {
    flex: 1,
    fontSize: FONT_SIZE.sm,
  },
});
