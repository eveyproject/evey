import { useTranslation } from "react-i18next";
import { Select } from "@/components/ui/Select";
import { supportedLanguages } from "@/i18n";
import { useLocaleStore } from "@evey/store";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, useUserServiceUpdateProfile, getUserServiceListProfilesQueryKey } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";

export function LanguageSettings() {
  const { t, i18n } = useTranslation();
  const setLocale = useLocaleStore((s) => s.setLocale);
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const defaultProfile = profilesResp?.profiles?.find((p) => p.isDefault) ?? profilesResp?.profiles?.[0];
  const updateProfile = useUserServiceUpdateProfile();

  const options = supportedLanguages.map((l) => ({
    value: l.code,
    label: l.label,
  }));

  const current = supportedLanguages.find((l) => i18n.language.startsWith(l.code))?.code ?? "en";

  const handleChange = (code: string) => {
    // 1. Update UI language
    void i18n.changeLanguage(code);
    // 2. Update locale store (sent as Accept-Language header)
    setLocale(code);
    // 3. Update profile locale on server
    if (defaultProfile?.id) {
      updateProfile.mutate(
        { id: defaultProfile.id, data: { locale: code } },
        { onSuccess: () => { if (user?.id) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) }); } },
      );
    }
  };

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("common.language")}</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          {t("common.languageDesc")}
        </p>
      </div>
      <Select
        label={t("common.language")}
        options={options}
        value={current}
        onChange={handleChange}
      />
    </div>
  );
}
