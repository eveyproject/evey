import { Text, Modal, Pressable, View, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import { FONT_SIZE } from "@evey/theme";

// A TV remote has no continuous slider, so volume is a discrete preset list -
// same paradigm as QualityPicker - rather than the web's draggable range input.
const VOLUME_PRESETS = [0, 25, 50, 75, 100] as const;

interface VolumePickerProps {
  visible: boolean;
  onClose: () => void;
  volumePct: number;
  muted: boolean;
  onSelect: (volumePct: number) => void;
  onToggleMute: () => void;
}

export function VolumePicker({ visible, onClose, volumePct, muted, onSelect, onToggleMute }: VolumePickerProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const muteLabel = muted
    ? t("player.controls.unmute", "Unmute")
    : t("player.controls.mute", "Mute");

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
            {t("player.controls.volume", "Volume")}
          </Text>
          <View style={styles.list}>
            <FocusableCard
              hasTVPreferredFocus={true}
              onPress={onToggleMute}
              accessibilityLabel={muteLabel}
              style={[styles.row, { borderBottomColor: theme.colors.border }]}
            >
              <Text style={[styles.label, { color: muted ? theme.colors.accent : theme.colors.text }]}>{muteLabel}</Text>
            </FocusableCard>
            {VOLUME_PRESETS.map((v) => {
              const active = !muted && v === volumePct;
              const label = `${v}%`;
              return (
                <FocusableCard
                  key={v}
                  onPress={() => {
                    onSelect(v);
                    onClose();
                  }}
                  accessibilityLabel={active ? `${label}, ${t("common.selected", "selected")}` : label}
                  style={[styles.row, { borderBottomColor: theme.colors.border }]}
                >
                  <Text style={[styles.label, { color: active ? theme.colors.accent : theme.colors.text }]}>{label}</Text>
                  {active && <Text style={[styles.check, { color: theme.colors.accent }]}>{"✓"}</Text>}
                </FocusableCard>
              );
            })}
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
    minWidth: 300,
    maxWidth: 400,
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
