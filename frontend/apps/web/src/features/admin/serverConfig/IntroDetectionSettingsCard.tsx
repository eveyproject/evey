import { useTranslation } from "react-i18next";
import { Spinner } from "@/components/ui/Spinner";
import { Checkbox } from "@/components/ui/Checkbox";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const INTRO_PROVIDERS_KEY = "intro_providers.enabled";
const INTRO_TIDB_SUBMIT_KEY = "intro_providers.tidb_submit_enabled";

// IntroDetectionSettingsCard surfaces the runtime knobs of the external
// intro/recap/credits providers (TIDB + IntroDB). The TIDB Bearer token
// itself lives in the API keys card above (api.tidb_key) - it's only
// useful when this toggle is on.
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
    <div className="flex flex-col gap-3">
        <Checkbox
          label={t("admin.serverConfig.introProvidersEnabled")}
          checked={enabled}
          onChange={(v) => setEntry.mutate({ key: INTRO_PROVIDERS_KEY, value: v ? "true" : "false" })}
          disabled={setEntry.isPending}
        />
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.serverConfig.introProvidersRestart")}
        </p>
        {enabled && (
          <div className="text-xs text-[var(--text-muted)] flex flex-col gap-1 mt-1 pl-1 border-l-2 border-[var(--border)] pl-3">
            <span>
              {t("admin.serverConfig.introProvidersTidbStatus")}{" "}
              {tidbKey ? (
                <span className="text-[var(--accent)]">
                  {t("admin.serverConfig.introProvidersTidbAuthed")}
                </span>
              ) : (
                <span className="text-[var(--color-warning)]">
                  {t("admin.serverConfig.introProvidersTidbAnonymous")}
                </span>
              )}
            </span>
            <span>{t("admin.serverConfig.introProvidersIntrodbStatus")}</span>
            {tidbKey && (
              <div className="mt-2">
                <Checkbox
                  label={t("admin.serverConfig.introProvidersTidbSubmit")}
                  checked={tidbSubmitEnabled}
                  onChange={(v) =>
                    setEntry.mutate({ key: INTRO_TIDB_SUBMIT_KEY, value: v ? "true" : "false" })
                  }
                  disabled={setEntry.isPending}
                />
                <p className="text-xs text-[var(--text-muted)] mt-1 pl-6">
                  {t("admin.serverConfig.introProvidersTidbSubmitDesc")}
                </p>
              </div>
            )}
          </div>
        )}
        {setEntry.isPending && <Spinner size="sm" />}
    </div>
  );
}
