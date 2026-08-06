import { View, Text, Pressable, StyleSheet } from "react-native";
import { useTheme } from "../../../providers/ThemeProvider";

export interface ChipOption {
  value: string;
  label: string;
}

interface ChipSelectProps {
  options: ChipOption[];
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}

// Small pill-select for short enums, reused by the server-config admin cards
// (memory profile, mkvgo modes, SMTP TLS mode) instead of duplicating the
// chip-row markup that already exists inline for the log-level picker.
export function ChipSelect({ options, value, onChange, disabled }: ChipSelectProps) {
  const theme = useTheme();
  return (
    <View style={styles.row}>
      {options.map((opt) => {
        const active = opt.value === value;
        return (
          <Pressable
            key={opt.value || "__empty__"}
            onPress={() => onChange(opt.value)}
            disabled={disabled}
            accessible={true}
            accessibilityLabel={opt.label}
            accessibilityRole="radio"
            accessibilityState={{ selected: active, disabled }}
            style={[
              styles.chip,
              {
                borderColor: theme.colors.border,
                backgroundColor: active ? theme.colors.accent : theme.colors.surface,
                opacity: disabled ? 0.5 : 1,
              },
            ]}
          >
            <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 13 }}>
              {opt.label}
            </Text>
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", flexWrap: "wrap", gap: 8 },
  chip: { paddingHorizontal: 12, paddingVertical: 8, borderRadius: 8, borderWidth: StyleSheet.hairlineWidth },
});
