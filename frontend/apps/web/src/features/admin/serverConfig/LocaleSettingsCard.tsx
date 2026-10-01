import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { toast, useLocaleStore } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Select } from "@/components/ui/Select";
import { configDefaultsKey, defaultRegionForLocale, getConfigDefaults } from "@evey/api";
import { timeZoneOptions, useServerClock } from "@evey/client";
import { ISO_REGIONS } from "@/features/user/regionList";
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
// Kids profiles' opening hours are read in this zone (server-side).
const SERVER_TZ_KEY = "server.timezone";

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
  const serverTz = entries?.find((e) => e.key === SERVER_TZ_KEY)?.value ?? "";

  function handleServerLang(value: string) {
    setEntry.mutate(
      { key: SERVER_LANG_KEY, value },
      { onSuccess: () => {
          // The admin's own UI mirrors the new server default; mark it
          // "server" so it never masquerades as a personal pick in the
          // language-precedence chain.
          if (value) setLocale(value, "server");
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

  function handleServerTz(value: string) {
    setEntry.mutate(
      { key: SERVER_TZ_KEY, value },
      { onSuccess: () => toast.success(t("admin.serverConfig.timezoneUpdated")) },
    );
  }
  const tzOptions = useMemo(() => timeZoneOptions(serverTz).map((z) => ({ value: z, label: z.replace(/_/g, " ") })), [serverTz]);

  const regionOptions = useMemo(() => regionOptionsForLocale(i18n.language || "en"), [i18n.language]);

  // "Default: French", what an empty setting really resolves to. The region
  // and the metadata language follow the server language the panel shows.
  const { data: defaults } = useQuery({ queryKey: configDefaultsKey, queryFn: ({ signal }) => getConfigDefaults(signal) });
  const langName = (code?: string) => LOCALE_OPTIONS.find((o) => o.value === code)?.label ?? code ?? "";
  const effectiveLang = serverLang || defaults?.["server.language"] || "";
  const regionName = (code: string) => {
    try {
      return new Intl.DisplayNames([i18n.language || "en"], { type: "region" }).of(code) ?? code;
    } catch {
      return code;
    }
  };
  // The time it is now in the zone in force, by the server's clock.
  const serverTime = useServerClock(defaults?.now, serverTz || defaults?.["server.timezone"], i18n.language || "en");
  const defaultLabel = (value: string | undefined) =>
    value ? t("admin.serverConfig.defaultValue", { value }) : t("admin.serverConfig.envDefault");

  return (
    <div className="space-y-4">
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <Select
          id="server-lang"
          label={t("admin.serverConfig.serverLanguage")}
          value={serverLang}
          onChange={handleServerLang}
          options={[{ label: defaultLabel(langName(defaults?.["server.language"])), value: "" }, ...LOCALE_OPTIONS]}
        />
        <Select
          id="server-region"
          label={t("admin.serverConfig.serverRegion")}
          value={serverRegion}
          onChange={handleServerRegion}
          options={[
            { label: defaultLabel(effectiveLang ? regionName(defaultRegionForLocale(effectiveLang)) : undefined), value: "" },
            ...regionOptions,
          ]}
        />
        <Select
          id="metadata-lang"
          label={t("admin.serverConfig.metadataLanguage")}
          value={metaLang}
          onChange={handleMetaLang}
          options={[{ label: defaultLabel(langName(effectiveLang)), value: "" }, ...LOCALE_OPTIONS]}
        />
        <div className="flex flex-col gap-1">
          <Select
            id="server-timezone"
            label={t("admin.serverConfig.timezone")}
            value={serverTz}
            onChange={handleServerTz}
            options={[{ label: defaultLabel(defaults?.["server.timezone"]?.replace(/_/g, " ")), value: "" }, ...tzOptions]}
          />
          {serverTime && (
            <p className="text-sm font-medium text-[var(--text)]" data-testid="server-time">
              {t("admin.serverConfig.timezoneNow", { time: serverTime })}
            </p>
          )}
          <p className="text-xs text-[var(--text-muted)]">{t("admin.serverConfig.timezoneHint")}</p>
        </div>
      </div>
      {setEntry.isPending && <div><Spinner size="sm" /></div>}
    </div>
  );
}
