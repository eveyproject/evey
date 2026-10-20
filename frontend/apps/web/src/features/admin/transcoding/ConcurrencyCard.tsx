import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Switch } from "@/components/ui/Switch";
import { Spinner } from "@/components/ui/Spinner";
import { SettingRow } from "@/components/ui/SettingRow";
import { SETTINGS_KEY, useTranscodingServerSettings, type ServerSettings } from "./hooks";

// ConcurrencyCard owns the max-simultaneous-transcodes policy: Auto (no
// override → the server's own default) or a manual cap. A weak NAS that can
// barely sustain one 4K transcode sets a low manual number; most hosts leave
// it on Auto.
export function ConcurrencyCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading } = useTranscodingServerSettings();

  const [auto, setAuto] = useState(true);
  const [manual, setManual] = useState("");
  useEffect(() => {
    if (data) {
      const overridden = data.maxConcurrentTranscodes.source === "override";
      setAuto(!overridden);
      setManual(overridden ? String(data.maxConcurrentTranscodes.value) : "");
    }
  }, [data]);

  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: body }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: SETTINGS_KEY }); toast.success(t("admin.transcoding.saved", "Saved")); },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  if (isLoading || !data) return <div className="flex justify-center py-6"><Spinner size="md" /></div>;

  const defaultVal = data.maxConcurrentTranscodes.default;

  const autoLabel = t("admin.transcoding.concurrencyAuto", "Detect automatically");
  return (
    <div className="flex flex-col divide-y divide-[var(--border)]">
      <SettingRow
        label={t("admin.settingsLayout.backgroundTranscodes")}
        info={t("admin.settingsHelp.backgroundTranscodes")}
        hint={t("admin.transcoding.jobConcurrencySubtitle")}
        kind="toggle"
        data-testid="background-transcodes-row"
      >
        <div className="flex items-center gap-2">
          <span className="text-xs text-[var(--text-muted)]">
            {auto
              ? t("admin.transcoding.concurrencyAutoHint", "Server default ({{n}})", { n: defaultVal })
              : t("admin.transcoding.concurrencyManualHint", "Cap concurrent transcodes")}
          </span>
          <Switch
            checked={auto}
            onChange={() => setAuto((v) => !v)}
            label={autoLabel}
            data-testid="max-concurrent-auto-toggle"
          />
        </div>
      </SettingRow>

      {!auto && (
        <SettingRow label={t("admin.serverSettings.maxConcurrentTranscodes", "Max simultaneous transcodes")} htmlFor="max-concurrent-input">
          <Input
            id="max-concurrent-input"
            className="w-full"
            type="number"
            min={1}
            placeholder={String(defaultVal)}
            value={manual}
            onChange={(e) => setManual(e.target.value)}
            data-testid="max-concurrent-input"
          />
        </SettingRow>
      )}

      <div className="flex justify-end pt-3">
        <Button
          data-testid="concurrency-save-btn"
          loading={save.isPending}
          onClick={() => {
            // Auto → clear the override (server falls back to its default).
            // Manual → persist the chosen cap.
            save.mutate({ maxConcurrentTranscodes: auto || manual === "" ? null : parseInt(manual, 10) });
          }}
        >
          {t("common.save")}
        </Button>
      </div>
    </div>
  );
}
