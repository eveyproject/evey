import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Switch } from "@/components/ui/Switch";
import { Spinner } from "@/components/ui/Spinner";
import { SETTINGS_KEY, useTranscodingServerSettings, type ServerSettings } from "./hooks";

// AdvancedTranscodingCard exposes the five knobs that were reachable only
// through environment variables until 2026-07-30 - which meant editing a
// compose file and recreating the container to change a playback ceiling. The
// 720p-HDR ceiling in particular was diagnosed and fixed that way in production.
//
// Every one of them is snapshotted at boot, so the server reports
// requiresRestart on each and this card says so ONCE, driven by the server's
// flag rather than a hardcoded string: a knob that becomes live later stops
// showing the warning without anyone remembering to edit this file.
export function AdvancedTranscodingCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading } = useTranscodingServerSettings();

  const [maxSessions, setMaxSessions] = useState("");
  const [swMaxHeight, setSwMaxHeight] = useState("");
  const [swHDRMaxHeight, setSwHDRMaxHeight] = useState("");
  const [singleVariant, setSingleVariant] = useState(false);
  const [hwPerVariant, setHWPerVariant] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (!data) return;
    // Blank means "no override": pre-filling the effective value would turn
    // every save into an override of a value the operator never chose.
    setMaxSessions(data.transcodeMaxSessions?.source === "override" ? String(data.transcodeMaxSessions.value) : "");
    setSwMaxHeight(data.softwareMaxHeight?.source === "override" ? String(data.softwareMaxHeight.value) : "");
    setSwHDRMaxHeight(data.softwareHDRMaxHeight?.source === "override" ? String(data.softwareHDRMaxHeight.value) : "");
    setSingleVariant(data.transcodeSingleVariant?.value ?? false);
    setHWPerVariant(data.transcodeHWPerVariant?.value ?? false);
  }, [data]);

  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: body }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: SETTINGS_KEY }); toast.success(t("admin.transcoding.savedRestart", "Saved - restart the server to apply")); setDirty(false); },
    onError: (err: Error) => toast.error(err.message),
  });

  if (isLoading || !data) return <div className="flex justify-center py-6"><Spinner size="md" /></div>;

  const needsRestart = [
    data.transcodeMaxSessions, data.transcodeSingleVariant, data.transcodeHWPerVariant,
    data.softwareMaxHeight, data.softwareHDRMaxHeight,
  ].some((s) => s?.requiresRestart);

  function handleSave() {
    // An empty field clears the override (explicit null, the documented Reset
    // path). That is NOT the same as saving 0, which is a meaningful value here.
    save.mutate({
      transcodeMaxSessions: maxSessions === "" ? null : parseInt(maxSessions, 10),
      softwareMaxHeight: swMaxHeight === "" ? null : parseInt(swMaxHeight, 10),
      softwareHDRMaxHeight: swHDRMaxHeight === "" ? null : parseInt(swHDRMaxHeight, 10),
      transcodeSingleVariant: singleVariant,
      transcodeHWPerVariant: hwPerVariant,
    });
  }

  return (
    <div className="flex flex-col gap-4">
      {needsRestart && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="advanced-restart-hint">
          {t("admin.transcoding.advancedRestartHint", "These are read when the server starts: saving stores your choice, restarting applies it.")}
        </p>
      )}

      <Input
        label={t("admin.transcoding.maxSessions", "Max concurrent HLS sessions (blank = no limit)")}
        type="number"
        min={0}
        placeholder={String(data.transcodeMaxSessions?.default ?? 0)}
        value={maxSessions}
        onChange={(e) => { setMaxSessions(e.target.value); setDirty(true); }}
        data-testid="max-sessions-input"
      />
      <Input
        label={t("admin.transcoding.softwareMaxHeight", "Software transcode height ceiling (blank = no cap)")}
        type="number"
        min={0}
        placeholder={String(data.softwareMaxHeight?.default ?? 0)}
        value={swMaxHeight}
        onChange={(e) => { setSwMaxHeight(e.target.value); setDirty(true); }}
        data-testid="software-max-height-input"
      />
      <Input
        label={t("admin.transcoding.softwareHDRMaxHeight", "Software HDR height ceiling (blank = no cap)")}
        type="number"
        min={0}
        placeholder={String(data.softwareHDRMaxHeight?.default ?? 0)}
        value={swHDRMaxHeight}
        onChange={(e) => { setSwHDRMaxHeight(e.target.value); setDirty(true); }}
        data-testid="software-hdr-max-height-input"
      />

      <div className="flex items-center gap-3">
        <label className="text-sm font-medium text-[var(--text)]">
          {t("admin.transcoding.singleVariant", "Encode one ladder rung at a time")}
        </label>
        <Switch
          checked={singleVariant}
          onChange={() => { setSingleVariant((v) => !v); setDirty(true); }}
          label={t("admin.transcoding.singleVariant", "Encode one ladder rung at a time")}
          data-testid="single-variant-toggle"
        />
      </div>

      <div className="flex items-center gap-3">
        <label className="text-sm font-medium text-[var(--text)]">
          {t("admin.transcoding.hwPerVariant", "One hardware encoder process per rung")}
        </label>
        <Switch
          checked={hwPerVariant}
          onChange={() => { setHWPerVariant((v) => !v); setDirty(true); }}
          label={t("admin.transcoding.hwPerVariant", "One hardware encoder process per rung")}
          data-testid="hw-per-variant-toggle"
        />
      </div>

      <div className="flex justify-end">
        <Button data-testid="advanced-transcoding-save-btn" loading={save.isPending} disabled={!dirty} onClick={handleSave}>
          {t("common.save")}
        </Button>
      </div>
    </div>
  );
}
