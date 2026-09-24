import { Text, Modal, Pressable, View, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
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

  const activeMinutesIndex = SLEEP_TIMER_PRESETS_MIN.findIndex(
    (m) => isSleepPresetActive(state, m),
  );
  const activeEndOfEpisode = state?.kind === "endOfEpisode";
  // First render: focus the active preset if there is one, otherwise the first row.
  const preferredIndex = activeMinutesIndex >= 0 ? activeMinutesIndex : activeEndOfEpisode ? SLEEP_TIMER_PRESETS_MIN.length : 0;

  function row(index: number, label: string, active: boolean, onPress: () => void) {
    return (
      <FocusableCard
        key={label}
        hasTVPreferredFocus={index === preferredIndex}
        onPress={() => { onPress(); onClose(); }}
        accessibilityLabel={active ? `${label}, ${t("common.selected", "selected")}` : label}
        style={[styles.row, { borderBottomColor: theme.colors.border }]}
      >
        <Text style={[styles.label, { color: active ? theme.colors.accent : theme.colors.text }]}>{label}</Text>
        {active && <Text style={[styles.check, { color: theme.colors.accent }]}>{"✓"}</Text>}
      </FocusableCard>
    );
  }

  // A hidden RN Modal keeps its children mounted, so this picker re-rendered
  // on every ~2/s currentTime tick of the player while closed. Render nothing
  // until opened.
  if (!visible) return null;

  return (
    <Modal visible={visible} animationType="fade" transparent onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose} focusable={false}>
        <Pressable
          style={[styles.sheet, { backgroundColor: theme.colors.surface }]}
          onPress={(e) => e.stopPropagation()}
          focusable={false}
        >
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("player.settings.sleepTimer", "Sleep timer")}
          </Text>
          <View style={styles.list}>
            {SLEEP_TIMER_PRESETS_MIN.map((m, i) =>
              row(
                i,
                t("player.sleepTimer.minutes", { count: m, defaultValue: "{{count}} min" }),
                activeMinutesIndex === i,
                () => onPickMinutes(m),
              ),
            )}
            {row(
              SLEEP_TIMER_PRESETS_MIN.length,
              t("player.sleepTimer.endOfEpisode", "End of episode"),
              !!activeEndOfEpisode,
              onPickEndOfEpisode,
            )}
            {state !== null &&
              row(SLEEP_TIMER_PRESETS_MIN.length + 1, t("player.sleepTimer.cancel", "Cancel timer"), false, onCancel)}
          </View>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    backgroundColor: "rgba(0,0,0,0.7)",
  },
  sheet: {
    borderRadius: 16,
    padding: 24,
    minWidth: 340,
    maxWidth: 440,
  },
  title: {
    fontSize: FONT_SIZE.xl,
    fontWeight: "600",
    textAlign: "center",
    marginBottom: 16,
  },
  list: {
    gap: 4,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 9,
    paddingHorizontal: 16,
    borderRadius: 8,
  },
  label: {
    fontSize: FONT_SIZE.base,
    flex: 1,
  },
  check: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "600",
    marginLeft: 12,
  },
});
