import { useTranslation } from "react-i18next";
import { ThemeSelector } from "@/components/ThemeSelector";

export function ThemeSettings() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-2">
      <h3 className="text-sm font-semibold text-[var(--text)]">{t("common.appearance")}</h3>
      <p className="text-xs text-[var(--text-muted)]">{t("common.appearanceDesc")}</p>
      <ThemeSelector />
    </div>
  );
}
