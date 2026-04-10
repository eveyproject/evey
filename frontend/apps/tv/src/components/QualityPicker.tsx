import { View, Text, Modal, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";

const QUALITY_OPTIONS = [
  { label: "Original", value: 0 },
  { label: "1080p", value: 1080 },
  { label: "720p", value: 720 },
  { label: "480p", value: 480 },
  { label: "360p", value: 360 },
];

interface Props {
  visible: boolean;
  currentQuality: number;
  onSelect: (quality: number) => void;
  onClose: () => void;
}

export function QualityPicker({ visible, currentQuality, onSelect, onClose }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();

  return (
    <Modal visible={visible} animationType="fade" transparent onRequestClose={onClose}>
      <Pressable style={styles.backdrop} onPress={onClose}>
        <Pressable
          style={[styles.sheet, { backgroundColor: theme.colors.surface }]}
          onPress={(e) => e.stopPropagation()}
        >
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("player.settings.quality", "Quality")}
          </Text>
          <View style={styles.list}>
            {QUALITY_OPTIONS.map((q, i) => {
              const active = q.value === currentQuality;
              return (
                <FocusableCard
                  key={q.value}
                  hasTVPreferredFocus={i === 0}
                  onPress={() => {
                    onSelect(q.value);
                    onClose();
                  }}
                  style={[styles.row, { borderBottomColor: theme.colors.border }]}
                >
                  <Text style={[styles.label, { color: active ? theme.colors.accent : theme.colors.text }]}>
                    {q.label}
                  </Text>
                  {active && (
                    <Text style={[styles.check, { color: theme.colors.accent }]}>{"\u2713"}</Text>
                  )}
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
    flex: 1,
  },
  check: {
    fontSize: 18,
    fontWeight: "600",
    marginLeft: 12,
  },
});
