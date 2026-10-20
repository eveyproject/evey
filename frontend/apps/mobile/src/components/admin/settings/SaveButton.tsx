import { ActivityIndicator, Pressable, StyleSheet, Text } from "react-native";
import { useTranslation } from "react-i18next";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../../providers/ThemeProvider";

interface SaveButtonProps {
  onPress: () => void;
  disabled?: boolean;
  loading?: boolean;
  label?: string;
  testID?: string;
}

// SaveButton is the one Save of a settings group: full width, greyed while
// nothing changed, a spinner while the write is in flight.
export function SaveButton({ onPress, disabled = false, loading = false, label, testID }: SaveButtonProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const text = label ?? t("common.save");
  const off = disabled || loading;
  return (
    <Pressable
      onPress={onPress}
      disabled={off}
      accessibilityRole="button"
      accessibilityLabel={text}
      accessibilityState={{ disabled: off, busy: loading }}
      testID={testID}
      style={[styles.btn, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.md, opacity: off ? 0.5 : 1 }]}
    >
      {loading ? (
        <ActivityIndicator size="small" color={theme.colors.onAccent} />
      ) : (
        <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: FONT_SIZE.sm }}>{text}</Text>
      )}
    </Pressable>
  );
}

const styles = StyleSheet.create({
  btn: { paddingVertical: 12, minHeight: 44, alignItems: "center", justifyContent: "center" },
});
