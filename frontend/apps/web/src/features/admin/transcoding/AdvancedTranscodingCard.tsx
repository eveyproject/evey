import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { useServerSettingsForm, type ServerSettingField } from "@/features/admin/serverConfig/serverSettings";

const FIELDS: ServerSettingField[] = [
  "transcodeMaxSessions",
  "softwareMaxHeight",
  "softwareHDRMaxHeight",
  "softwareAV1MaxHeight",
  "transcodeSingleVariant",
  "transcodeHWPerVariant",
];

// AdvancedTranscodingCard holds the expert transcoding limits. Most were
// reachable only through environment variables until 2026-07-30 (editing a
// compose file and recreating the container to change a playback ceiling);
// the AV1 software ceiling was accepted by the server but offered nowhere.
//
// Most of them are read at boot: each row carries the restart badge the
// server's own requiresRestart flag asks for, so a knob that becomes live
// later loses its badge without anyone editing this file. A save sends only
// the rows changed here, and a refetch never blanks a row being edited.
export function AdvancedTranscodingCard() {
  const { t } = useTranslation();
  const form = useServerSettingsForm(FIELDS);

  if (form.isLoading || !form.data) return <div className="flex justify-center py-6"><Spinner size="md" /></div>;

  const needsRestart = FIELDS.some((f) => form.data?.[f]?.requiresRestart);

  return (
    <div className="flex flex-col divide-y divide-[var(--border)]">
      {needsRestart && (
        <p className="pb-3 text-xs text-[var(--text-muted)]" data-testid="advanced-restart-hint">
          {t("admin.transcoding.advancedRestartHint")}
        </p>
      )}
      {FIELDS.map((f) => form.renderField(f))}
      <div className="flex justify-end pt-3">
        <Button data-testid="advanced-transcoding-save-btn" loading={form.saving} disabled={!form.dirty} onClick={() => void form.submit()}>
          {t("common.save")}
        </Button>
      </div>
    </div>
  );
}
