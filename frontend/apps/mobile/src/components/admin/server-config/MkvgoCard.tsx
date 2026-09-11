import { View, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../../providers/ThemeProvider";
import { ChipSelect } from "./ChipSelect";
import type { SaveEntry } from "./types";

const INGEST_SERVING_PLAN_KEY = "library.ingest_serving_plan";
const PLAYABILITY_SHADOW_KEY = "transcoding.playability_shadow";

interface MkvgoCardProps {
  getEntryValue: (key: string) => string;
  saveEntry: SaveEntry;
  saving: boolean;
}

export function MkvgoCard({ getEntryValue, saveEntry, saving }: MkvgoCardProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const ingestPlan = getEntryValue(INGEST_SERVING_PLAN_KEY);
  // Absent = the server default (on); only an explicit "off" disables it.
  const playabilityShadow = getEntryValue(PLAYABILITY_SHADOW_KEY) || "on";

  return (
    <View>
      <Text style={[styles.label, { color: theme.colors.textMuted }]}>
        {t("admin.serverConfig.ingestServingPlanLabel", "Ingest serving plan")}
      </Text>
      <ChipSelect
        value={ingestPlan}
        onChange={(v) => saveEntry({ key: INGEST_SERVING_PLAN_KEY, value: v })}
        disabled={saving}
        options={[
          { value: "", label: t("admin.serverConfig.ingestServingPlanOff", "Off") },
          { value: "plan", label: t("admin.serverConfig.ingestServingPlanPlan", "Plan") },
          { value: "plan+repair", label: t("admin.serverConfig.ingestServingPlanPlanRepair", "Plan + repair") },
        ]}
      />
      <Text style={[styles.desc, { color: theme.colors.textMuted }]}>
        {t(
          "admin.serverConfig.ingestServingPlanDesc",
          'The scan precomputes the mkvgo playback strategy for newly scanned files. "Plan + repair" also repairs a missing seek index (writes to the media file).',
        )}
      </Text>

      <Text style={[styles.label, { color: theme.colors.textMuted, marginTop: 16 }]}>
        {t("admin.serverConfig.playabilityShadowLabel", "Playability shadow")}
      </Text>
      <ChipSelect
        value={playabilityShadow}
        onChange={(v) => saveEntry({ key: PLAYABILITY_SHADOW_KEY, value: v })}
        disabled={saving}
        options={[
          { value: "off", label: t("admin.serverConfig.playabilityShadowOff", "Off") },
          { value: "on", label: t("admin.serverConfig.playabilityShadowOn", "On") },
        ]}
      />
      <Text style={[styles.desc, { color: theme.colors.textMuted }]}>
        {t(
          "admin.serverConfig.playabilityShadowDesc",
          "Compares every playback decision against the mkvgo verdict and logs mismatches. Observability only, it never influences playback.",
        )}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  label: { fontSize: 12, fontWeight: "600", textTransform: "uppercase", marginBottom: 6 },
  desc: { fontSize: 12, marginTop: 8 },
});
