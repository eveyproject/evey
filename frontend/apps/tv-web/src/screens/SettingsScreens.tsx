import { useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  getUserServiceListProfilesQueryKey,
  useActivityServiceGetUserStats,
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  useUserServiceUpdateProfile,
  type V1Profile,
} from "@evey/api";
import { useAuthStore, useServerListStore, isDemoMode, toast } from "@evey/store";
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
import { Pressable, Spinner } from "../components/ui";
import { ScreenState } from "../components/ScreenState";
import { SettingsGroup, SettingsRow } from "../components/SettingsList";
import { goBack, navigate, replace } from "../app/router";
import { selectServer, signOut } from "../lib/session";
import { SavedServers } from "./ServerConnect";

interface Option<T> { value: T; labelKey: string; fallback: string }

/**
 * The playback preferences, written onto the ACTIVE profile: the player reads
 * its languages and its codec choices there, so a preference saved on the
 * household's default profile would look saved and change nothing.
 */
export function PlaybackSettingsScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const user = useUserServiceGetCurrentUser();
  const profiles = useUserServiceListProfiles(user.data?.id ?? "", { query: { enabled: !!user.data?.id } });
  const activeProfileId = useAuthStore((s) => s.profileId);
  const list = profiles.data?.profiles;
  const profile: V1Profile | undefined = list?.find((p) => p.id === activeProfileId) ?? list?.find((p) => p.isDefault) ?? list?.[0];

  const [prefs, setPrefs] = useState<PrefMap>({});
  useEffect(() => { setPrefs((profile?.preferences ?? {}) as PrefMap); }, [profile]);

  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onSuccess: () => { if (user.data?.id) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.data.id) }); },
    },
  });

  // The value lands on the screen only once the server has taken it, so a
  // failed save never leaves a choice marked that the profile does not hold.
  const save = async (patch: PrefMap) => {
    if (!profile?.id || updateProfile.isPending) return;
    // Demo: these preferences live on the shared profile and only there (the
    // player reads them from it), so a local copy would be a lie. Shown, not
    // changed - said on the press rather than failed by the server.
    if (isDemoMode()) {
      toast.info(t("common.demo.locked"));
      return;
    }
    try {
      await updateProfile.mutateAsync({ id: profile.id, data: { preferences: { ...profile.preferences, ...patch } } });
      setPrefs((p) => ({ ...p, ...patch }));
    } catch { /* the global net already surfaced it */ }
  };
  const bool = (key: string, dflt = true) => (prefs[key] === undefined ? dflt : prefs[key] !== "false");
  const label = (o: Option<unknown>) => (o.labelKey ? t(o.labelKey, o.fallback) : o.fallback);
  const rows = <T extends string | boolean>(options: readonly Option<T>[], value: T, onPick: (v: T) => void, first?: boolean) =>
    options.map((o, i) => (
      <SettingsRow key={String(o.value)} label={label(o)} checked={value === o.value} autoFocus={first && i === 0} onPress={() => onPick(o.value)} />
    ));
  const toggle = (key: string) => (v: boolean) => void save({ [key]: v ? "true" : "false" });

  if (user.isLoading || profiles.isLoading || user.isError || profiles.isError) {
    return (
      <ScreenState
        loading={user.isLoading || profiles.isLoading}
        error={user.isError || profiles.isError}
        onRetry={() => { void user.refetch(); if (user.data?.id) void profiles.refetch(); }}
      />
    );
  }

  return (
    <FocusRoot autoFocus className="screen set-page">
      <h1 className="set-title">{t("auth.settings.tabs.playback", "Playback")}</h1>
      <SettingsGroup title={t("auth.settings.playback.bitrateTitle")} hint={t("auth.settings.playback.bitrateDesc")}>
        {rows(BITRATE_OPTIONS, prefs.max_bitrate_kbps ?? "0", (v) => void save({ max_bitrate_kbps: v }), true)}
      </SettingsGroup>
      <SettingsGroup title={t("auth.settings.playback.preferredVideoCodec")}>
        {rows(VIDEO_CODEC_OPTIONS, prefs.preferred_video_codec ?? "auto", (v) => void save({ preferred_video_codec: v }))}
      </SettingsGroup>
      <SettingsGroup title={t("auth.settings.playback.preferredAudioCodec")}>
        {rows(AUDIO_CODEC_OPTIONS, prefs.preferred_audio_codec ?? "auto", (v) => void save({ preferred_audio_codec: v }))}
      </SettingsGroup>
      <SettingsGroup title={t("auth.settings.playback.maxAudioChannels")}>
        {rows(AUDIO_CHANNELS_OPTIONS, prefs.max_audio_channels ?? "auto", (v) => void save({ max_audio_channels: v }))}
      </SettingsGroup>
      <SettingsGroup title={t("auth.settings.playback.subtitleLanguage")}>
        {rows(LANG_OPTIONS, resolveLangPref(prefs, "subtitle_languages", "default_subtitle_language"), (v) => void save({ subtitle_languages: v }))}
      </SettingsGroup>
      <SettingsGroup title={t("auth.settings.playback.audioLanguage")}>
        {rows(LANG_OPTIONS, resolveLangPref(prefs, "audio_languages", "default_audio_language"), (v) => void save({ audio_languages: v }))}
      </SettingsGroup>
      <SettingsGroup title={t("auth.settings.playback.subtitleBurnIn")} hint={t("auth.settings.playback.subtitleBurnInDesc")}>
        {rows(SUBTITLE_BURN_OPTIONS, prefs.subtitle_burn_in ?? "auto", (v) => void save({ subtitle_burn_in: v }))}
      </SettingsGroup>
      <SettingsGroup label={t("auth.settings.playback.autoplayNextTitle")} hint={t("auth.settings.playback.autoplayNextDesc")}>
        {rows(TOGGLE_OPTIONS, bool("autoplay_next_episode"), toggle("autoplay_next_episode"))}
      </SettingsGroup>
      <h2 className="set-section-title">{t("auth.settings.playback.transcodeTitle", "Transcoding")}</h2>
      <SettingsGroup label={t("auth.settings.playback.allowDirectPlay")} hint={t("auth.settings.playback.allowDirectPlayDesc")}>
        {rows(TOGGLE_OPTIONS, bool("allow_direct_play"), toggle("allow_direct_play"))}
      </SettingsGroup>
      <SettingsGroup label={t("auth.settings.playback.allowDirectStream")} hint={t("auth.settings.playback.allowDirectStreamDesc")}>
        {rows(TOGGLE_OPTIONS, bool("allow_direct_stream"), toggle("allow_direct_stream"))}
      </SettingsGroup>
      <SettingsGroup label={t("auth.settings.playback.allowTranscoding")} hint={t("auth.settings.playback.allowTranscodingDesc")}>
        {rows(TOGGLE_OPTIONS, bool("allow_transcoding"), toggle("allow_transcoding"))}
      </SettingsGroup>
      {updateProfile.isPending ? <div className="center-fill set-pending"><Spinner /></div> : null}
    </FocusRoot>
  );
}

