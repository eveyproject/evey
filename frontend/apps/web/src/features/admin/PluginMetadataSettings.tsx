import { useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminPluginServiceGetPluginMetadataSettings,
  useAdminPluginServiceUpdatePluginMetadataSettings,
  useAdminPluginServiceRefreshPluginMetadata,
  getAdminPluginServiceGetPluginMetadataSettingsQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { useDemoLockFor } from "@/hooks/useDemoMode";

// How often the card re-reads the library fetch while it runs.
const REFRESH_POLL_MS = 2000;

/**
 * The part of a metadata plugin's card (capability metadata.provider) that
 * says which of TMDB's fields the plugin replaces, and fetches the library
 * that was there before the plugin was installed.
 */
export function PluginMetadataSettings({ pluginId }: { pluginId: string }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { lock: saveLock } = useDemoLockFor("/evey.v1.AdminPluginService/UpdatePluginMetadataSettings");
  const { lock: refreshLock } = useDemoLockFor("/evey.v1.AdminPluginService/RefreshPluginMetadata");
  const [running, setRunning] = useState(false);
  const { data, isLoading } = useAdminPluginServiceGetPluginMetadataSettings(pluginId, {
    query: { refetchInterval: running ? REFRESH_POLL_MS : false },
  });
  const [fields, setFields] = useState<string[]>([]);

  useEffect(() => {
    if (!data) return;
    setFields(data.fields ?? []);
    setRunning(!!data.refresh?.running);
  }, [data]);

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: getAdminPluginServiceGetPluginMetadataSettingsQueryKey(pluginId) });
  };
  const save = useAdminPluginServiceUpdatePluginMetadataSettings({
    mutation: {
      onSuccess: () => { invalidate(); toast.success(t("admin.plugin.metadata.saved")); },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.metadata.saveFailed"))),
    },
  });
  const refresh = useAdminPluginServiceRefreshPluginMetadata({
    mutation: {
      onSuccess: () => { setRunning(true); invalidate(); toast.success(t("admin.plugin.metadata.refreshStarted")); },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.metadata.refreshFailed"))),
    },
  });

  if (isLoading) {
    return <div className="flex justify-center py-3"><Spinner /></div>;
  }
  const available = data?.availableFields ?? [];
  const status = data?.refresh;
  const toggle = (f: string) => setFields((cur) => (cur.includes(f) ? cur.filter((x) => x !== f) : [...cur, f]));

  return (
    <div className="flex flex-col gap-3" data-testid="plugin-metadata-settings">
      <p className="text-xs text-[var(--text-muted)]">{t("admin.plugin.metadata.description")}</p>
      <fieldset className="flex flex-col gap-1.5">
        <legend className="text-xs font-semibold mb-1 text-[var(--text-muted)]">
          {t("admin.plugin.metadata.fieldsLabel")}
        </legend>
        {available.map((f) => (
          <label key={f} className="flex items-center gap-2 text-sm text-[var(--text)] cursor-pointer">
            <input
              type="checkbox"
              checked={fields.includes(f)}
              onChange={() => toggle(f)}
              disabled={saveLock.disabled}
              data-testid={`metadata-field-${f}`}
            />
            {t(`admin.plugin.metadata.fields.${f}`)}
          </label>
        ))}
      </fieldset>
      <div className="flex justify-end">
        <Button

          onClick={() => save.mutate({ pluginId, data: { fields } })}
          loading={save.isPending}
          data-testid="metadata-save"
          {...saveLock}
        >
          {t("admin.plugin.metadata.save")}
        </Button>
      </div>
      <div className="flex flex-col gap-2 border-t border-[var(--border)] pt-3">
        <p className="text-xs text-[var(--text-muted)]">{t("admin.plugin.metadata.refreshHint")}</p>
        {status?.running ? (
          <p className="text-sm text-[var(--text)]" data-testid="metadata-refresh-running">
            {t("admin.plugin.metadata.running", { done: status.done ?? 0, total: status.total ?? 0 })}
          </p>
        ) : status?.finishedAt ? (
          <p className="text-sm text-[var(--text)]" data-testid="metadata-refresh-finished">
            {t("admin.plugin.metadata.finished", {
              covered: status.covered ?? 0,
              total: status.total ?? 0,
              failed: status.failed ?? 0,
            })}
          </p>
        ) : null}
        <div className="flex justify-end">
          <Button
            variant="secondary"
            onClick={() => refresh.mutate({ pluginId, data: {} })}
            loading={refresh.isPending}
            disabled={!!status?.running}
            data-testid="metadata-refresh"
            {...refreshLock}
          >
            {t("admin.plugin.metadata.refresh")}
          </Button>
        </div>
      </div>
    </div>
  );
}
