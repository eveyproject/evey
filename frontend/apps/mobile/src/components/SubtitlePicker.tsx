import { View, Text, Pressable, StyleSheet } from "react-native";
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
  return (
    <BottomSheet visible={visible} onClose={onClose} title="Subtitles">
      <Pressable
        style={styles.row}
        onPress={() => {
          onSelect(null);
          onClose();
        }}
        accessible={true}
        accessibilityLabel="None"
        accessibilityRole="button"
      >
        <Text style={[styles.label, currentIndex === null && styles.activeLabel]}>
          None
        </Text>
        {currentIndex === null && <Text style={styles.check}>{"\u2713"}</Text>}
      </Pressable>
      {streams.map((s) => {
        const active = s.index === currentIndex;
        return (
          <Pressable
            key={s.index}
            style={styles.row}
            onPress={() => {
              onSelect(s.index);
              onClose();
            }}
            accessible={true}
            accessibilityLabel={formatLabel(s)}
            accessibilityRole="button"
          >
            <View style={styles.labelRow}>
              <Text style={[styles.label, active && styles.activeLabel]}>
                {formatLabel(s)}
              </Text>
              <View style={styles.badges}>
                {s.isForced && (
                  <View style={styles.badge}>
                    <Text style={styles.badgeText}>Forced</Text>
                  </View>
                )}
                {s.isDefault && (
                  <View style={styles.badge}>
                    <Text style={styles.badgeText}>Default</Text>
                  </View>
                )}
                {s.burnIn && (
                  <View style={styles.badgeBurnIn}>
                    <Text style={styles.badgeText}>Burn-in</Text>
                  </View>
                )}
              </View>
            </View>
            {active && <Text style={styles.check}>{"\u2713"}</Text>}
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
    borderBottomColor: "rgba(255,255,255,0.08)",
  },
  labelRow: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
  },
  label: {
    color: "#fff",
    fontSize: 15,
  },
  activeLabel: {
    color: "#60a5fa",
  },
  badges: {
    flexDirection: "row",
    gap: 4,
  },
  badge: {
    backgroundColor: "rgba(255,255,255,0.12)",
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  badgeBurnIn: {
    backgroundColor: "rgba(251,146,60,0.2)",
    paddingHorizontal: 6,
    paddingVertical: 2,
    borderRadius: 4,
  },
  badgeText: {
    color: "rgba(255,255,255,0.6)",
    fontSize: 11,
    fontWeight: "500",
  },
  check: {
    color: "#60a5fa",
    fontSize: 18,
    fontWeight: "600",
    marginLeft: 12,
  },
});
