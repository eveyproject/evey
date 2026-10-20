import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { SegmentedControl } from "@/components/ui/SegmentedControl";
import { SettingRow } from "@/components/ui/SettingRow";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const INGEST_SERVING_PLAN_KEY = "library.ingest_serving_plan";
const PLAYABILITY_SHADOW_KEY = "transcoding.playability_shadow";

export function MkvgoCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const ingestPlan = entries?.find((e) => e.key === INGEST_SERVING_PLAN_KEY)?.value ?? "";
  // Absent = the server default (on); only an explicit "off" disables it.
  const playabilityShadow = entries?.find((e) => e.key === PLAYABILITY_SHADOW_KEY)?.value || "on";

  function write(key: string, v: string) {
    setEntry.mutate({ key, value: v }, { onSuccess: () => toast.success(t("admin.serverConfig.configUpdated")) });
  }

  const ingestLabel = t("admin.serverConfig.ingestServingPlanLabel");
  const shadowLabel = t("admin.serverConfig.playabilityShadowLabel");
  return (
    <div className="flex flex-col divide-y divide-[var(--border)]">
      <SettingRow label={ingestLabel} info={t("admin.serverConfig.ingestServingPlanDesc")} kind="wide">
        <SegmentedControl
          id="ingest-serving-plan"
          label={ingestLabel}
          value={ingestPlan}
          onChange={(v) => write(INGEST_SERVING_PLAN_KEY, v)}
          disabled={setEntry.isPending}
          options={[
            { label: t("admin.serverConfig.ingestServingPlanOff"), value: "" },
            { label: t("admin.serverConfig.ingestServingPlanPlan"), value: "plan" },
            { label: t("admin.serverConfig.ingestServingPlanPlanRepair"), value: "plan+repair" },
          ]}
        />
      </SettingRow>
      <SettingRow label={shadowLabel} info={t("admin.serverConfig.playabilityShadowDesc")} kind="wide">
        <SegmentedControl
          id="playability-shadow"
          label={shadowLabel}
          value={playabilityShadow}
          onChange={(v) => write(PLAYABILITY_SHADOW_KEY, v)}
          disabled={setEntry.isPending}
          options={[
            { label: t("admin.serverConfig.playabilityShadowOff"), value: "off" },
            { label: t("admin.serverConfig.playabilityShadowOn"), value: "on" },
          ]}
        />
      </SettingRow>
      {setEntry.isPending && <div className="pt-3"><Spinner size="sm" /></div>}
    </div>
  );
}
