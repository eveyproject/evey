import { View, Text, Pressable, StyleSheet } from "react-native";
import { BottomSheet } from "./BottomSheet";

export interface QualityOption {
  label: string;
  value: number;
}

const QUALITY_OPTIONS: QualityOption[] = [
  { label: "Original", value: 0 },
  { label: "1080p", value: 1080 },
  { label: "720p", value: 720 },
  { label: "480p", value: 480 },
  { label: "360p", value: 360 },
];

interface QualityPickerProps {
  visible: boolean;
  onClose: () => void;
  currentQuality: number;
  onSelect: (quality: number) => void;
}

export function QualityPicker({
  visible,
  onClose,
  currentQuality,
  onSelect,
}: QualityPickerProps) {
  return (
    <BottomSheet visible={visible} onClose={onClose} title="Quality">
      {QUALITY_OPTIONS.map((q) => {
        const active = q.value === currentQuality;
        return (
          <Pressable
            key={q.value}
            style={styles.row}
            onPress={() => {
              onSelect(q.value);
              onClose();
            }}
            accessible={true}
            accessibilityLabel={q.label}
            accessibilityRole="button"
          >
            <Text style={[styles.label, active && styles.activeLabel]}>
              {q.label}
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
