import { View, Text, Pressable, StyleSheet } from "react-native";
import { BottomSheet } from "./BottomSheet";

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
  const detail = s.title ?? (s.channels ? `${s.channels}ch` : "");
  return detail ? `${lang} \u2014 ${detail}` : lang;
}

export function AudioTrackPicker({
  visible,
  onClose,
  streams,
  currentIndex,
  onSelect,
}: AudioTrackPickerProps) {
  return (
    <BottomSheet visible={visible} onClose={onClose} title="Audio">
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
            <Text style={[styles.label, active && styles.activeLabel]}>
              {formatLabel(s)}
            </Text>
            {active && <Text style={styles.check}>{"\u2713"}</Text>}
          </Pressable>
        );
      })}
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
    borderBottomColor: "rgba(255,255,255,0.08)",
  },
  label: {
    color: "#fff",
    fontSize: 15,
    flex: 1,
  },
  activeLabel: {
    color: "#60a5fa",
  },
  check: {
    color: "#60a5fa",
    fontSize: 18,
    fontWeight: "600",
    marginLeft: 12,
  },
});
