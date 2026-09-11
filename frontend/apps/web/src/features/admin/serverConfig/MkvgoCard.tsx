import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Select } from "@/components/ui/Select";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const INGEST_SERVING_PLAN_KEY = "library.ingest_serving_plan";
const PLAYABILITY_SHADOW_KEY = "transcoding.playability_shadow";

export function MkvgoCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const ingestPlan = entries?.find((e) => e.key === INGEST_SERVING_PLAN_KEY)?.value ?? "";
  // Absent = the server default (on); only an explicit "off" disables it.
  const playabilityShadow = entries?.find((e) => e.key === PLAYABILITY_SHADOW_KEY)?.value || "on";

  function handleIngestPlan(v: string) {
    setEntry.mutate(
      { key: INGEST_SERVING_PLAN_KEY, value: v },
      { onSuccess: () => toast.success(t("admin.serverConfig.configUpdated")) },
    );
  }

  function handlePlayabilityShadow(v: string) {
    setEntry.mutate(
      { key: PLAYABILITY_SHADOW_KEY, value: v },
      { onSuccess: () => toast.success(t("admin.serverConfig.configUpdated")) },
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1">
        <Select
          id="ingest-serving-plan"
          label={t("admin.serverConfig.ingestServingPlanLabel")}
          value={ingestPlan}
          onChange={handleIngestPlan}
          options={[
            { label: t("admin.serverConfig.ingestServingPlanOff"), value: "" },
            { label: t("admin.serverConfig.ingestServingPlanPlan"), value: "plan" },
            { label: t("admin.serverConfig.ingestServingPlanPlanRepair"), value: "plan+repair" },
          ]}
        />
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.serverConfig.ingestServingPlanDesc")}
        </p>
      </div>
      <div className="flex flex-col gap-1">
        <Select
          id="playability-shadow"
          label={t("admin.serverConfig.playabilityShadowLabel")}
          value={playabilityShadow}
          onChange={handlePlayabilityShadow}
          options={[
            { label: t("admin.serverConfig.playabilityShadowOff"), value: "off" },
            { label: t("admin.serverConfig.playabilityShadowOn"), value: "on" },
          ]}
        />
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.serverConfig.playabilityShadowDesc")}
        </p>
      </div>
      {setEntry.isPending && <Spinner size="sm" />}
    </div>
  );
}
