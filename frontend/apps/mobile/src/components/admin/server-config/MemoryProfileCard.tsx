import { View, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../providers/ThemeProvider";
import { ChipSelect } from "./ChipSelect";
import type { SaveEntry } from "./types";

const MEMORY_PROFILE_KEY = "server.memory_profile";

interface MemoryProfileCardProps {
  getEntryValue: (key: string) => string;
  saveEntry: SaveEntry;
  saving: boolean;
}

export function MemoryProfileCard({ getEntryValue, saveEntry, saving }: MemoryProfileCardProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const value = getEntryValue(MEMORY_PROFILE_KEY);

  return (
    <View>
      <Text style={[styles.label, { color: theme.colors.textMuted }]}>
        {t("admin.serverConfig.memoryProfileLabel", "Memory profile")}
      </Text>
      <ChipSelect
        value={value}
        onChange={(v) => saveEntry({ key: MEMORY_PROFILE_KEY, value: v })}
        disabled={saving}
        options={[
          { value: "", label: t("admin.serverConfig.memoryProfileAuto", "Auto (recommended)") },
          { value: "normal", label: t("admin.serverConfig.memoryProfileNormal", "Normal") },
          { value: "low", label: t("admin.serverConfig.memoryProfileLow", "Low") },
        ]}
      />
      <Text style={[styles.desc, { color: theme.colors.textMuted }]}>
        {t(
          "admin.serverConfig.memoryProfileDesc",
          "Low suspends idle plugin processes to save RAM (resumed on demand); Auto picks Low on machines under 3 GB.",
        )}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  label: { fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginBottom: 6 },
  desc: { fontSize: 12, marginTop: 8 },
});
