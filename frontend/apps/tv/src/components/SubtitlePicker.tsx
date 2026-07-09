import { View, Text, Modal, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";

export interface SubtitleStream {
  index: number;
  trackIndex: number;
  language?: string;
  title?: string;
  isDefault?: boolean;
  isForced?: boolean;
  codec?: string;
  burnIn?: boolean;
}

interface SubtitlePickerProps {
  visible: boolean;
  onClose: () => void;
  streams: SubtitleStream[];
  currentIndex: number | null;
  onSelect: (index: number | null) => void;
}

function formatLabel(s: SubtitleStream): string {
  const lang = s.language ?? "Unknown";
  const detail = s.title ?? "";
  return detail ? `${lang} - ${detail}` : lang;
}

const IMAGE_SUBTITLE_CODECS = new Set(["pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub"]);

export function isImageSubtitle(codec?: string): boolean {
  return IMAGE_SUBTITLE_CODECS.has(codec?.toLowerCase() ?? "");
}

export function SubtitlePicker({ visible, onClose, streams, currentIndex, onSelect }: SubtitlePickerProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const offLabel = t("player.settings.off");

  return (
    <Modal visible={visible} animationType="fade" transparent onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable style={[styles.sheet, { backgroundColor: theme.colors.surface }]} onPress={(e) => e.stopPropagation()}>
          <Text style={[styles.title, { color: theme.colors.text }]}>{t("player.settings.subtitles")}</Text>
          <View style={styles.list}>
            <FocusableCard
              hasTVPreferredFocus={currentIndex === null}
              onPress={() => {
                onSelect(null);
                onClose();
              }}
              accessibilityLabel={currentIndex === null ? `${offLabel}, ${t("common.selected", "selected")}` : offLabel}
              style={[styles.row, { borderBottomColor: theme.colors.border }]}
            >
              <Text style={[styles.label, { color: currentIndex === null ? theme.colors.accent : theme.colors.text }]}>
                {offLabel}
              </Text>
              {currentIndex === null && <Text style={[styles.check, { color: theme.colors.accent }]}>{"✓"}</Text>}
            </FocusableCard>
            {streams.map((s) => {
              const active = s.index === currentIndex;
              const label = formatLabel(s);
              const badges = [
                s.isForced ? "Forced" : null,
                s.isDefault ? "Default" : null,
                s.burnIn ? t("media.burnIn") : null,
              ].filter(Boolean).join(" · ");
              const fullLabel = badges ? `${label} (${badges})` : label;
              return (
                <FocusableCard
                  key={s.index}
                  onPress={() => {
                    onSelect(s.index);
                    onClose();
                  }}
                  accessibilityLabel={active ? `${fullLabel}, ${t("common.selected", "selected")}` : fullLabel}
                  style={[styles.row, { borderBottomColor: theme.colors.border }]}
                >
                  <View style={{ flex: 1 }}>
                    <Text style={[styles.label, { color: active ? theme.colors.accent : theme.colors.text }]}>{label}</Text>
                    {badges ? (
                      <Text style={[styles.badges, { color: theme.colors.textMuted }]}>{badges}</Text>
                    ) : null}
                  </View>
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
    fontSize: 20,
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
    fontSize: 16,
  },
  badges: {
    fontSize: 12,
    marginTop: 2,
  },
  check: {
    fontSize: 18,
    fontWeight: "600",
    marginLeft: 12,
  },
});
