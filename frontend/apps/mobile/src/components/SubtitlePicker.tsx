import { View, Text, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { BottomSheet } from "./BottomSheet";

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
  return detail ? `${lang} \u2014 ${detail}` : lang;
}

export function SubtitlePicker({
  visible,
  onClose,
  streams,
  currentIndex,
  onSelect,
}: SubtitlePickerProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  return (
    <BottomSheet visible={visible} onClose={onClose} title={t("player.settings.subtitles")}>
      <Pressable
        style={[styles.row, { borderBottomColor: theme.colors.divider }]}
        onPress={() => {
          onSelect(null);
          onClose();
        }}
        accessible={true}
        accessibilityLabel={t("player.settings.off")}
        accessibilityRole="button"
      >
        <Text style={[styles.label, { color: theme.colors.text }, currentIndex === null && { color: theme.colors.accent }]}>
          {t("player.settings.off")}
        </Text>
        {currentIndex === null && <Text style={[styles.check, { color: theme.colors.accent }]}>{"\u2713"}</Text>}
      </Pressable>
      {streams.map((s) => {
        const active = s.index === currentIndex;
        return (
          <Pressable
            key={s.index}
            style={[styles.row, { borderBottomColor: theme.colors.divider }]}
            onPress={() => {
              onSelect(s.index);
              onClose();
            }}
            accessible={true}
            accessibilityLabel={formatLabel(s)}
            accessibilityRole="button"
          >
            <View style={styles.labelRow}>
              <Text style={[styles.label, { color: theme.colors.text }, active && { color: theme.colors.accent }]}>
                {formatLabel(s)}
              </Text>
              <View style={styles.badges}>
                {s.isForced && (
                  <View style={[styles.badge, { backgroundColor: theme.colors.divider }]}>
                    <Text style={[styles.badgeText, { color: theme.colors.textMuted }]}>Forced</Text>
                  </View>
                )}
                {s.isDefault && (
                  <View style={[styles.badge, { backgroundColor: theme.colors.divider }]}>
                    <Text style={[styles.badgeText, { color: theme.colors.textMuted }]}>Default</Text>
                  </View>
                )}
                {s.burnIn && (
                  <View style={[styles.badgeBurnIn, { backgroundColor: theme.colors.warning + "33" }]}>
                    <Text style={[styles.badgeText, { color: theme.colors.textMuted }]}>{t("media.burnIn")}</Text>
                  </View>
                )}
              </View>
            </View>
            {active && <Text style={[styles.check, { color: theme.colors.accent }]}>{"\u2713"}</Text>}
          </Pressable>
        );
      })}
    </BottomSheet>
  );
}

const IMAGE_SUBTITLE_CODECS = new Set([
  "pgssub", "hdmv_pgs_subtitle", "dvd_subtitle", "dvbsub", "xsub",
]);

export function isImageSubtitle(codec?: string): boolean {
  return IMAGE_SUBTITLE_CODECS.has(codec?.toLowerCase() ?? "");
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  labelRow: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
  },
  label: {
    fontSize: 15,
  },
  badges: {
    flexDirection: "row",
    gap: 4,
  },
  badge: {
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  badgeBurnIn: {
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  badgeText: {
    fontSize: 11,
    fontWeight: "500",
  },
  check: {
    fontSize: 18,
    fontWeight: "600",
    marginLeft: 12,
  },
});
