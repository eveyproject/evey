import { useState, useEffect, useCallback } from "react";
import { View, Text, ScrollView, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  useUserServiceUpdateProfile,
  getUserServiceListProfilesQueryKey,
  type V1Profile,
} from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { ScreenState } from "../../../src/components/ScreenState";
import { useAuthStore } from "@evey/store";

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

// resolveLangPref stays exported here: the screen's test imports it by this path.
export { resolveLangPref };

export default function PlaybackSettingsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const {
    data: user,
    isLoading: userLoading,
    isError: userError,
    refetch: refetchUser,
  } = useUserServiceGetCurrentUser();
  const {
    data: profilesResp,
    isLoading: profilesLoading,
    isError: profilesError,
    refetch: refetchProfiles,
  } = useUserServiceListProfiles(user?.id ?? "", {
    query: { enabled: !!user?.id },
  });

  const isLoading = userLoading || profilesLoading;
  const isError = userError || profilesError;
  const refetch = useCallback(() => {
    void refetchUser();
    if (user?.id) void refetchProfiles();
  }, [refetchUser, refetchProfiles, user?.id]);

  const profiles = profilesResp?.profiles;
  // The profile in front of the viewer, not the household's default one. The
  // player reads audio_languages / subtitle_languages off the ACTIVE profile,
  // so preferences written onto the default one looked saved and changed
  // nothing about playback. (tabs)/settings.tsx already resolves it this way.
  const activeProfileId = useAuthStore((s) => s.profileId);
  const editedProfile: V1Profile | undefined =
    profiles?.find((p) => p.id === activeProfileId) ??
    profiles?.find((p) => p.isDefault) ??
    profiles?.[0];

  const [bitrate, setBitrate] = useState("0");
  const [subtitleLang, setSubtitleLang] = useState("");
  const [audioLang, setAudioLang] = useState("");
  const [videoCodec, setVideoCodec] = useState("auto");
  const [audioCodec, setAudioCodec] = useState("auto");
  const [audioChannels, setAudioChannels] = useState("auto");
  const [subtitleBurn, setSubtitleBurn] = useState("auto");
  const [autoplayNext, setAutoplayNext] = useState(true);
  const [allowDirectPlay, setAllowDirectPlay] = useState(true);
  const [allowDirectStream, setAllowDirectStream] = useState(true);
  const [allowTranscoding, setAllowTranscoding] = useState(true);
  const [focusedItem, setFocusedItem] = useState<string | null>(null);

  useEffect(() => {
    const prefs: PrefMap = editedProfile?.preferences ?? {};
    if (prefs.max_bitrate_kbps) setBitrate(prefs.max_bitrate_kbps);
    setSubtitleLang(resolveLangPref(prefs, "subtitle_languages", "default_subtitle_language"));
    setAudioLang(resolveLangPref(prefs, "audio_languages", "default_audio_language"));
    if (prefs.preferred_video_codec) setVideoCodec(prefs.preferred_video_codec);
    if (prefs.preferred_audio_codec) setAudioCodec(prefs.preferred_audio_codec);
    if (prefs.max_audio_channels) setAudioChannels(prefs.max_audio_channels);
    if (prefs.subtitle_burn_in) setSubtitleBurn(prefs.subtitle_burn_in);
    if (prefs.autoplay_next_episode !== undefined) setAutoplayNext(prefs.autoplay_next_episode !== "false");
    if (prefs.allow_direct_play !== undefined) setAllowDirectPlay(prefs.allow_direct_play !== "false");
    if (prefs.allow_direct_stream !== undefined) setAllowDirectStream(prefs.allow_direct_stream !== "false");
    if (prefs.allow_transcoding !== undefined) setAllowTranscoding(prefs.allow_transcoding !== "false");
  }, [editedProfile]);

  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onSuccess: () => {
        if (user?.id) {
          void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
        }
      },
    },
  });

  // Applies the value to the screen only once the server confirms it, so a
  // failed save (toasted by the global mutationErrorNet) never leaves the
  // check mark on a value the profile doesn't actually have.
  const savePref = useCallback(
    (patch: PrefMap) => {
      if (!editedProfile?.id) return Promise.reject(new Error("no default profile"));
      return updateProfile.mutateAsync({
        id: editedProfile.id,
        data: { preferences: { ...editedProfile.preferences, ...patch } },
      });
    },
    [editedProfile, updateProfile],
  );

  async function handleBitrate(v: string) {
    try {
      await savePref({ max_bitrate_kbps: v });
      setBitrate(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleSubtitleLang(v: string) {
    try {
      await savePref({ subtitle_languages: v });
      setSubtitleLang(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleAudioLang(v: string) {
    try {
      await savePref({ audio_languages: v });
      setAudioLang(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleVideoCodec(v: string) {
    try {
      await savePref({ preferred_video_codec: v });
      setVideoCodec(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleAudioCodec(v: string) {
    try {
      await savePref({ preferred_audio_codec: v });
      setAudioCodec(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleAudioChannels(v: string) {
    try {
      await savePref({ max_audio_channels: v });
      setAudioChannels(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleSubtitleBurn(v: string) {
    try {
      await savePref({ subtitle_burn_in: v });
      setSubtitleBurn(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleAutoplayNext(v: boolean) {
    try {
      await savePref({ autoplay_next_episode: v ? "true" : "false" });
      setAutoplayNext(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleAllowDirectPlay(v: boolean) {
    try {
      await savePref({ allow_direct_play: v ? "true" : "false" });
      setAllowDirectPlay(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleAllowDirectStream(v: boolean) {
    try {
      await savePref({ allow_direct_stream: v ? "true" : "false" });
      setAllowDirectStream(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleAllowTranscoding(v: boolean) {
    try {
      await savePref({ allow_transcoding: v ? "true" : "false" });
      setAllowTranscoding(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }

  const labelOf = (o: { labelKey: string; fallback: string }) =>
    o.labelKey ? t(o.labelKey, o.fallback) : o.fallback;

  const rowStyle = useCallback(
    (key: string) => [
      styles.row,
      {
        backgroundColor: focusedItem === key ? theme.colors.accent + "33" : "transparent",
        borderRadius: theme.radii.md,
      },
    ],
    [focusedItem, theme],
  );

  function OptionRow({
    rowKey,
    label,
    selected,
    onPress,
    hasTVPreferredFocus,
  }: {
    rowKey: string;
    label: string;
    selected: boolean;
    onPress: () => void;
    hasTVPreferredFocus?: boolean;
  }) {
    return (
      <Pressable
        style={rowStyle(rowKey)}
        onPress={onPress}
        onFocus={() => setFocusedItem(rowKey)}
        onBlur={() => setFocusedItem(null)}
        disabled={!editedProfile || updateProfile.isPending}
        accessible={true}
        accessibilityLabel={label}
        accessibilityRole="radio"
        accessibilityState={{ selected }}
        hasTVPreferredFocus={hasTVPreferredFocus}
      >
        <Text style={[styles.label, { color: theme.colors.text }]}>{label}</Text>
        {selected && (
          <Text style={{ color: theme.colors.accent, fontSize: 20, fontWeight: "600" }}>{"✓"}</Text>
        )}
      </Pressable>
    );
  }

  // A failed GetCurrentUser/ListProfiles used to leave every option row
  // permanently unselected with no explanation - indistinguishable from a
  // profile that simply has no saved preferences yet.
  if (isLoading || isError) {
    return <ScreenState loading={isLoading} error={isError} onRetry={refetch} />;
  }

  return (
    <ScrollView
      style={[styles.container, { backgroundColor: theme.colors.bg }]}
      contentContainerStyle={{ padding: 32 }}
    >
      <Text style={[styles.header, { color: theme.colors.text }]}>{t("auth.settings.tabs.playback", "Playback")}</Text>

      {/* Max Streaming Bitrate */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.bitrateTitle", "Max Streaming Bitrate")}
      </Text>
      <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.bitrateDesc", "Limit the maximum bitrate when streaming. Lower values use less bandwidth.")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {BITRATE_OPTIONS.map((opt, i) => (
          <OptionRow
            key={opt.value}
            rowKey={`bitrate-${opt.value}`}
            label={labelOf(opt)}
            selected={bitrate === opt.value}
            onPress={() => handleBitrate(opt.value)}
            hasTVPreferredFocus={i === 0}
          />
        ))}
      </View>

      {/* Preferred Video Codec */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.preferredVideoCodec", "Preferred Video Codec")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {VIDEO_CODEC_OPTIONS.map((opt) => (
          <OptionRow
            key={opt.value}
            rowKey={`video-codec-${opt.value}`}
            label={labelOf(opt)}
            selected={videoCodec === opt.value}
            onPress={() => handleVideoCodec(opt.value)}
          />
        ))}
      </View>

      {/* Preferred Audio Codec */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.preferredAudioCodec", "Preferred Audio Codec")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {AUDIO_CODEC_OPTIONS.map((opt) => (
          <OptionRow
            key={opt.value}
            rowKey={`audio-codec-${opt.value}`}
            label={labelOf(opt)}
            selected={audioCodec === opt.value}
            onPress={() => handleAudioCodec(opt.value)}
          />
        ))}
      </View>

      {/* Max Audio Channels */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.maxAudioChannels", "Max Audio Channels")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {AUDIO_CHANNELS_OPTIONS.map((opt) => (
          <OptionRow
            key={opt.value}
            rowKey={`audio-channels-${opt.value}`}
            label={labelOf(opt)}
            selected={audioChannels === opt.value}
            onPress={() => handleAudioChannels(opt.value)}
          />
        ))}
      </View>

      {/* Default Subtitle Language */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.subtitleLanguage", "Default Subtitle Language")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {LANG_OPTIONS.map((opt) => (
          <OptionRow
            key={opt.value}
            rowKey={`subtitle-lang-${opt.value}`}
            label={labelOf(opt)}
            selected={subtitleLang === opt.value}
            onPress={() => handleSubtitleLang(opt.value)}
          />
        ))}
      </View>

      {/* Default Audio Language */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.audioLanguage", "Default Audio Language")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {LANG_OPTIONS.map((opt) => (
          <OptionRow
            key={opt.value}
            rowKey={`audio-lang-${opt.value}`}
            label={labelOf(opt)}
            selected={audioLang === opt.value}
            onPress={() => handleAudioLang(opt.value)}
          />
        ))}
      </View>

      {/* Subtitle Burn-in Mode */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.subtitleBurnIn", "Subtitle Burn-in")}
      </Text>
      <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.subtitleBurnInDesc", "Burn subtitles directly into the video stream when needed.")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {SUBTITLE_BURN_OPTIONS.map((opt) => (
          <OptionRow
            key={opt.value}
            rowKey={`subtitle-burn-${opt.value}`}
            label={labelOf(opt)}
            selected={subtitleBurn === opt.value}
            onPress={() => handleSubtitleBurn(opt.value)}
          />
        ))}
      </View>

      {/* Auto-play next episode */}
      <Text style={[styles.toggleLabel, { color: theme.colors.text }]}>
        {t("auth.settings.playback.autoplayNextTitle", "Auto-play next episode")}
      </Text>
      <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.autoplayNextDesc", "Start the next episode automatically when this one ends.")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {TOGGLE_OPTIONS.map((opt) => (
          <OptionRow
            key={`autoplay-next-${String(opt.value)}`}
            rowKey={`autoplay-next-${String(opt.value)}`}
            label={t(opt.labelKey, opt.fallback)}
            selected={autoplayNext === opt.value}
            onPress={() => handleAutoplayNext(opt.value)}
          />
        ))}
      </View>

      {/* Transcoding toggles */}
      <Text style={[styles.sectionTitle, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.transcodeTitle", "Transcoding")}
      </Text>

      <Text style={[styles.toggleLabel, { color: theme.colors.text }]}>
        {t("auth.settings.playback.allowDirectPlay", "Allow direct play")}
      </Text>
      <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.allowDirectPlayDesc", "Play original file when device supports it.")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {TOGGLE_OPTIONS.map((opt) => (
          <OptionRow
            key={`direct-play-${String(opt.value)}`}
            rowKey={`direct-play-${String(opt.value)}`}
            label={t(opt.labelKey, opt.fallback)}
            selected={allowDirectPlay === opt.value}
            onPress={() => handleAllowDirectPlay(opt.value)}
          />
        ))}
      </View>

      <Text style={[styles.toggleLabel, { color: theme.colors.text }]}>
        {t("auth.settings.playback.allowDirectStream", "Allow remux")}
      </Text>
      <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.allowDirectStreamDesc", "Remux container without re-encoding when possible.")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {TOGGLE_OPTIONS.map((opt) => (
          <OptionRow
            key={`direct-stream-${String(opt.value)}`}
            rowKey={`direct-stream-${String(opt.value)}`}
            label={t(opt.labelKey, opt.fallback)}
            selected={allowDirectStream === opt.value}
            onPress={() => handleAllowDirectStream(opt.value)}
          />
        ))}
      </View>

      <Text style={[styles.toggleLabel, { color: theme.colors.text }]}>
        {t("auth.settings.playback.allowTranscoding", "Allow transcoding")}
      </Text>
      <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
        {t("auth.settings.playback.allowTranscodingDesc", "Fall back to transcoding when neither Direct Play nor remux is possible.")}
      </Text>
      <View style={[styles.section, { backgroundColor: theme.colors.surface }]}>
        {TOGGLE_OPTIONS.map((opt) => (
          <OptionRow
            key={`transcoding-${String(opt.value)}`}
            rowKey={`transcoding-${String(opt.value)}`}
            label={t(opt.labelKey, opt.fallback)}
            selected={allowTranscoding === opt.value}
            onPress={() => handleAllowTranscoding(opt.value)}
          />
        ))}
      </View>

      {updateProfile.isPending && (
        <View style={{ paddingVertical: 16, alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      )}

      <View style={{ height: 64 }} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  header: { fontSize: 30, fontWeight: "700", marginBottom: 24 },
  sectionTitle: {
    fontSize: 15,
    fontWeight: "600",
    textTransform: "uppercase",
    marginTop: 24,
    marginBottom: 8,
  },
  toggleLabel: {
    fontSize: 15,
    fontWeight: "600",
    marginTop: 24,
    marginBottom: 4,
  },
  hint: { fontSize: 13, marginBottom: 8 },
  section: {
    borderRadius: 12,
    overflow: "hidden",
    padding: 4,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 11,
    paddingHorizontal: 20,
  },
  label: {
    fontSize: 18,
  },
});
