import { useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  formatDurationMs,
  getUserServiceListProfilesQueryKey,
  useActivityServiceGetUserStats,
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  useUserServiceUpdateProfile,
  type V1Profile,
} from "@evey/api";
import { useAuthStore, useServerListStore } from "@evey/store";
import {
  AUDIO_CHANNELS_OPTIONS,
  AUDIO_CODEC_OPTIONS,
  BITRATE_OPTIONS,
  LANG_OPTIONS,
  SUBTITLE_BURN_OPTIONS,
  TOGGLE_OPTIONS,
  VIDEO_CODEC_OPTIONS,
  resolveLangPref,
  type PrefMap,
} from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button, Spinner } from "../components/ui";
import { navigate, replace } from "../app/router";
import { signOut } from "../lib/session";

interface Option<T> { value: T; labelKey: string; fallback: string }

function Choice<T extends string | boolean>({ title, options, value, onPick, autoFocus }: {
  title: string;
  options: readonly Option<T>[];
  value: T;
  onPick: (v: T) => void;
  autoFocus?: boolean;
}) {
  const { t } = useTranslation();
  return (
    <>
      <h2 className="row-title">{title}</h2>
      <FocusRoot className="chips">
        {options.map((o, i) => (
          <Button
            key={String(o.value)}
            autoFocus={autoFocus && i === 0}
            variant={value === o.value ? "primary" : "default"}
            onPress={() => onPick(o.value)}
          >
            {o.labelKey ? t(o.labelKey, o.fallback) : o.fallback}
          </Button>
        ))}
      </FocusRoot>
    </>
  );
}

/**
 * The playback preferences, written onto the ACTIVE profile: the player reads
 * its languages and its codec choices there, so a preference saved on the
 * household's default profile would look saved and change nothing.
 */
export function PlaybackSettingsScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp, isLoading } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const activeProfileId = useAuthStore((s) => s.profileId);
  const profile: V1Profile | undefined =
    profilesResp?.profiles?.find((p) => p.id === activeProfileId)
    ?? profilesResp?.profiles?.find((p) => p.isDefault)
    ?? profilesResp?.profiles?.[0];

  const [prefs, setPrefs] = useState<PrefMap>({});
  useEffect(() => { setPrefs((profile?.preferences ?? {}) as PrefMap); }, [profile]);

  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onSuccess: () => { if (user?.id) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) }); },
    },
  });

  // The value lands on the screen only once the server has taken it, so a
  // failed save never leaves a choice marked that the profile does not hold.
  const save = async (patch: PrefMap) => {
    if (!profile?.id) return;
    try {
      await updateProfile.mutateAsync({ id: profile.id, data: { preferences: { ...profile.preferences, ...patch } } });
      setPrefs((p) => ({ ...p, ...patch }));
    } catch { /* the global net already surfaced it */ }
  };
  const bool = (key: string, dflt = true) => (prefs[key] === undefined ? dflt : prefs[key] !== "false");

  if (isLoading) return <div className="screen screen-center"><Spinner /></div>;

  return (
    <div className="screen">
      <h1 className="title">{t("auth.settings.tabs.playback")}</h1>
      <Choice
        autoFocus
        title={t("auth.settings.playback.audioLanguage")}
        options={LANG_OPTIONS}
        value={resolveLangPref(prefs, "audio_languages", "default_audio_language")}
        onPick={(v) => void save({ audio_languages: v })}
      />
      <Choice
        title={t("auth.settings.playback.subtitleLanguage")}
        options={LANG_OPTIONS}
        value={resolveLangPref(prefs, "subtitle_languages", "default_subtitle_language")}
        onPick={(v) => void save({ subtitle_languages: v })}
      />
      <Choice
        title={t("auth.settings.playback.bitrateTitle")}
        options={BITRATE_OPTIONS}
        value={prefs.max_bitrate_kbps ?? "0"}
        onPick={(v) => void save({ max_bitrate_kbps: v })}
      />
      <Choice
        title={t("auth.settings.playback.preferredVideoCodec")}
        options={VIDEO_CODEC_OPTIONS}
        value={prefs.preferred_video_codec ?? "auto"}
        onPick={(v) => void save({ preferred_video_codec: v })}
      />
      <Choice
        title={t("auth.settings.playback.preferredAudioCodec")}
        options={AUDIO_CODEC_OPTIONS}
        value={prefs.preferred_audio_codec ?? "auto"}
        onPick={(v) => void save({ preferred_audio_codec: v })}
      />
      <Choice
        title={t("auth.settings.playback.maxAudioChannels")}
        options={AUDIO_CHANNELS_OPTIONS}
        value={prefs.max_audio_channels ?? "auto"}
        onPick={(v) => void save({ max_audio_channels: v })}
      />
      <Choice
        title={t("auth.settings.playback.subtitleBurnIn")}
        options={SUBTITLE_BURN_OPTIONS}
        value={prefs.subtitle_burn_in ?? "auto"}
        onPick={(v) => void save({ subtitle_burn_in: v })}
      />
      <Choice
        title={t("auth.settings.playback.autoplayNextTitle")}
        options={TOGGLE_OPTIONS}
        value={bool("autoplay_next_episode")}
        onPick={(v) => void save({ autoplay_next_episode: v ? "true" : "false" })}
      />
      <Choice
        title={t("auth.settings.playback.allowDirectPlay")}
        options={TOGGLE_OPTIONS}
        value={bool("allow_direct_play")}
        onPick={(v) => void save({ allow_direct_play: v ? "true" : "false" })}
      />
      <Choice
        title={t("auth.settings.playback.allowDirectStream")}
        options={TOGGLE_OPTIONS}
        value={bool("allow_direct_stream")}
        onPick={(v) => void save({ allow_direct_stream: v ? "true" : "false" })}
      />
      <Choice
        title={t("auth.settings.playback.allowTranscoding")}
        options={TOGGLE_OPTIONS}
        value={bool("allow_transcoding")}
        onPick={(v) => void save({ allow_transcoding: v ? "true" : "false" })}
      />
    </div>
  );
}

