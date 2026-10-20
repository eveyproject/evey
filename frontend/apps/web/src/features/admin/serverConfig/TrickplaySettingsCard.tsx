import { useTranslation } from "react-i18next";
import { Switch } from "@/components/ui/Switch";
import { SettingRow } from "@/components/ui/SettingRow";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const TRICKPLAY_KEY = "trickplay.enabled";

// The generation's CPU share is set in the CPU budget card (background.cpu_percent.*).
export function TrickplaySettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const enabled = entries?.find((e) => e.key === TRICKPLAY_KEY)?.value !== "false";

  return (
    <SettingRow
      label={t("admin.serverConfig.trickplayEnabled")}
      info={t("admin.settingsHelp.trickplay")}
      hint={t("admin.serverConfig.trickplayCpuHint")}
      kind="toggle"
      data-testid="trickplay-settings"
    >
      <Switch
        label={t("admin.serverConfig.trickplayEnabled")}
        checked={enabled}
        onChange={() => setEntry.mutate({ key: TRICKPLAY_KEY, value: enabled ? "false" : "true" })}
        disabled={setEntry.isPending}
        data-testid="trickplay-toggle"
      />
    </SettingRow>
  );
}
