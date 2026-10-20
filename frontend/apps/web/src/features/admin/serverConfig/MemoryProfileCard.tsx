import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { SegmentedControl } from "@/components/ui/SegmentedControl";
import { SettingRow } from "@/components/ui/SettingRow";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const MEMORY_PROFILE_KEY = "server.memory_profile";

export function MemoryProfileCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const value = entries?.find((e) => e.key === MEMORY_PROFILE_KEY)?.value ?? "";

  function handleChange(v: string) {
    setEntry.mutate(
      { key: MEMORY_PROFILE_KEY, value: v },
      { onSuccess: () => toast.success(t("admin.serverConfig.configUpdated")) },
    );
  }

  const label = t("admin.serverConfig.memoryProfileLabel");
  return (
    <SettingRow label={label} info={t("admin.serverConfig.memoryProfileDesc")} hint={t("admin.serverConfig.memoryProfileSubtitle")} kind="wide" data-testid="memory-profile-row">
      <SegmentedControl
        id="memory-profile"
        label={label}
        value={value}
        onChange={handleChange}
        disabled={setEntry.isPending}
        options={[
          { label: t("admin.serverConfig.memoryProfileAuto"), value: "" },
          { label: t("admin.serverConfig.memoryProfileNormal"), value: "normal" },
          { label: t("admin.serverConfig.memoryProfileLow"), value: "low" },
        ]}
      />
      {setEntry.isPending && <Spinner size="sm" />}
    </SettingRow>
  );
}
