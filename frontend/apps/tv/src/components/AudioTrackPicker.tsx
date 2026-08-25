import { View, Text, Modal, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import { FONT_SIZE } from "@evey/theme";
import { formatChannelLayout } from "@evey/api";

export interface AudioStream {
  index: number;
  trackIndex: number;
  language?: string;
  title?: string;
  isDefault?: boolean;
  channels?: number;
}

interface AudioTrackPickerProps {
  visible: boolean;
  onClose: () => void;
  streams: AudioStream[];
  currentIndex: number;
  onSelect: (index: number) => void;
}

function formatLabel(s: AudioStream): string {
  const lang = s.language ?? "Unknown";
  const detail = s.title ?? formatChannelLayout(s.channels);
  return detail ? `${lang} - ${detail}` : lang;
}

export function AudioTrackPicker({ visible, onClose, streams, currentIndex, onSelect }: AudioTrackPickerProps) {
  const { t } = useTranslation();
  const theme = useTheme();

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
          <Text style={[styles.title, { color: theme.colors.text }]}>{t("player.settings.audio")}</Text>
          <View style={styles.list}>
            {streams.map((s) => {
              const active = s.index === currentIndex;
              const label = formatLabel(s);
              return (
                <FocusableCard
                  key={s.index}
                  hasTVPreferredFocus={active}
                  onPress={() => {
                    onSelect(s.index);
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
    minWidth: 400,
    maxWidth: 560,
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
    paddingVertical: 12,
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
