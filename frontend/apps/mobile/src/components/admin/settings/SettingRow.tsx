import { type ReactNode } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { FONT_SIZE } from "@evey/theme";
import { useTheme } from "../../../providers/ThemeProvider";
import { InfoButton } from "./InfoButton";

export interface SettingRowReset {
  /** True when the admin chose a value (an override); false while the default applies. */
  overridden: boolean;
  /** The default, as shown in "Using default: X" while no override is set. */
  defaultText?: string;
  onReset: () => void;
  disabled?: boolean;
}

interface SettingRowProps {
  label: string;
  /** Plain-language explanation behind the "i" next to the label. */
  info?: string;
  /** One short line under the label, always visible. */
  hint?: ReactNode;
  /** The server reads the value at boot: shows an "applied on restart" badge. */
  restart?: boolean;
  /** Default / reset line under the control. */
  reset?: SettingRowReset;
  /** A validation message, shown in the danger colour. */
  error?: string;
  /**
   * How the control sits:
   * - "toggle": a switch on the right of the label;
   * - "field" and "wide": the control takes the full width, under the label
   *   (a phone has no room for a label column next to an input).
   */
  kind?: "toggle" | "field" | "wide";
  children: ReactNode;
  testID?: string;
}

// SettingRow is the one layout every admin setting uses on the phone: label,
// its "i" and the restart badge on top, the control on the right (a switch) or
// under the label (anything else), then the hint, the error and the
// default/reset line. The web has the same row (components/ui/SettingRow).
export function SettingRow({ label, info, hint, restart = false, reset, error, kind = "field", children, testID }: SettingRowProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const toggle = kind === "toggle";

  const heading = (
    <View style={styles.heading}>
      <Text style={[styles.label, { color: theme.colors.text }]}>{label}</Text>
      {info ? <InfoButton label={label} info={info} /> : null}
      {restart ? (
        <View style={[styles.badge, { borderColor: theme.colors.warning }]} testID="setting-restart-badge">
          <Text style={{ color: theme.colors.warning, fontSize: 11, fontWeight: "600" }}>{t("admin.serverConfig.appliedOnRestart")}</Text>
        </View>
      ) : null}
    </View>
  );

  const below = (
    <>
      {hint ? <Text style={[styles.small, { color: theme.colors.textMuted }]}>{hint}</Text> : null}
      {error ? (
        <Text style={[styles.small, { color: theme.colors.danger }]} accessibilityRole="alert" accessibilityLiveRegion="polite" testID="setting-error">
          {error}
        </Text>
      ) : null}
      {reset ? (
        reset.overridden ? (
          <Pressable
            onPress={reset.onReset}
            disabled={reset.disabled}
            hitSlop={8}
            accessibilityRole="button"
            accessibilityState={{ disabled: !!reset.disabled }}
            style={[styles.resetBtn, { opacity: reset.disabled ? 0.5 : 1 }]}
            testID="setting-reset"
          >
            <Text style={[styles.small, { color: theme.colors.accent, marginTop: 0 }]}>{t("admin.serverSettings.reset")}</Text>
          </Pressable>
        ) : (
          <Text style={[styles.small, { color: theme.colors.textMuted }]} testID="setting-using-default">
            {t("admin.serverSettings.usingDefault")} {reset.defaultText}
          </Text>
        )
      ) : null}
    </>
  );

  if (toggle) {
    return (
      <View style={styles.row} testID={testID}>
        <View style={styles.toggleLine}>
          <View style={styles.flex}>{heading}</View>
          <View style={styles.toggleControl}>{children}</View>
        </View>
        {below}
      </View>
    );
  }
  return (
    <View style={styles.row} testID={testID}>
      {heading}
      <View style={styles.control}>{children}</View>
      {below}
    </View>
  );
}

const styles = StyleSheet.create({
  row: { paddingVertical: 12 },
  heading: { flexDirection: "row", flexWrap: "wrap", alignItems: "center", columnGap: 6, rowGap: 4 },
  label: { fontSize: FONT_SIZE.sm, fontWeight: "500", flexShrink: 1 },
  badge: { borderWidth: 1, borderRadius: 4, paddingHorizontal: 6, paddingVertical: 1 },
  toggleLine: { flexDirection: "row", alignItems: "center", gap: 12 },
  toggleControl: { flexShrink: 0 },
  flex: { flex: 1, minWidth: 0 },
  control: { marginTop: 8 },
  small: { fontSize: 12, marginTop: 4 },
  resetBtn: { alignSelf: "flex-start", marginTop: 4 },
});
