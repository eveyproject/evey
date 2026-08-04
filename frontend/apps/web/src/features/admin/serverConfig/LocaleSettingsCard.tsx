import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { toast, useLocaleStore } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Select } from "@/components/ui/Select";
import { ISO_REGIONS } from "@/features/user/regionList";
import { defaultRegionForLocale } from "@/features/user/regionDefaults";
import { supportedLanguages } from "@/i18n";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

// Kept in sync with backend i18n supported locales + frontend setup wizard.
// Admin panel must use the same short codes ("fr", "en" ...) that the setup
// wizard persists to server.language - otherwise the Select can't match the
// stored value and shows the placeholder ("envDefault") on every render.
const LOCALE_OPTIONS = supportedLanguages.map((l) => ({ value: l.code, label: l.label }));

const SERVER_LANG_KEY = "server.language";
const SERVER_REGION_KEY = "server.region";
const METADATA_LANG_KEY = "metadata.language";

function regionOptionsForLocale(locale: string): { value: string; label: string }[] {
  const sv = (Intl as unknown as { supportedValuesOf?: (k: string) => string[] }).supportedValuesOf;
  let codes: string[] = [];
  try { codes = typeof sv === "function" ? sv("region") : []; } catch { codes = []; }
  if (codes.length === 0) codes = Array.from(ISO_REGIONS);
  let names: Intl.DisplayNames | null = null;
  try { names = new Intl.DisplayNames([locale || "en"], { type: "region" }); } catch { names = null; }
  return codes
    .filter((c) => /^[A-Z]{2}$/.test(c))
    .map((c) => ({ value: c, label: names?.of(c) ?? c }))
    .sort((a, b) => a.label.localeCompare(b.label, locale || "en"));
}

export function LocaleSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t, i18n } = useTranslation();
  const setEntry = useSetConfigEntry();
  const setLocale = useLocaleStore((s) => s.setLocale);

  const serverLang = entries?.find((e) => e.key === SERVER_LANG_KEY)?.value ?? "";
  const serverRegion = entries?.find((e) => e.key === SERVER_REGION_KEY)?.value ?? "";
  const metaLang = entries?.find((e) => e.key === METADATA_LANG_KEY)?.value ?? "";

  function handleServerLang(value: string) {
    setEntry.mutate(
      { key: SERVER_LANG_KEY, value },
      { onSuccess: () => {
          if (value) setLocale(value);
          toast.success(t("admin.serverConfig.languageUpdated"));
        } },
    );
  }
  function handleServerRegion(value: string) {
    setEntry.mutate(
      { key: SERVER_REGION_KEY, value },
      { onSuccess: () => toast.success(t("admin.serverConfig.languageUpdated")) },
    );
  }
  function handleMetaLang(value: string) {
    setEntry.mutate(
      { key: METADATA_LANG_KEY, value },
      { onSuccess: () => toast.success(t("admin.serverConfig.languageUpdated")) },
    );
  }

  const regionOptions = useMemo(() => regionOptionsForLocale(i18n.language || "en"), [i18n.language]);

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Select
          id="server-lang"
          label={t("admin.serverConfig.serverLanguage")}
          value={serverLang}
          onChange={handleServerLang}
          options={[{ label: t("admin.serverConfig.envDefault"), value: "" }, ...LOCALE_OPTIONS]}
        />
        <Select
          id="server-region"
          label={t("admin.serverConfig.serverRegion")}
          value={serverRegion}
          onChange={handleServerRegion}
          options={[
            { label: `${t("admin.serverConfig.envDefault")} (${defaultRegionForLocale(serverLang)})`, value: "" },
            ...regionOptions,
          ]}
        />
        <Select
          id="metadata-lang"
          label={t("admin.serverConfig.metadataLanguage")}
          value={metaLang}
          onChange={handleMetaLang}
          options={[{ label: t("admin.serverConfig.envDefault"), value: "" }, ...LOCALE_OPTIONS]}
        />
      </div>
      {setEntry.isPending && <div><Spinner size="sm" /></div>}
    </div>
  );
}
