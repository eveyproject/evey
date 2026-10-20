import { useTranslation } from "react-i18next";
import { Spinner } from "@/components/ui/Spinner";
import { Switch } from "@/components/ui/Switch";
import { SettingRow } from "@/components/ui/SettingRow";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const INTRO_PROVIDERS_KEY = "intro_providers.enabled";
const INTRO_TIDB_SUBMIT_KEY = "intro_providers.tidb_submit_enabled";

// IntroDetectionSettingsCard surfaces the runtime knobs of the external
// intro/recap/credits providers (TIDB + IntroDB). The TIDB Bearer token
// itself lives in the API keys card above (api.tidb_key) - it's only
// useful when this toggle is on. The detection's CPU share is set in the
// CPU budget card (background.cpu_percent.intro_detection).
//
// Toggling 'enabled' here writes to server_config but the providers are
// only constructed once at boot - admins must restart the server for the
// change to take effect. The note in the description spells that out.
export function IntroDetectionSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();

  const enabled = entries?.find((e) => e.key === INTRO_PROVIDERS_KEY)?.value === "true";
  const tidbKey = entries?.find((e) => e.key === "api.tidb_key")?.value ?? "";
  const tidbSubmitEnabled = entries?.find((e) => e.key === INTRO_TIDB_SUBMIT_KEY)?.value === "true";
  return (
    <div className="flex flex-col divide-y divide-[var(--border)]">
      <SettingRow
        label={t("admin.serverConfig.introProvidersEnabled")}
        info={t("admin.serverConfig.introProvidersInfo")}
        hint={t("admin.serverConfig.introProvidersDesc")}
        restart
        kind="toggle"
      >
        <Switch
          label={t("admin.serverConfig.introProvidersEnabled")}
          checked={enabled}
          onChange={() => setEntry.mutate({ key: INTRO_PROVIDERS_KEY, value: enabled ? "false" : "true" })}
          disabled={setEntry.isPending}
          data-testid="intro-providers-toggle"
        />
      </SettingRow>
      {enabled && (
        <div className="flex flex-col gap-1 py-3 text-xs text-[var(--text-muted)] last:pb-0">
          <span>
            {t("admin.serverConfig.introProvidersTidbStatus")}{" "}
            {tidbKey ? (
              <span className="text-[var(--accent)]">{t("admin.serverConfig.introProvidersTidbAuthed")}</span>
            ) : (
              <span className="text-[var(--color-warning)]">{t("admin.serverConfig.introProvidersTidbAnonymous")}</span>
            )}
          </span>
          <span>{t("admin.serverConfig.introProvidersIntrodbStatus")}</span>
        </div>
      )}
      {enabled && tidbKey && (
        <SettingRow
          label={t("admin.serverConfig.introProvidersTidbSubmit")}
          info={t("admin.serverConfig.introProvidersTidbSubmitDesc")}
          kind="toggle"
        >
          <Switch
            label={t("admin.serverConfig.introProvidersTidbSubmit")}
            checked={tidbSubmitEnabled}
            onChange={() => setEntry.mutate({ key: INTRO_TIDB_SUBMIT_KEY, value: tidbSubmitEnabled ? "false" : "true" })}
            disabled={setEntry.isPending}
            data-testid="intro-tidb-submit-toggle"
          />
        </SettingRow>
      )}
      {setEntry.isPending && <div className="pt-3"><Spinner size="sm" /></div>}
    </div>
  );
}
