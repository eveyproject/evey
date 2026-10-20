import { Children, Fragment, isValidElement, type ReactNode } from "react";
import { StyleSheet, Text, View } from "react-native";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../../providers/ThemeProvider";
import { InfoButton } from "./InfoButton";

interface SettingsGroupProps {
  title: string;
  /** One short sentence under the title. */
  description?: string;
  /** Longer explanation behind an "i" next to the title. */
  info?: string;
  /** Action row under the settings (typically the group's Save button). */
  footer?: ReactNode;
  children: ReactNode;
  testID?: string;
}

// flatten unwraps fragments so each row gets its own hairline.
function flatten(children: ReactNode): ReactNode[] {
  const out: ReactNode[] = [];
  Children.forEach(children, (c) => {
    if (c === null || c === undefined || c === false) return;
    if (isValidElement<{ children?: ReactNode }>(c) && c.type === Fragment) out.push(...flatten(c.props.children));
    else out.push(c);
  });
  return out;
}

// SettingsGroup is a titled card of setting rows separated by hairlines, with
// an optional footer for its Save. A section of the settings screens is a
// stack of these (the web's components/ui/SettingsGroup).
export function SettingsGroup({ title, description, info, footer, children, testID }: SettingsGroupProps) {
  const theme = useTheme();
  const rows = flatten(children);
  return (
    <View
      style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg, borderColor: theme.colors.border }]}
      testID={testID}
    >
      <View style={styles.header}>
        <View style={styles.titleLine}>
          <Text style={[styles.title, { color: theme.colors.text }]} accessibilityRole="header">
            {title}
          </Text>
          {info ? <InfoButton label={title} info={info} /> : null}
        </View>
        {description ? <Text style={[styles.desc, { color: theme.colors.textMuted }]}>{description}</Text> : null}
      </View>
      <View style={[styles.body, { borderTopColor: theme.colors.border }]}>
        {rows.map((row, i) => (
          <View key={i} style={i > 0 ? [styles.divider, { borderTopColor: theme.colors.border }] : undefined}>
            {row}
          </View>
        ))}
      </View>
      {footer ? <View style={[styles.footer, { borderTopColor: theme.colors.border }]}>{footer}</View> : null}
    </View>
  );
}

const styles = StyleSheet.create({
  card: { borderWidth: StyleSheet.hairlineWidth, marginBottom: 16 },
  header: { paddingHorizontal: 16, paddingTop: 14, paddingBottom: 10 },
  titleLine: { flexDirection: "row", alignItems: "center", gap: 6 },
  title: { fontSize: FONT_SIZE.base, fontWeight: "700", flexShrink: 1 },
  desc: { fontSize: 12, marginTop: 2 },
  body: { borderTopWidth: StyleSheet.hairlineWidth, paddingHorizontal: 16, paddingVertical: 4 },
  divider: { borderTopWidth: StyleSheet.hairlineWidth },
  footer: { borderTopWidth: StyleSheet.hairlineWidth, paddingHorizontal: 16, paddingVertical: 12 },
});
