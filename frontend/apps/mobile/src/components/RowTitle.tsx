import { Pressable, StyleSheet, Text, View, type StyleProp, type ViewStyle } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { router } from "expo-router";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../providers/ThemeProvider";

// A home row's title opens the page the row previews, like the web home; the
// chevron is what tells a thumb the title can be tapped.
export function RowTitle({ title, href, style }: { title: string; href?: string; style?: StyleProp<ViewStyle> }) {
  const theme = useTheme();
  if (!href) {
    return (
      <View style={style}>
        <Text style={[styles.title, { color: theme.colors.text }]}>{title}</Text>
      </View>
    );
  }
  return (
    <Pressable
      onPress={() => router.push(href as never)}
      hitSlop={8}
      accessible={true}
      accessibilityRole="button"
      accessibilityLabel={title}
      style={[styles.link, style]}
    >
      <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={1}>{title}</Text>
      <Ionicons name="chevron-forward" size={18} color={theme.colors.textMuted} />
    </Pressable>
  );
}

const styles = StyleSheet.create({
  link: { flexDirection: "row", alignItems: "center", gap: 4, flexShrink: 1 },
  title: { fontSize: FONT_SIZE.lg, fontWeight: "600", flexShrink: 1 },
});
