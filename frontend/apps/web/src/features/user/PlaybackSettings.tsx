import { useEffect, useMemo, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceUpdateProfile, getUserServiceListProfilesQueryKey } from "@evey/api";
import { Select } from "@/components/ui/Select";
import { DATA_SAVER_LABEL_KEY, DATA_SAVER_LEVELS, dataSaverLabelValues, useDataSaverStore, type DataSaverLevel } from "@evey/store";
import { useActiveProfile } from "@/hooks/useActiveProfile";

// Per-profile playback preferences. The server reads these off the ACTIVE
// profile when it decides how to serve a title (see the transcoding decision),
// so they apply to every client, not only the one that wrote them. Mobile and
// TV have carried this screen for a while; the web had none, which left the
// same account with different playback rules depending on where it was set.
//
// The cellular cap that mobile also offers is deliberately absent: a browser
// has no notion of being on mobile data. The data saver at the top is the one
// setting that is NOT on the profile: it belongs to this browser alone.

const BITRATE_VALUES = ["0", "2000", "4000", "8000", "10000", "15000", "20000", "40000"] as const;
const VIDEO_CODECS = ["auto", "h264", "hevc", "av1"] as const;
const AUDIO_CODECS = ["auto", "aac", "ac3", "eac3", "opus"] as const;

const CODEC_LABEL: Record<string, string> = {
  h264: "H.264",
  hevc: "HEVC (H.265)",
  av1: "AV1",
  aac: "AAC",
  ac3: "AC3",
  eac3: "E-AC3",
  opus: "Opus",
};

export function PlaybackSettings() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const { profile: activeProfile, userId } = useActiveProfile();
  const updateProfile = useUserServiceUpdateProfile();
  const dataSaver = useDataSaverStore((s) => s.level);
  const setDataSaver = useDataSaverStore((s) => s.setLevel);

  // Server-truth preferences mirrored in a ref: UpdateProfile REPLACES the
  // whole map, so two quick edits must merge onto each other instead of both
  // spreading the same stale render snapshot (same guard as LanguageSettings).
  const prefsRef = useRef<Record<string, string>>({});
  useEffect(() => {
    prefsRef.current = { ...activeProfile?.preferences };
  }, [activeProfile?.preferences]);

  const prefs = activeProfile?.preferences ?? {};

  async function save(patch: Record<string, string>) {
    if (!activeProfile?.id) return;
    const next = { ...prefsRef.current, ...patch };
    prefsRef.current = next;
    try {
      await updateProfile.mutateAsync({ id: activeProfile.id, data: { preferences: next } });
      if (userId) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
    } catch (err) { console.warn("playback preference update failed", err); }
  }

  const bitrateOptions = useMemo(
    () => BITRATE_VALUES.map((v) => ({
      value: v,
      label: v === "0" ? t("auth.settings.playback.bitrateUnlimited") : `${Number(v) / 1000} Mbps`,
    })),
    [t],
  );
  const codecOptions = (values: readonly string[]) =>
    values.map((v) => ({ value: v, label: v === "auto" ? t("auth.settings.playback.codecAuto") : CODEC_LABEL[v] ?? v }));
  const channelOptions = [
    { value: "auto", label: t("auth.settings.playback.codecAuto") },
    { value: "2", label: t("auth.settings.playback.channelsStereo") },
    { value: "6", label: t("auth.settings.playback.channels51") },
    { value: "8", label: t("auth.settings.playback.channels71") },
  ];
  const burnOptions = [
    { value: "auto", label: t("auth.settings.playback.subtitleBurnAuto") },
    { value: "always", label: t("auth.settings.playback.subtitleBurnAlways") },
    { value: "never", label: t("auth.settings.playback.subtitleBurnNever") },
  ];

  const dataSaverOptions = DATA_SAVER_LEVELS.map((level) => ({
    value: level,
    label: t(DATA_SAVER_LABEL_KEY[level], dataSaverLabelValues(level, i18n.language)),
  }));

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-1">
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.playback.dataSaverTitle")}</h3>
        <p className="text-xs text-[var(--text-muted)]">{t("auth.settings.playback.dataSaverDesc")}</p>
        <Select
          label={t("auth.settings.playback.dataSaverTitle")}
          options={dataSaverOptions}
          value={dataSaver}
          onChange={(v) => setDataSaver(v as DataSaverLevel)}
        />
      </div>

      <div className="flex flex-col gap-1">
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.playback.bitrateTitle")}</h3>
        <p className="text-xs text-[var(--text-muted)]">{t("auth.settings.playback.bitrateDesc")}</p>
        <Select
          label={t("auth.settings.playback.bitrateTitle")}
          options={bitrateOptions}
          value={prefs.max_bitrate_kbps ?? "0"}
          onChange={(v) => void save({ max_bitrate_kbps: v })}
          disabled={!activeProfile}
        />
      </div>

      <Select
        label={t("auth.settings.playback.preferredVideoCodec")}
        options={codecOptions(VIDEO_CODECS)}
        value={prefs.preferred_video_codec ?? "auto"}
        onChange={(v) => void save({ preferred_video_codec: v })}
        disabled={!activeProfile}
      />

      <Select
        label={t("auth.settings.playback.preferredAudioCodec")}
        options={codecOptions(AUDIO_CODECS)}
        value={prefs.preferred_audio_codec ?? "auto"}
        onChange={(v) => void save({ preferred_audio_codec: v })}
        disabled={!activeProfile}
      />

      <Select
        label={t("auth.settings.playback.maxAudioChannels")}
        options={channelOptions}
        value={prefs.max_audio_channels ?? "auto"}
        onChange={(v) => void save({ max_audio_channels: v })}
        disabled={!activeProfile}
      />

      <div className="flex flex-col gap-1">
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.playback.subtitleBurnIn")}</h3>
        <p className="text-xs text-[var(--text-muted)]">{t("auth.settings.playback.subtitleBurnInDesc")}</p>
        <Select
          label={t("auth.settings.playback.subtitleBurnIn")}
          options={burnOptions}
          value={prefs.subtitle_burn_in ?? "auto"}
          onChange={(v) => void save({ subtitle_burn_in: v })}
          disabled={!activeProfile}
        />
      </div>
    </div>
  );
}
