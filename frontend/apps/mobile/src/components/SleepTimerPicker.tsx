import { Text, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { BottomSheet } from "./BottomSheet";
import { useTheme } from "../providers/ThemeProvider";
import type { SleepTimerState } from "../hooks/useSleepTimer";
import { SLEEP_TIMER_PRESETS_MIN, isSleepPresetActive } from "@evey/client";
import { FONT_SIZE } from "@evey/theme";


interface SleepTimerPickerProps {
  visible: boolean;
  onClose: () => void;
  state: SleepTimerState | null;
  onPickMinutes: (minutes: number) => void;
  onPickEndOfEpisode: () => void;
  onCancel: () => void;
}

export function SleepTimerPicker({
  visible,
  onClose,
  state,
  onPickMinutes,
  onPickEndOfEpisode,
  onCancel,
}: SleepTimerPickerProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  const renderRow = (label: string, active: boolean, onPress: () => void) => (
    <Pressable
      key={label}
      style={[styles.row, { borderBottomColor: theme.colors.border }]}
      onPress={() => { onPress(); onClose(); }}
      accessible={true}
      accessibilityLabel={label}
      accessibilityRole="button"
    >
      <Text style={[styles.label, { color: theme.colors.text }, active && { color: theme.colors.accent }]}>
        {label}
      </Text>
      {active && <Text style={[styles.check, { color: theme.colors.accent }]}>{"✓"}</Text>}
    </Pressable>
  );

  return (
    <BottomSheet visible={visible} onClose={onClose} title={t("player.settings.sleepTimer", "Sleep timer")}>
      {SLEEP_TIMER_PRESETS_MIN.map((m) =>
        renderRow(
          t("player.sleepTimer.minutes", { count: m, defaultValue: "{{count}} min" }),
          isSleepPresetActive(state, m),
          () => onPickMinutes(m),
        ),
      )}
      {renderRow(
        t("player.sleepTimer.endOfEpisode", "End of episode"),
        state?.kind === "endOfEpisode",
        onPickEndOfEpisode,
      )}
      {state !== null && renderRow(
        t("player.sleepTimer.cancel", "Cancel timer"),
        false,
        onCancel,
      )}
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  label: {
    fontSize: FONT_SIZE.sm,
    flex: 1,
  },
  check: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "600",
    marginLeft: 12,
  },
});