/** What this viewer has watched, the same figures the television shows. */
export function StatsScreen() {
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId);
  const { data, isLoading } = useActivityServiceGetUserStats(profileId ?? "", undefined, { query: { enabled: !!profileId } });
  const genres = useMemo(
    () => [...(data?.genres ?? [])].sort((a, b) => parseInt(b.totalTimeMs ?? "0", 10) - parseInt(a.totalTimeMs ?? "0", 10)),
    [data?.genres],
  );
  if (isLoading) return <div className="screen screen-center"><Spinner /></div>;
  if (!data || (data.totalPlays ?? 0) === 0) {
    return (
      <FocusRoot autoFocus className="screen screen-center">
        <p className="muted">{t("common.userStats.noData")}</p>
        <Button onPress={() => navigate({ name: "settings" })}>{t("common.back")}</Button>
      </FocusRoot>
    );
  }
  const top = parseInt(genres[0]?.totalTimeMs ?? "0", 10) || 1;
  return (
    <FocusRoot autoFocus className="screen">
      <h1 className="title">{t("common.userStats.title")}</h1>
      <div className="stat-grid">
        <div className="stat"><span className="muted">{t("common.userStats.totalWatchTime")}</span><strong>{formatDurationMs(data.totalWatchTimeMs)}</strong></div>
        <div className="stat"><span className="muted">{t("common.userStats.totalPlays")}</span><strong>{data.totalPlays ?? 0}</strong></div>
        <div className="stat"><span className="muted">{t("common.userStats.uniqueItems")}</span><strong>{data.uniqueItemsWatched ?? 0}</strong></div>
        <div className="stat"><span className="muted">{t("common.userStats.favoriteGenre")}</span><strong>{genres[0]?.genre ?? "-"}</strong></div>
      </div>
      {genres.length > 0 ? (
        <>
          <h2 className="row-title">{t("common.userStats.genreBreakdown")}</h2>
          {genres.slice(0, 8).map((g) => (
            <div className="genre-row" key={g.genre}>
              <span className="genre-name">{g.genre}</span>
              <span className="genre-track"><span className="genre-fill" style={{ width: `${Math.round((parseInt(g.totalTimeMs ?? "0", 10) / top) * 100)}%` }} /></span>
              <span className="muted">{formatDurationMs(g.totalTimeMs)}</span>
            </div>
          ))}
        </>
      ) : null}
      <div className="actions">
        <Button onPress={() => navigate({ name: "settings" })}>{t("common.back")}</Button>
      </div>
    </FocusRoot>
  );
}

/** The servers this television knows: pick one, forget one. */
export function ServersScreen() {
  const { t } = useTranslation();
  const servers = Object.values(useServerListStore((s) => s.servers));
  const active = useServerListStore((s) => s.getActiveServer());
  const setActive = useServerListStore((s) => s.setActiveServer);
  const removeServer = useServerListStore((s) => s.removeServer);
  const [confirming, setConfirming] = useState<string | null>(null);

  return (
    <FocusRoot autoFocus className="screen">
      <h1 className="title">{t("mobile.servers.manageTitle")}</h1>
      <div className="track-list">
        {servers.map((s, i) => (
          <div className="feed-row" key={s.id}>
            <Button
              autoFocus={i === 0}
              variant={s.id === active?.id ? "primary" : "default"}
              onPress={() => { setActive(s.id); void signOut().then(() => replace({ name: "login" })); }}
            >
              <span className="track-title">{s.label || s.serverName || s.url}</span>
              <span className="muted">{s.url}</span>
            </Button>
            <Button
              disabled={s.id === active?.id}
              onPress={() => { if (confirming === s.id) { removeServer(s.id); setConfirming(null); } else setConfirming(s.id); }}
            >
              {confirming === s.id ? t("mobile.servers.deleteConfirmTitle") : t("mobile.servers.delete")}
            </Button>
          </div>
        ))}
      </div>
      <div className="actions">
        <Button onPress={() => { void signOut().then(() => replace({ name: "server" })); }}>{t("mobile.servers.addAnother")}</Button>
      </div>
    </FocusRoot>
  );
}
