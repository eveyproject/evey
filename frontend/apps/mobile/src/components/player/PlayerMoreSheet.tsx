import { Text, Pressable, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { BottomSheet } from "../BottomSheet";
import { useTheme } from "../../providers/ThemeProvider";
import type { SleepTimerState } from "../../hooks/useSleepTimer";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

// Secondary sheets these rows deep-link into - kept distinct from "more"/"audio"/
// "subtitle"/"syncPlay" (which the bottom bar and top-right cluster open directly).
export type MoreSheetTarget = "chapters" | "quality" | "sleepTimer" | "streamInfo";

interface PlayerMoreSheetProps {
  visible: boolean;
  onClose: () => void;
  hasChapters: boolean;
  playbackSpeed: number;
  onCycleSpeed: () => void;
  sleepTimerState: SleepTimerState | null;
  pipSupported: boolean;
  onPip: () => void;
  onOpenSheet: (sheet: MoreSheetTarget) => void;
}

// Groups every secondary player control (chapters, quality, speed, sleep timer,
// stream info, PiP) that used to be its own direct button in the bottom bar -
// "50 options, not great" feedback (2026-07-25). Nothing here is new behaviour,
// each row calls the exact same handler the old direct button called.
export function PlayerMoreSheet({
  visible,
  onClose,
  hasChapters,
  playbackSpeed,
  onCycleSpeed,
  sleepTimerState,
  pipSupported,
  onPip,
  onOpenSheet,
}: PlayerMoreSheetProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  function Row({
    icon,
    label,
    trailing,
    iconColor,
    onPress,
  }: {
    icon: keyof typeof Ionicons.glyphMap;
    label: string;
    trailing?: string;
    iconColor?: string;
    onPress: () => void;
  }) {
    return (
      <Pressable
        style={[styles.row, { borderBottomColor: theme.colors.border }]}
        onPress={onPress}
        accessible={true}
        accessibilityLabel={label}
        accessibilityRole="button"
      >
        <Ionicons name={icon} size={ICON_SIZE.md} color={iconColor ?? theme.colors.text} style={styles.rowIcon} />
        <Text style={[styles.label, { color: theme.colors.text }]}>{label}</Text>
        {trailing !== undefined && (
          <Text style={[styles.trailing, { color: theme.colors.textMuted }]}>{trailing}</Text>
        )}
      </Pressable>
    );
  }

  return (
    <BottomSheet visible={visible} onClose={onClose} title={t("player.settings.more", "More options")}>
      {hasChapters && (
        <Row
          icon="book-outline"
          label={t("player.controls.chapters")}
          onPress={() => onOpenSheet("chapters")}
        />
      )}
      <Row
        icon="settings-sharp"
        label={t("player.settings.quality")}
        onPress={() => onOpenSheet("quality")}
      />
      {/* Cycling in place (rather than closing) matches the old direct button's
          tap-to-cycle behaviour exactly - the row's trailing text updates live. */}
      <Row
        icon="speedometer-outline"
        label={t("player.settings.speed")}
        trailing={`${playbackSpeed}x`}
        onPress={onCycleSpeed}
      />
      <Row
        icon={sleepTimerState ? "moon" : "moon-outline"}
        iconColor={sleepTimerState ? theme.colors.accent : undefined}
        label={t("player.settings.sleepTimer", "Sleep timer")}
        trailing={
          sleepTimerState?.kind === "minutes" && sleepTimerState.minutesLeft !== null
            ? String(sleepTimerState.minutesLeft)
            : undefined
        }
        onPress={() => onOpenSheet("sleepTimer")}
      />
      <Row
        icon="information-circle-outline"
        label={t("player.streamInfo.title", "Stream Info")}
        onPress={() => onOpenSheet("streamInfo")}
      />
      {pipSupported && (
        <Row
          icon="albums-outline"
          label={t("player.controls.pip")}
          onPress={() => { onPip(); onClose(); }}
        />
      )}
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  rowIcon: {
    marginRight: 12,
  },
  label: {
    fontSize: FONT_SIZE.sm,
    flex: 1,
  },
  trailing: {
    fontSize: FONT_SIZE.sm,
    fontWeight: "600",
    marginLeft: 12,
  },
});
