import { useTranslation } from "react-i18next";
import { ThemeSelector } from "@/components/ThemeSelector";

export function ThemeSettings() {
  const { t } = useTranslation("common");

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-semibold text-[var(--text)]">{t("appearance")}</h3>
      <p className="text-xs text-[var(--text-muted)]">{t("appearanceDesc")}</p>
      <ThemeSelector />
    </div>
  );
}
