import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Select } from "@/components/ui/Select";
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

  return (
    <div className="flex flex-col gap-1">
      <Select
        id="memory-profile"
        label={t("admin.serverConfig.memoryProfileLabel")}
        value={value}
        onChange={handleChange}
        options={[
          { label: t("admin.serverConfig.memoryProfileAuto"), value: "" },
          { label: t("admin.serverConfig.memoryProfileNormal"), value: "normal" },
          { label: t("admin.serverConfig.memoryProfileLow"), value: "low" },
        ]}
      />
      <p className="text-xs text-[var(--text-muted)]">
        {t("admin.serverConfig.memoryProfileDesc")}
      </p>
      {setEntry.isPending && <Spinner size="sm" />}
    </div>
  );
}
