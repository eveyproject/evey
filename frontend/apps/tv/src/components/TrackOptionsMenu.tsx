import { View, Text, Modal, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";

interface TrackOptionsMenuProps {
  visible: boolean;
  onClose: () => void;
  hasAudioOptions: boolean;
  hasSubtitleOptions: boolean;
  onOpenAudio: () => void;
  onOpenSubtitles: () => void;
}

/**
 * Entry point opened via a long-press of the D-pad select button. The main
 * player screen already claims every plain D-pad direction (seek/speed/
 * quality), so track selection lives one level down behind this menu instead
 * of competing for a free direction.
 */
export function TrackOptionsMenu({
  visible,
  onClose,
  hasAudioOptions,
  hasSubtitleOptions,
  onOpenAudio,
  onOpenSubtitles,
}: TrackOptionsMenuProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  return (
    <Modal visible={visible} animationType="fade" transparent onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable style={[styles.sheet, { backgroundColor: theme.colors.surface }]} onPress={(e) => e.stopPropagation()}>
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("player.settings.trackOptions", "Audio & Subtitles")}
          </Text>
          <View style={styles.list}>
            {hasAudioOptions && (
              <FocusableCard
                hasTVPreferredFocus={true}
                onPress={onOpenAudio}
                accessibilityLabel={t("player.settings.audio")}
                style={[styles.row, { borderBottomColor: theme.colors.border }]}
              >
                <Text style={[styles.label, { color: theme.colors.text }]}>{t("player.settings.audio")}</Text>
              </FocusableCard>
            )}
            {hasSubtitleOptions && (
              <FocusableCard
                hasTVPreferredFocus={!hasAudioOptions}
                onPress={onOpenSubtitles}
                accessibilityLabel={t("player.settings.subtitles")}
                style={[styles.row, { borderBottomColor: theme.colors.border }]}
              >
                <Text style={[styles.label, { color: theme.colors.text }]}>{t("player.settings.subtitles")}</Text>
              </FocusableCard>
            )}
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
    minWidth: 360,
    maxWidth: 480,
  },
  title: {
    fontSize: 20,
    fontWeight: "600",
    textAlign: "center",
    marginBottom: 16,
  },
  list: {
    gap: 4,
  },
  row: {
    paddingVertical: 12,
    paddingHorizontal: 16,
    borderRadius: 8,
  },
  label: {
    fontSize: 16,
  },
});
