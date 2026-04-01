import { View, Text, Pressable, StyleSheet } from "react-native";
import { useTheme } from "../providers/ThemeProvider";
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
  const theme = useTheme();
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
            <Text style={[styles.label, { color: theme.colors.text }, active && { color: theme.colors.accent }]}>
              {formatLabel(s)}
            </Text>
            {active && <Text style={[styles.check, { color: theme.colors.accent }]}>{"\u2713"}</Text>}
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
    fontSize: 15,
    flex: 1,
  },
  check: {
    fontSize: 18,
    fontWeight: "600",
    marginLeft: 12,
  },
});
