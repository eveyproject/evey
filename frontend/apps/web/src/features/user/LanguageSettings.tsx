import { useTranslation } from "react-i18next";
import { Select } from "@/components/ui/Select";
import { supportedLanguages } from "@/i18n";

export function LanguageSettings() {
  const { t, i18n } = useTranslation("common");

  const options = supportedLanguages.map((l) => ({
    value: l.code,
    label: l.label,
  }));

  const current = supportedLanguages.find((l) => i18n.language.startsWith(l.code))?.code ?? "en";

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("language")}</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          {t("languageDesc")}
        </p>
      </div>
      <Select
        label={t("language")}
        options={options}
        value={current}
        onChange={(code) => void i18n.changeLanguage(code)}
      />
    </div>
  );
}