function watchTime(ms: string | undefined): string {
  const total = parseInt(ms ?? "0", 10);
  const h = Math.floor(total / 3_600_000);
  const m = Math.floor((total % 3_600_000) / 60_000);
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

/** What this viewer has watched, the Android television's figures and bars. */
export function StatsScreen() {
  const { t } = useTranslation();
  const profileId = useAuthStore((s) => s.profileId);
  const { data, isLoading, isError, refetch } = useActivityServiceGetUserStats(profileId ?? "", undefined, { query: { enabled: !!profileId } });
  const genres = useMemo(
    () => [...(data?.genres ?? [])].sort((a, b) => parseInt(b.totalTimeMs ?? "0", 10) - parseInt(a.totalTimeMs ?? "0", 10)),
    [data?.genres],
  );
  if (isLoading || isError) return <ScreenState loading={isLoading} error={isError} onRetry={() => void refetch()} />;
  if (!data || (data.totalPlays ?? 0) === 0) {
    return (
      <FocusRoot autoFocus className="center-fill stats-empty">
        <p className="stats-empty-text">{t("common.userStats.noData", "Nothing watched yet")}</p>
        <Pressable className="tv-card state-btn" autoFocus onPress={goBack}>{t("common.back", "Back")}</Pressable>
      </FocusRoot>
    );
  }
  const top = parseInt(genres[0]?.totalTimeMs ?? "0", 10) || 1;
  const mostPlayed = data.mostPlayed ?? [];
  const card = (label: string, value: string | number) => (
    <div className="stats-card"><span className="stats-value">{value}</span><span className="stats-label">{label}</span></div>
  );
  return (
    <FocusRoot autoFocus className="screen stats-page">
      <h1 className="stats-title">{t("common.userStats.title", "My statistics")}</h1>
      <div className="stats-grid">
        {card(t("common.userStats.totalWatchTime"), watchTime(data.totalWatchTimeMs))}
        {card(t("common.userStats.totalPlays"), data.totalPlays ?? 0)}
        {card(t("common.userStats.uniqueItems"), data.uniqueItemsWatched ?? 0)}
        {card(t("common.userStats.favoriteGenre"), genres[0]?.genre ?? "-")}
      </div>
      {genres.length > 0 ? (
        <div className="stats-block">
          <h2 className="section-title stats-section">{t("common.userStats.genreBreakdown")}</h2>
          {genres.slice(0, 8).map((g) => (
            <div className="stats-genre" key={g.genre}>
              <span className="stats-genre-name">{g.genre}</span>
              <span className="stats-genre-track"><i style={{ width: `${Math.max(2, (parseInt(g.totalTimeMs ?? "0", 10) / top) * 100)}%` }} /></span>
              <span className="stats-genre-time">{watchTime(g.totalTimeMs)}</span>
            </div>
          ))}
        </div>
      ) : null}
      {mostPlayed.length > 0 ? (
        <FocusRoot className="stats-block">
          <h2 className="section-title stats-section">{t("common.userStats.mostPlayed")}</h2>
          {mostPlayed.map((m, i) => (
            <Pressable key={m.mediaItemId ?? i} className="tv-card stats-played" onPress={() => { if (m.mediaItemId) navigate({ name: "media", mediaId: m.mediaItemId }); }}>
              <span className="stats-played-title">{m.title}</span>
              <span className="stats-played-count">{m.playCount ?? 0} {t("common.userStats.playsLabel")}</span>
            </Pressable>
          ))}
        </FocusRoot>
      ) : null}
    </FocusRoot>
  );
}

/** The servers this television knows, as Android's Manage Servers lists them. */
export function ServersScreen() {
  const { t } = useTranslation();
  const [error, setError] = useState<string | null>(null);
  const activeId = useServerListStore((s) => s.activeServerId);
  const pick = (server: { id: string; url: string }) => {
    if (server.id === activeId) { goBack(); return; }
    selectServer(server.url);
    void signOut().then(() => replace({ name: "login" }));
  };
  return (
    <FocusRoot autoFocus className="auth-page servers-page">
      <h1 className="set-title">{t("mobile.servers.manageTitle", "Manage Servers")}</h1>
      <SavedServers onPick={pick} onError={setError} />
      {error ? <p className="auth-error">{error}</p> : null}
    </FocusRoot>
  );
}
