import { useTranslation } from "react-i18next";
import { ChipSelect } from "./ChipSelect";
import type { SaveEntry } from "./types";

const MEMORY_PROFILE_KEY = "server.memory_profile";

interface MemoryProfileCardProps {
  getEntryValue: (key: string) => string;
  saveEntry: SaveEntry;
  saving: boolean;
}

export function MemoryProfileCard({
  getEntryValue,
  saveEntry,
  saving,
}: MemoryProfileCardProps) {
  const { t } = useTranslation();
  const value = getEntryValue(MEMORY_PROFILE_KEY);
  const label = t("admin.serverConfig.memoryProfileLabel", "Memory profile");

  // The group's title names the choice and carries its "i": a row label
  // would only repeat it (as on the web).
  return (
    <ChipSelect
      label={label}
      value={value}
      onChange={(v) => saveEntry({ key: MEMORY_PROFILE_KEY, value: v })}
      disabled={saving}
      options={[
        {
          value: "",
          label: t(
            "admin.serverConfig.memoryProfileAuto",
            "Auto (recommended)",
          ),
        },
        {
          value: "normal",
          label: t("admin.serverConfig.memoryProfileNormal", "Normal"),
        },
        {
          value: "low",
          label: t("admin.serverConfig.memoryProfileLow", "Low"),
        },
      ]}
    />
  );
}
