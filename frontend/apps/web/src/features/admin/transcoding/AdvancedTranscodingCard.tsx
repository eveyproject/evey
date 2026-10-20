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
import { SegmentedControl } from "@/components/ui/SegmentedControl";
import { SETTINGS_KEY, useTranscodingServerSettings, type ServerSettings, type SettingValue } from "./hooks";

// The AV1 software ceilings the server accepts (settings_handler.go
// validSoftwareAV1Heights). 0 is "Auto": AV1 stays hardware-only unless the
// calibration found software AV1 viable.
const SOFTWARE_AV1_HEIGHTS = ["0", "480", "720", "1080", "1440"] as const;

type Field = "transcodeMaxSessions" | "softwareMaxHeight" | "softwareHDRMaxHeight" | "softwareAV1MaxHeight" | "transcodeSingleVariant" | "transcodeHWPerVariant";

// AdvancedTranscodingCard exposes the knobs that were reachable only through
// environment variables until 2026-07-30 - which meant editing a compose file
// and recreating the container to change a playback ceiling. The 720p-HDR
// ceiling in particular was diagnosed and fixed that way in production - plus
// the AV1 software ceiling, which the server always accepted but no screen
// offered.
//
// Most of them are snapshotted at boot, so the server reports requiresRestart
// on each and every such row carries the restart badge, driven by the
// server's flag rather than a hardcoded list: a knob that becomes live later
// loses its badge without anyone remembering to edit this file.
export function AdvancedTranscodingCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading } = useTranscodingServerSettings();

  const [maxSessions, setMaxSessions] = useState("");
  const [swMaxHeight, setSwMaxHeight] = useState("");
  const [swHDRMaxHeight, setSwHDRMaxHeight] = useState("");
  const [swAV1MaxHeight, setSwAV1MaxHeight] = useState("0");
  const [singleVariant, setSingleVariant] = useState(false);
  const [hwPerVariant, setHWPerVariant] = useState(false);
  const [dirty, setDirty] = useState<Set<Field>>(new Set());

  useEffect(() => {
    if (!data) return;
    // Blank means "no override": pre-filling the effective value would turn
    // every save into an override of a value the operator never chose.
    setMaxSessions(data.transcodeMaxSessions?.source === "override" ? String(data.transcodeMaxSessions.value) : "");
    setSwMaxHeight(data.softwareMaxHeight?.source === "override" ? String(data.softwareMaxHeight.value) : "");
    setSwHDRMaxHeight(data.softwareHDRMaxHeight?.source === "override" ? String(data.softwareHDRMaxHeight.value) : "");
    setSwAV1MaxHeight(String(data.softwareAV1MaxHeight?.value ?? 0));
    setSingleVariant(data.transcodeSingleVariant?.value ?? false);
    setHWPerVariant(data.transcodeHWPerVariant?.value ?? false);
  }, [data]);

  const touch = (f: Field) => setDirty((d) => new Set(d).add(f));
  const restartOf = (f: Field) => (data?.[f] as SettingValue<unknown> | undefined)?.requiresRestart === true;
  const needsRestartAfterSave = [...dirty].some(restartOf);

  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: body }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: SETTINGS_KEY });
      toast.success(
        needsRestartAfterSave
          ? t("admin.transcoding.savedRestart", "Saved - restart the server to apply")
          : t("admin.transcoding.saved", "Saved"),
      );
      setDirty(new Set());
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  if (isLoading || !data) return <div className="flex justify-center py-6"><Spinner size="md" /></div>;

  const needsRestart = (["transcodeMaxSessions", "transcodeSingleVariant", "transcodeHWPerVariant", "softwareMaxHeight", "softwareHDRMaxHeight"] as Field[]).some(restartOf);

  function handleSave() {
    // An empty field clears the override (explicit null, the documented Reset
    // path). That is NOT the same as saving 0, which is a meaningful value here.
    // The AV1 ceiling's "Auto" is the default, so it clears the override too.
    save.mutate({
      transcodeMaxSessions: maxSessions === "" ? null : parseInt(maxSessions, 10),
      softwareMaxHeight: swMaxHeight === "" ? null : parseInt(swMaxHeight, 10),
      softwareHDRMaxHeight: swHDRMaxHeight === "" ? null : parseInt(swHDRMaxHeight, 10),
      ...(dirty.has("softwareAV1MaxHeight")
        ? { softwareAV1MaxHeight: swAV1MaxHeight === "0" ? null : parseInt(swAV1MaxHeight, 10) }
        : {}),
      transcodeSingleVariant: singleVariant,
      transcodeHWPerVariant: hwPerVariant,
    });
  }

  const singleLabel = t("admin.transcoding.singleVariant", "Encode one ladder rung at a time");
  const perVariantLabel = t("admin.transcoding.hwPerVariant", "One hardware encoder process per rung");
  const av1Label = t("admin.serverSettings.softwareAV1MaxHeight");

  return (
    <div className="flex flex-col divide-y divide-[var(--border)]">
      {needsRestart && (
        <p className="pb-3 text-xs text-[var(--text-muted)]" data-testid="advanced-restart-hint">
          {t("admin.transcoding.advancedRestartHint", "These are read when the server starts: saving stores your choice, restarting applies it.")}
        </p>
      )}

      <SettingRow
        label={t("admin.transcoding.maxSessions", "Max concurrent HLS sessions (blank = no limit)")}
        htmlFor="adv-max-sessions"
        info={t("admin.settingsHelp.transcodeMaxSessions")}
        restart={restartOf("transcodeMaxSessions")}
      >
        <Input
          id="adv-max-sessions"
          className="w-full"
          type="number"
          min={0}
          placeholder={String(data.transcodeMaxSessions?.default ?? 0)}
          value={maxSessions}
          onChange={(e) => { setMaxSessions(e.target.value); touch("transcodeMaxSessions"); }}
          data-testid="max-sessions-input"
        />
      </SettingRow>
      <SettingRow
        label={t("admin.transcoding.softwareMaxHeight", "Software transcode height ceiling (blank = no cap)")}
        htmlFor="adv-sw-max-height"
        info={t("admin.settingsHelp.softwareMaxHeight")}
        restart={restartOf("softwareMaxHeight")}
      >
        <Input
          id="adv-sw-max-height"
          className="w-full"
          type="number"
          min={0}
          placeholder={String(data.softwareMaxHeight?.default ?? 0)}
          value={swMaxHeight}
          onChange={(e) => { setSwMaxHeight(e.target.value); touch("softwareMaxHeight"); }}
          data-testid="software-max-height-input"
        />
      </SettingRow>
      <SettingRow
        label={t("admin.transcoding.softwareHDRMaxHeight", "Software HDR height ceiling (blank = no cap)")}
        htmlFor="adv-sw-hdr-max-height"
        info={t("admin.settingsHelp.softwareHDRMaxHeight")}
        restart={restartOf("softwareHDRMaxHeight")}
      >
        <Input
          id="adv-sw-hdr-max-height"
          className="w-full"
          type="number"
          min={0}
          placeholder={String(data.softwareHDRMaxHeight?.default ?? 0)}
          value={swHDRMaxHeight}
          onChange={(e) => { setSwHDRMaxHeight(e.target.value); touch("softwareHDRMaxHeight"); }}
          data-testid="software-hdr-max-height-input"
        />
      </SettingRow>
      <SettingRow
        label={av1Label}
        info={t("admin.settingsHelp.softwareAV1MaxHeight")}
        restart={restartOf("softwareAV1MaxHeight")}
        kind="wide"
        data-testid="software-av1-row"
      >
        <SegmentedControl
          id="adv-sw-av1-max-height"
          label={av1Label}
          data-testid="software-av1-max-height"
          value={swAV1MaxHeight}
          onChange={(v) => { setSwAV1MaxHeight(v); touch("softwareAV1MaxHeight"); }}
          options={SOFTWARE_AV1_HEIGHTS.map((h) => ({
            value: h,
            label: h === "0" ? t("admin.serverSettings.softwareAV1Auto") : `${h}p`,
          }))}
        />
      </SettingRow>
      <SettingRow label={singleLabel} info={t("admin.settingsHelp.singleVariant")} restart={restartOf("transcodeSingleVariant")} kind="toggle">
        <Switch
          checked={singleVariant}
          onChange={() => { setSingleVariant((v) => !v); touch("transcodeSingleVariant"); }}
          label={singleLabel}
          data-testid="single-variant-toggle"
        />
      </SettingRow>
      <SettingRow label={perVariantLabel} info={t("admin.settingsHelp.hwPerVariant")} restart={restartOf("transcodeHWPerVariant")} kind="toggle">
        <Switch
          checked={hwPerVariant}
          onChange={() => { setHWPerVariant((v) => !v); touch("transcodeHWPerVariant"); }}
          label={perVariantLabel}
          data-testid="hw-per-variant-toggle"
        />
      </SettingRow>

      <div className="flex justify-end pt-3">
        <Button data-testid="advanced-transcoding-save-btn" loading={save.isPending} disabled={dirty.size === 0} onClick={handleSave}>
          {t("common.save")}
        </Button>
      </div>
    </div>
  );
}
