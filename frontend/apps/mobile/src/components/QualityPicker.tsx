import { Text, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { BottomSheet } from "./BottomSheet";
import { useTheme } from "../providers/ThemeProvider";

export interface QualityOption {
  label: string;
  value: number;
}

function useQualityOptions(): QualityOption[] {
  const { t } = useTranslation();
  return [
    { label: t("player.quality.original"), value: 0 },
    { label: "1080p", value: 1080 },
    { label: "720p", value: 720 },
    { label: "480p", value: 480 },
    { label: "360p", value: 360 },
  ];
}

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
  const { t } = useTranslation();
  const theme = useTheme();
  const QUALITY_OPTIONS = useQualityOptions();

  return (
    <BottomSheet visible={visible} onClose={onClose} title={t("player.settings.quality")}>
      {QUALITY_OPTIONS.map((q) => {
        const active = q.value === currentQuality;
        return (
          <Pressable
            key={q.value}
            style={[styles.row, { borderBottomColor: theme.colors.border }]}
            onPress={() => {
              onSelect(q.value);
              onClose();
            }}
            accessible={true}
            accessibilityLabel={q.label}
            accessibilityRole="button"
          >
            <Text style={[styles.label, { color: theme.colors.text }, active && { color: theme.colors.accent }]}>
              {q.label}
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
