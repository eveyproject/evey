import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { SegmentedControl } from "@/components/ui/SegmentedControl";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const MEMORY_PROFILE_KEY = "server.memory_profile";

export function MemoryProfileCard({ entries, labelledBy }: { entries: ConfigEntry[] | undefined; labelledBy?: string }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const value = entries?.find((e) => e.key === MEMORY_PROFILE_KEY)?.value ?? "";

  function handleChange(v: string) {
    setEntry.mutate(
      { key: MEMORY_PROFILE_KEY, value: v },
      { onSuccess: () => toast.success(t("admin.serverConfig.configUpdated")) },
    );
  }

  return (
    <div className="flex flex-col items-start gap-2" data-testid="memory-profile-row">
      {/* The group's title names the choice (labelledBy): a row label would
          only repeat it. Saved on pick: the arrows only move. */}
      <SegmentedControl
        id="memory-profile"
        label={t("admin.serverConfig.memoryProfileLabel")}
        labelledBy={labelledBy}
        activation="manual"
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
    </div>
  );
}
