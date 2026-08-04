import { useTranslation } from "react-i18next";
import { Spinner } from "@/components/ui/Spinner";
import { Checkbox } from "@/components/ui/Checkbox";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const TRICKPLAY_KEY = "trickplay.enabled";
const TRICKPLAY_CPU_KEY = "trickplay.adaptive_cpu";

export function TrickplaySettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const enabled = entries?.find((e) => e.key === TRICKPLAY_KEY)?.value !== "false";
  const adaptiveCpu = entries?.find((e) => e.key === TRICKPLAY_CPU_KEY)?.value !== "false";

  return (
    <div className="flex flex-col gap-3">
        <div className="flex items-center gap-3">
          <Checkbox
            label={t("admin.serverConfig.trickplayEnabled")}
            checked={enabled}
            onChange={(v) => setEntry.mutate({ key: TRICKPLAY_KEY, value: v ? "true" : "false" })}
            disabled={setEntry.isPending}
          />
        </div>
        <div className="flex items-center gap-3">
          <Checkbox
            label={t("admin.serverConfig.trickplayAdaptiveCpu")}
            checked={adaptiveCpu}
            onChange={(v) => setEntry.mutate({ key: TRICKPLAY_CPU_KEY, value: v ? "true" : "false" })}
            disabled={setEntry.isPending || !enabled}
          />
        </div>
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.serverConfig.trickplayAdaptiveCpuDesc")}
        </p>
        {setEntry.isPending && <Spinner size="sm" />}
    </div>
  );
}
