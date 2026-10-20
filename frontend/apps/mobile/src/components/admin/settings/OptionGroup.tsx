import { useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../../providers/ThemeProvider";

export interface OptionGroupOption {
  value: string;
  label: string;
  disabled?: boolean;
}

interface OptionGroupProps {
  /** Accessible name of the group (the setting it picks). */
  label: string;
  options: OptionGroupOption[];
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  /** Label of a stored value no option offers; it is shown, never hidden. */
  formatUnknown?: (value: string) => string;
  testID?: string;
}

const GAP = 8;
const CELL_PADDING = 10;
// An average glyph of the UI font at FONT_SIZE.xs is a little over half its
// size; the estimate errs wide, and the measured line count corrects it.
const GLYPH = FONT_SIZE.xs * 0.62;

export type OptionGroupLayout = "row" | "column";

// optionGroupLayout decides how a group sits in `width` pixels: every option
// in one row of equal cells when the longest label fits a cell on one line,
// otherwise one option per line at full width. Never a ragged 2+1 grid.
// `wrappedAt` is the width at which a label was seen wrapping in the row.
export function optionGroupLayout(width: number, labels: string[], wrappedAt: number | null = null): OptionGroupLayout {
  if (labels.length <= 1 || width <= 0) return "row";
  if (wrappedAt !== null && wrappedAt === width) return "column";
  const cell = (width - GAP * (labels.length - 1)) / labels.length;
  const longest = Math.max(...labels.map((l) => l.length));
  return longest * GLYPH + CELL_PADDING * 2 <= cell ? "row" : "column";
}

// OptionGroup is the radio group of the settings screens (the web's
// SegmentedControl): exactly one option picked, all options the same width.
// It starts from an estimate and switches to one option per line as soon as
// a label is seen wrapping, so a long translation never squeezes a cell.
export function OptionGroup({ label, options: given, value, onChange, disabled = false, formatUnknown, testID }: OptionGroupProps) {
  const theme = useTheme();
  const [width, setWidth] = useState(0);
  const [wrappedAt, setWrappedAt] = useState<number | null>(null);
  const options = given.some((o) => o.value === value)
    ? given
    : [...given, { value, label: formatUnknown ? formatUnknown(value) : value }];
  const layout = optionGroupLayout(width, options.map((o) => o.label), wrappedAt);
  const row = layout === "row";

  return (
    <View
      accessibilityRole="radiogroup"
      accessibilityLabel={label}
      onLayout={(e) => setWidth(Math.round(e.nativeEvent.layout.width))}
      style={row ? styles.row : styles.column}
      testID={testID}
    >
      {options.map((o) => {
        const checked = o.value === value;
        const off = disabled || !!o.disabled;
        return (
          <Pressable
            key={o.value || "__default__"}
            onPress={() => {
              if (!off && !checked) onChange(o.value);
            }}
            disabled={off}
            accessible={true}
            accessibilityRole="radio"
            accessibilityLabel={o.label}
            accessibilityState={{ checked, disabled: off }}
            testID={testID ? `${testID}-option-${o.value || "default"}` : undefined}
            style={[
              styles.cell,
              row ? styles.rowCell : styles.columnCell,
              {
                borderRadius: theme.radii.md,
                borderColor: checked ? theme.colors.accent : theme.colors.border,
                backgroundColor: checked ? theme.colors.accent : theme.colors.surfaceRaised,
                opacity: off ? 0.5 : 1,
              },
            ]}
          >
            <Text
              style={[styles.text, { color: checked ? theme.colors.onAccent : theme.colors.text, textAlign: row ? "center" : "left" }]}
              onTextLayout={(e) => {
                if (row && width > 0 && e.nativeEvent.lines.length > 1) setWrappedAt(width);
              }}
            >
              {o.label}
            </Text>
            {!row && checked ? <Ionicons name="checkmark" size={16} color={theme.colors.onAccent} /> : null}
          </Pressable>
        );
      })}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { flexDirection: "row", gap: GAP },
  column: { flexDirection: "column", gap: GAP },
  cell: { borderWidth: 1, paddingVertical: 10, paddingHorizontal: CELL_PADDING, minHeight: 44, alignItems: "center" },
  // flexBasis 0 + flexGrow 1: every cell gets the same share of the row,
  // whatever its label.
  rowCell: { flexGrow: 1, flexShrink: 1, flexBasis: 0, justifyContent: "center" },
  columnCell: { alignSelf: "stretch", flexDirection: "row", justifyContent: "space-between" },
  text: { fontSize: FONT_SIZE.xs, fontWeight: "600", flexShrink: 1 },
});
