import { View } from "react-native";
import { useTranslation } from "react-i18next";
import { SettingRow } from "../settings/SettingRow";
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
  const { t } = useTranslation();
  const ingestPlan = getEntryValue(INGEST_SERVING_PLAN_KEY);
  // Absent = the server default (on); only an explicit "off" disables it.
  const playabilityShadow = getEntryValue(PLAYABILITY_SHADOW_KEY) || "on";
  const ingestLabel = t("admin.serverConfig.ingestServingPlanLabel", "Ingest serving plan");
  const shadowLabel = t("admin.serverConfig.playabilityShadowLabel", "Playability shadow");

  return (
    <View>
      <SettingRow label={ingestLabel} info={t("admin.serverConfig.ingestServingPlanDesc")} kind="wide">
        <ChipSelect
          label={ingestLabel}
          value={ingestPlan}
          onChange={(v) => saveEntry({ key: INGEST_SERVING_PLAN_KEY, value: v })}
          disabled={saving}
          options={[
            { value: "", label: t("admin.serverConfig.ingestServingPlanOff", "Off") },
            { value: "plan", label: t("admin.serverConfig.ingestServingPlanPlan", "Plan") },
            { value: "plan+repair", label: t("admin.serverConfig.ingestServingPlanPlanRepair", "Plan + repair") },
          ]}
        />
      </SettingRow>
      <SettingRow label={shadowLabel} info={t("admin.serverConfig.playabilityShadowDesc")} kind="wide">
        <ChipSelect
          label={shadowLabel}
          value={playabilityShadow}
          onChange={(v) => saveEntry({ key: PLAYABILITY_SHADOW_KEY, value: v })}
          disabled={saving}
          options={[
            { value: "off", label: t("admin.serverConfig.playabilityShadowOff", "Off") },
            { value: "on", label: t("admin.serverConfig.playabilityShadowOn", "On") },
          ]}
        />
      </SettingRow>
    </View>
  );
}
