import { useEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Select } from "@/components/ui/Select";
import { supportedLanguages } from "@/i18n";
import { useLocaleStore, useAuthStore } from "@evey/store";
import { useUserServiceUpdateProfile, userServiceRefreshToken, getUserServiceListProfilesQueryKey } from "@evey/api";
import { useQueryClient } from "@tanstack/react-query";
import { useActiveProfile } from "@/hooks/useActiveProfile";
import { useDemoLock } from "@/hooks/useDemoMode";

// Common media languages offered in the audio/subtitle selectors (ISO 639-1).
// Rendered via Intl.DisplayNames so the label is localized to the UI language.
const MEDIA_LANG_CODES = [
  "af", "ar", "bg", "bn", "ca", "cs", "cy", "da", "de", "el",
  "en", "es", "et", "eu", "fa", "fi", "fil", "fr", "ga", "gl",
  "gu", "he", "hi", "hr", "hu", "hy", "id", "is", "it", "ja",
  "ka", "kn", "ko", "lt", "lv", "mk", "ml", "mr", "ms", "mt",
  "nl", "no", "pa", "pl", "pt", "ro", "ru", "sk", "sl", "sq",
  "sr", "sv", "sw", "ta", "te", "th", "tr", "uk", "ur", "vi",
  "zh",
];

function buildMediaLangOptions(locale: string): { value: string; label: string }[] {
  let names: Intl.DisplayNames | null = null;
  try { names = new Intl.DisplayNames([locale], { type: "language" }); } catch { /* ignore */ }
  return MEDIA_LANG_CODES
    .map((code) => ({ value: code, label: names?.of(code) ?? code }))
    .sort((a, b) => a.label.localeCompare(b.label, locale));
}

export function LanguageSettings() {
  const { t, i18n } = useTranslation();
  const setLocale = useLocaleStore((s) => s.setLocale);
  const qc = useQueryClient();
  // The active profile (from the JWT), falling back to the default one.
  const { profile: activeProfile, userId } = useActiveProfile();
  const updateProfile = useUserServiceUpdateProfile();
  // Demo: the profile is shared by every visitor. The UI language stays in
  // this browser (locale store), the media-language preferences cannot be
  // saved anywhere private, so their selects are locked.
  const { demo, lock } = useDemoLock();

  // Mirror the server-truth preferences in a ref so two quick edits (e.g. audio
  // then subtitle) merge onto each other instead of both spreading the same
  // stale render snapshot - otherwise the second write would drop the first.
  const prefsRef = useRef<Record<string, string>>({});
  useEffect(() => {
    prefsRef.current = { ...activeProfile?.preferences };
  }, [activeProfile?.preferences]);

  const uiLangOptions = supportedLanguages.map((l) => ({
    value: l.code,
    label: l.label,
  }));

  const mediaLangOptions = useMemo(
    () => buildMediaLangOptions(i18n.language),
    [i18n.language],
  );
  const mediaLangOptionsWithNone = useMemo(
    () => [{ value: "", label: t("common.noPreference") }, ...mediaLangOptions],
    [mediaLangOptions, t],
  );

  const currentUiLang = supportedLanguages.find((l) => i18n.language.startsWith(l.code))?.code ?? "en";

  // Read audio/subtitle prefs from the profile preferences map (CSV for ordered list;
  // v1 stores a single language code - we take only the first entry for the select).
  const rawAudioLangs = activeProfile?.preferences?.["audio_languages"] ?? "";
  const rawSubtitleLangs = activeProfile?.preferences?.["subtitle_languages"] ?? "";
  const currentAudioLang = rawAudioLangs.split(",")[0]?.trim() ?? "";
  const currentSubtitleLang = rawSubtitleLangs.split(",")[0]?.trim() ?? "";

  const handleUiLangChange = async (code: string) => {
    // 1. Update UI language
    void i18n.changeLanguage(code);
    // 2. Update locale store (sent as Accept-Language header)
    setLocale(code);
    // 3. Update profile locale on server + refresh JWT (not on the demo: the
    //    profile is shared, the browser keeps the choice)
    if (activeProfile?.id && !demo) {
      try {
        await updateProfile.mutateAsync({ id: activeProfile.id, data: { locale: code } });
        // 4. Refresh token to get new JWT with updated locale
        const resp = await userServiceRefreshToken({});
        if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      } catch (err) { console.warn("locale update failed", err); }
    }
    // 5. Server responses are localized by locale (Accept-Language + JWT), but
    //    React Query keys don't include it - so cached lists, the home feed
    //    especially, keep the old language. refetchType "all" refetches the
    //    INACTIVE queries too (the home feed is not mounted from the settings
    //    page): the default "active" only marks them stale, so navigating back
    //    to the home showed the old-language cache (stale-while-revalidate)
    //    until a remount refetched it. Refetching inactive queries in the
    //    background now means the home is already fresh on arrival.
    void qc.invalidateQueries({ refetchType: "all" });
  };

  const handleAudioLangChange = async (code: string) => {
    if (!activeProfile?.id) return;
    // Merge onto the latest known prefs (ref), not the render snapshot, so a
    // rapid following subtitle change can't overwrite this one.
    const next = { ...prefsRef.current, audio_languages: code };
    prefsRef.current = next;
    try {
      await updateProfile.mutateAsync({ id: activeProfile.id, data: { preferences: next } });
      if (userId) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
    } catch (err) { console.warn("audio lang update failed", err); }
  };

  const handleSubtitleLangChange = async (code: string) => {
    if (!activeProfile?.id) return;
    // Merge onto the latest known prefs (ref), not the render snapshot, so a
    // rapid following audio change can't overwrite this one.
    const next = { ...prefsRef.current, subtitle_languages: code };
    prefsRef.current = next;
    try {
      await updateProfile.mutateAsync({ id: activeProfile.id, data: { preferences: next } });
      if (userId) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
    } catch (err) { console.warn("subtitle lang update failed", err); }
  };

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-4">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text)]">{t("common.language")}</h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            {t("common.languageDesc")}
          </p>
        </div>
        <Select
          label={t("common.language")}
          options={uiLangOptions}
          value={currentUiLang}
          onChange={handleUiLangChange}
        />
      </div>

      <div className="flex flex-col gap-4">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text)]">{t("common.audioLanguage")}</h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            {t("common.audioLanguageDesc")}
          </p>
        </div>
        <Select
          label={t("common.audioLanguage")}
          options={mediaLangOptionsWithNone}
          value={currentAudioLang}
          onChange={handleAudioLangChange}
          {...lock}
        />
      </div>

      <div className="flex flex-col gap-4">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text)]">{t("common.subtitleLanguage")}</h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">
            {t("common.subtitleLanguageDesc")}
          </p>
        </div>
        <Select
          label={t("common.subtitleLanguage")}
          options={mediaLangOptionsWithNone}
          value={currentSubtitleLang}
          onChange={handleSubtitleLangChange}
          {...lock}
        />
      </div>
    </div>
  );
}
