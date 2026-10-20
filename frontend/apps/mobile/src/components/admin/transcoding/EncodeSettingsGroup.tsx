import { useEffect, useState } from "react";
import { ActivityIndicator, Switch, Text, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import { SERVER_SETTINGS_KEY, SERVER_SETTINGS_URL, useServerSettings } from "@evey/client";
import { useTheme } from "../../../providers/ThemeProvider";
import { SettingRow } from "../settings/SettingRow";
import { SettingsGroup } from "../settings/SettingsGroup";
import { OptionGroup } from "../settings/OptionGroup";
import { CALIBRATION_KEY, useCalibration } from "./calibration";

const mbps = (kbps: string) => `${Number(kbps) / 1000} Mb/s`;

// EncodeSettingsGroup holds the encoding SETTINGS (the web's
// EncodeSettingsCard): hardware acceleration and its backend, the bitrate
// ceilings, AV1 output and the low-power encoder. Each applies at once, no
// Save. What the host measured lives in the diagnostics part of the screen.
export function EncodeSettingsGroup() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: calib } = useCalibration();
  const { data: settings } = useServerSettings();

  const av1Enabled = Boolean(settings?.av1Enabled?.value ?? false);
  const av1MaxHeight = Number(settings?.av1MaxHeight?.value ?? 0);
  const lowPowerEnabled = Boolean(settings?.hwLowPowerEnabled?.value ?? false);
  const maxBitrate = Number(settings?.maxVideoBitrateKbps?.value ?? 0);
  // Absent = follow the LAN (0), the backend default: never fall back to a
  // number a save would then persist.
  const maxBitrateRemote = Number(settings?.maxVideoBitrateRemoteKbps?.value ?? 0);

  // Local mirror of the active mode so the picker answers at once while the
  // detached re-bench runs server-side (the query refetches a few seconds later).
  const [hwMode, setHwModeLocal] = useState("auto");
  useEffect(() => {
    if (calib?.hw_mode) setHwModeLocal(calib.hw_mode);
  }, [calib?.hw_mode]);

  const put = (data: Record<string, unknown>) => eveyFetch({ url: SERVER_SETTINGS_URL, method: "PUT", data });
  const onError = (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error")));

  const setMode = useMutation({
    mutationFn: (mode: string) => put({ hwMode: mode }),
    onSuccess: (_r, mode) => {
      setHwModeLocal(mode);
      toast.success(t("admin.calibration.modeApplied", "Mode applied - recalibrating…"));
      void qc.invalidateQueries({ queryKey: CALIBRATION_KEY });
      void qc.invalidateQueries({ queryKey: SERVER_SETTINGS_KEY });
      [3000, 9000, 20000].forEach((ms) => setTimeout(() => void qc.invalidateQueries({ queryKey: CALIBRATION_KEY }), ms));
    },
    onError,
  });
  const setAV1 = useMutation({
    mutationFn: (patch: { av1Enabled?: boolean; av1MaxHeight?: number }) => put(patch),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: SERVER_SETTINGS_KEY });
      toast.success(t("admin.calibration.av1Applied", "AV1 setting applied"));
    },
    onError,
  });
  const setLowPower = useMutation({
    mutationFn: (enabled: boolean) => put({ hwLowPowerEnabled: enabled }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: SERVER_SETTINGS_KEY });
      toast.success(t("admin.calibration.lowPowerApplied", "Low-power setting applied"));
    },
    onError,
  });
  const setBitrate = useMutation({
    mutationFn: (patch: { maxVideoBitrateKbps?: number; maxVideoBitrateRemoteKbps?: number }) => put(patch),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: SERVER_SETTINGS_KEY });
      toast.success(t("admin.calibration.maxBitrateApplied", "Bitrate ceiling applied"));
    },
    onError,
  });

  const title = t("admin.settingsLayout.groupEncoding");
  const description = t("admin.settingsLayout.groupEncodingDesc");
  if (!calib || !settings) {
    return (
      <SettingsGroup title={title} description={description} testID="group-encoding">
        <View style={{ paddingVertical: 16, alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} />
        </View>
      </SettingsGroup>
    );
  }

  const detected = calib.detected_backends ?? [];
  const isWsl = calib.is_wsl ?? false;
  const hwEnabled = hwMode !== "off";
  // Forceable = detected backends plus the current pin, so a forced mode is
  // never silently dropped. VAAPI/QSV cannot sustain the encode under WSL.
  const forceable = Array.from(new Set([...detected, ["nvenc", "vaapi", "qsv", "rkmpp"].includes(hwMode) ? hwMode : ""].filter(Boolean)));
  const wslBlocked = (b: string) => isWsl && (b === "vaapi" || b === "qsv");
  const lowPowerCodecs = (calib.hw_features ?? []).filter((f) => f.startsWith("low_power_")).map((f) => f.slice("low_power_".length));
  const track = { false: theme.colors.border, true: theme.colors.accent };

  const hwLabel = t("admin.calibration.hwEnableLabel", "Hardware acceleration");
  const backendLabel = t("admin.calibration.hwBackendLabel", "Backend");
  const bitrateLabel = t("admin.calibration.maxBitrateLabel", "Video bitrate ceiling");
  const remoteLabel = t("admin.calibration.maxBitrateRemoteLabel", "Video bitrate ceiling, remote viewers");
  const av1Label = t("admin.calibration.av1Label", "AV1 output");
  const av1MaxLabel = t("admin.calibration.av1MaxLabel", "AV1 max quality");
  const lowPowerLabel = t("admin.calibration.lowPowerLabel", "Low-power encoder");

  return (
    <SettingsGroup title={title} description={description} testID="group-encoding">
      <SettingRow
        label={hwLabel}
        info={t("admin.settingsHelp.hwAccel")}
        hint={hwEnabled ? t("admin.calibration.hwEnabledHint", "Encoding on the GPU when possible") : t("admin.calibration.hwDisabledHint", "Software only (CPU)")}
        kind="toggle"
        testID="setting-row-hw-enable"
      >
        <Switch
          value={hwEnabled}
          onValueChange={() => setMode.mutate(hwEnabled ? "off" : "auto")}
          disabled={setMode.isPending}
          accessibilityLabel={hwLabel}
          testID="hw-enable-toggle"
          trackColor={track}
        />
      </SettingRow>

      {hwEnabled ? (
        <SettingRow
          label={backendLabel}
          info={t("admin.settingsHelp.hwBackend")}
          hint={
            <>
              {detected.length > 0
                ? t("admin.calibration.detected", "Detected: {{list}}", { list: detected.join(", ") })
                : t("admin.calibration.detectedNone", "No GPU accelerator detected")}
              {isWsl ? (
                <Text style={{ color: theme.colors.warning }}>
                  {"\n"}
                  {t("admin.calibration.wslVaapiNote")}
                </Text>
              ) : null}
            </>
          }
          kind="wide"
          testID="setting-row-hw-backend"
        >
          <OptionGroup
            label={backendLabel}
            value={hwMode}
            disabled={setMode.isPending}
            onChange={(v) => setMode.mutate(v)}
            options={[
              { value: "auto", label: t("admin.calibration.hwModeAuto", "Auto (best available)") },
              ...forceable.map((b) => ({
                value: b,
                label: wslBlocked(b) ? `${b.toUpperCase()} - ${t("admin.calibration.wslUnsupported", "not supported on WSL")}` : b.toUpperCase(),
                disabled: wslBlocked(b),
              })),
            ]}
            testID="hw-mode-select"
          />
        </SettingRow>
      ) : null}

      <SettingRow label={bitrateLabel} info={t("admin.calibration.maxBitrateHint")} kind="wide" testID="setting-row-max-bitrate">
        <OptionGroup
          label={bitrateLabel}
          value={String(maxBitrate)}
          disabled={setBitrate.isPending}
          onChange={(v) => setBitrate.mutate({ maxVideoBitrateKbps: parseInt(v, 10) })}
          options={[
            { value: "0", label: t("admin.calibration.maxBitrateSource", "Follow the source") },
            ...[20000, 40000, 60000, 80000].map((k) => ({ value: String(k), label: mbps(String(k)) })),
          ]}
          formatUnknown={mbps}
          testID="max-video-bitrate-select"
        />
      </SettingRow>

      <SettingRow label={remoteLabel} info={t("admin.calibration.maxBitrateRemoteHint")} kind="wide" testID="setting-row-max-bitrate-remote">
        <OptionGroup
          label={remoteLabel}
          value={String(maxBitrateRemote)}
          disabled={setBitrate.isPending}
          onChange={(v) => setBitrate.mutate({ maxVideoBitrateRemoteKbps: parseInt(v, 10) })}
          options={[
            { value: "0", label: t("admin.calibration.maxBitrateRemoteNone", "Follow the LAN (no extra remote cap)") },
            ...[8000, 12000, 20000, 40000].map((k) => ({ value: String(k), label: mbps(String(k)) })),
          ]}
          formatUnknown={mbps}
          testID="max-video-bitrate-remote-select"
        />
      </SettingRow>

      {/* AV1 is hardware-only: shown with the hardware on. */}
      {hwEnabled ? (
        <SettingRow
          label={av1Label}
          info={t("admin.calibration.av1Hint")}
          hint={(calib.hw_codecs ?? []).includes("av1") ? undefined : <Text style={{ color: theme.colors.warning }}>{t("admin.calibration.av1NoHw")}</Text>}
          kind="toggle"
          testID="setting-row-av1"
        >
          <Switch
            value={av1Enabled}
            onValueChange={() => setAV1.mutate({ av1Enabled: !av1Enabled })}
            disabled={setAV1.isPending}
            accessibilityLabel={av1Label}
            testID="av1-enable-toggle"
            trackColor={track}
          />
        </SettingRow>
      ) : null}

      {hwEnabled && av1Enabled ? (
        <SettingRow label={av1MaxLabel} info={t("admin.settingsHelp.av1MaxHeight")} kind="wide" testID="setting-row-av1-max">
          <OptionGroup
            label={av1MaxLabel}
            value={String(av1MaxHeight)}
            disabled={setAV1.isPending}
            onChange={(v) => setAV1.mutate({ av1MaxHeight: parseInt(v, 10) })}
            options={[
              { value: "0", label: t("admin.calibration.av1MaxAuto", "Auto (up to source)") },
              ...["720", "1080", "1440", "2160"].map((h) => ({ value: h, label: `${h}p` })),
            ]}
            formatUnknown={(v) => `${v}p`}
            testID="av1-max-height-select"
          />
        </SettingRow>
      ) : null}

      {hwEnabled && lowPowerCodecs.length > 0 ? (
        <SettingRow label={lowPowerLabel} info={t("admin.calibration.lowPowerHint")} hint={`(${lowPowerCodecs.join(", ")})`} kind="toggle" testID="setting-row-low-power">
          <Switch
            value={lowPowerEnabled}
            onValueChange={() => setLowPower.mutate(!lowPowerEnabled)}
            disabled={setLowPower.isPending}
            accessibilityLabel={lowPowerLabel}
            testID="low-power-toggle"
            trackColor={track}
          />
        </SettingRow>
      ) : null}
    </SettingsGroup>
  );
}
