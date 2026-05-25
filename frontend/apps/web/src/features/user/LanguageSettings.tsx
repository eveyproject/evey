import { useTranslation } from "react-i18next";
import { Select } from "@/components/ui/Select";
import { supportedLanguages } from "@/i18n";
import { useLocaleStore, useAuthStore } from "@evey/store";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, useUserServiceUpdateProfile, userServiceRefreshToken } from "@evey/api";
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

  const handleChange = async (code: string) => {
    // 1. Update UI language
    void i18n.changeLanguage(code);
    // 2. Update locale store (sent as Accept-Language header)
    setLocale(code);
    // 3. Update profile locale on server + refresh JWT
    if (defaultProfile?.id) {
      try {
        await updateProfile.mutateAsync({ id: defaultProfile.id, data: { locale: code } });
        // 4. Refresh token to get new JWT with updated locale
        const resp = await userServiceRefreshToken({});
        if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      } catch (err) { console.warn("locale update failed", err); }
    }
    // 5. Server responses are localized by locale (Accept-Language + JWT), but
    //    React Query keys don't include it — so cached lists, the home feed
    //    especially, keep the old language until staleTime lapses. Invalidate
    //    everything now that both locale signals are updated, so they refetch
    //    in the new language.
    void qc.invalidateQueries();
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
