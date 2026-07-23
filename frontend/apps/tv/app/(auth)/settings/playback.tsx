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

const LANG_OPTIONS = [
  { value: "", label: "None" },
  { value: "eng", label: "English" },
  { value: "fre", label: "Français" },
  { value: "spa", label: "Español" },
  { value: "ger", label: "Deutsch" },
  { value: "ita", label: "Italiano" },
  { value: "por", label: "Português" },
  { value: "jpn", label: "日本語" },
  { value: "kor", label: "한국어" },
  { value: "chi", label: "中文" },
  { value: "rus", label: "Русский" },
  { value: "ara", label: "العربية" },
];

const BITRATE_OPTIONS = [
  { value: "0", label: "Unlimited" },
  { value: "2000", label: "2 Mbps" },
  { value: "4000", label: "4 Mbps" },
  { value: "8000", label: "8 Mbps" },
  { value: "10000", label: "10 Mbps" },
  { value: "15000", label: "15 Mbps" },
  { value: "20000", label: "20 Mbps" },
  { value: "40000", label: "40 Mbps" },
];

const VIDEO_CODEC_OPTIONS = [
  { value: "auto", labelKey: "auth.settings.playback.codecAuto", fallback: "Auto" },
  { value: "h264", labelKey: "", fallback: "H.264" },
  { value: "hevc", labelKey: "", fallback: "HEVC (H.265)" },
  { value: "av1", labelKey: "", fallback: "AV1" },
];

const AUDIO_CODEC_OPTIONS = [
  { value: "auto", labelKey: "auth.settings.playback.codecAuto", fallback: "Auto" },
  { value: "aac", labelKey: "", fallback: "AAC" },
  { value: "ac3", labelKey: "", fallback: "AC3" },
  { value: "eac3", labelKey: "", fallback: "E-AC3" },
  { value: "opus", labelKey: "", fallback: "Opus" },
];

const AUDIO_CHANNELS_OPTIONS = [
  { value: "auto", labelKey: "auth.settings.playback.codecAuto", fallback: "Auto" },
  { value: "2", labelKey: "auth.settings.playback.channelsStereo", fallback: "Stereo (2.0)" },
  { value: "6", labelKey: "auth.settings.playback.channels51", fallback: "Surround (5.1)" },
  { value: "8", labelKey: "auth.settings.playback.channels71", fallback: "Surround (7.1)" },
];

const SUBTITLE_BURN_OPTIONS = [
  { value: "auto", labelKey: "auth.settings.playback.subtitleBurnAuto", fallback: "Auto" },
  { value: "always", labelKey: "auth.settings.playback.subtitleBurnAlways", fallback: "Always burn in" },
  { value: "never", labelKey: "auth.settings.playback.subtitleBurnNever", fallback: "Never burn in" },
];

// Boolean toggles read as a 2-row choice ("Enabled"/"Disabled") rather than a
// react-native Switch: D-pad focus/select on a row is the idiom the rest of
// this screen (and the tabs settings screen) already uses for every choice.
const TOGGLE_OPTIONS = [
  { value: true, labelKey: "common.enabled", fallback: "Enabled" },
  { value: false, labelKey: "common.disabled", fallback: "Disabled" },
];

type PrefMap = Record<string, string>;

export default function PlaybackSettingsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", {
    query: { enabled: !!user?.id },
  });

  const profiles = profilesResp?.profiles;
  const defaultProfile: V1Profile | undefined = profiles?.find((p) => p.isDefault) ?? profiles?.[0];

  const [bitrate, setBitrate] = useState("0");
  const [subtitleLang, setSubtitleLang] = useState("");
  const [audioLang, setAudioLang] = useState("");
  const [videoCodec, setVideoCodec] = useState("auto");
  const [audioCodec, setAudioCodec] = useState("auto");
  const [audioChannels, setAudioChannels] = useState("auto");
  const [subtitleBurn, setSubtitleBurn] = useState("auto");
  const [allowDirectPlay, setAllowDirectPlay] = useState(true);
  const [allowDirectStream, setAllowDirectStream] = useState(true);
  const [allowTranscoding, setAllowTranscoding] = useState(true);
  const [focusedItem, setFocusedItem] = useState<string | null>(null);

  useEffect(() => {
    const prefs: PrefMap = defaultProfile?.preferences ?? {};
    if (prefs.max_bitrate_kbps) setBitrate(prefs.max_bitrate_kbps);
    if (prefs.default_subtitle_language) setSubtitleLang(prefs.default_subtitle_language);
    if (prefs.default_audio_language) setAudioLang(prefs.default_audio_language);
    if (prefs.preferred_video_codec) setVideoCodec(prefs.preferred_video_codec);
    if (prefs.preferred_audio_codec) setAudioCodec(prefs.preferred_audio_codec);
    if (prefs.max_audio_channels) setAudioChannels(prefs.max_audio_channels);
    if (prefs.subtitle_burn_in) setSubtitleBurn(prefs.subtitle_burn_in);
    if (prefs.allow_direct_play !== undefined) setAllowDirectPlay(prefs.allow_direct_play !== "false");
    if (prefs.allow_direct_stream !== undefined) setAllowDirectStream(prefs.allow_direct_stream !== "false");
    if (prefs.allow_transcoding !== undefined) setAllowTranscoding(prefs.allow_transcoding !== "false");
  }, [defaultProfile]);

  const updateProfile = useUserServiceUpdateProfile({
    mutation: {
      onSuccess: () => {
        if (user?.id) {
          void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
        }
      },
    },
  });

  const savePref = useCallback(
    (patch: PrefMap) => {
      if (!defaultProfile?.id) return;
      updateProfile.mutate({
        id: defaultProfile.id,
        data: { preferences: { ...defaultProfile.preferences, ...patch } },
      });
    },
    [defaultProfile, updateProfile],
  );

  function handleBitrate(v: string) {
    setBitrate(v);
    savePref({ max_bitrate_kbps: v });
  }
  function handleSubtitleLang(v: string) {
    setSubtitleLang(v);
    savePref({ default_subtitle_language: v });
  }
  function handleAudioLang(v: string) {
    setAudioLang(v);
    savePref({ default_audio_language: v });
  }
  function handleVideoCodec(v: string) {
    setVideoCodec(v);
    savePref({ preferred_video_codec: v });
  }
  function handleAudioCodec(v: string) {
    setAudioCodec(v);
    savePref({ preferred_audio_codec: v });
  }
  function handleAudioChannels(v: string) {
    setAudioChannels(v);
    savePref({ max_audio_channels: v });
  }
  function handleSubtitleBurn(v: string) {
    setSubtitleBurn(v);
    savePref({ subtitle_burn_in: v });
  }
  function handleAllowDirectPlay(v: boolean) {
    setAllowDirectPlay(v);
    savePref({ allow_direct_play: v ? "true" : "false" });
  }
  function handleAllowDirectStream(v: boolean) {
    setAllowDirectStream(v);
    savePref({ allow_direct_stream: v ? "true" : "false" });
  }
  function handleAllowTranscoding(v: boolean) {
    setAllowTranscoding(v);
    savePref({ allow_transcoding: v ? "true" : "false" });
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
        disabled={!defaultProfile || updateProfile.isPending}
        accessible={true}
        accessibilityLabel={label}
        accessibilityRole="radio"
        accessibilityState={{ selected }}
        hasTVPreferredFocus={hasTVPreferredFocus}
      >
        <Text style={[styles.label, { color: theme.colors.text }]}>{label}</Text>
        {selected && (
          <Text style={{ color: theme.colors.accent, fontSize: 22, fontWeight: "600" }}>OK</Text>
        )}
      </Pressable>
    );
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
            label={opt.label}
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
            label={opt.label}
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
            label={opt.label}
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
        {t("auth.settings.playback.allowDirectStream", "Allow direct stream")}
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
        {t("auth.settings.playback.allowTranscodingDesc", "Fall back to transcoding if direct play/stream is unavailable.")}
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
        <View style={{ paddingVertical: 24, alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="large" />
        </View>
      )}

      <View style={{ height: 64 }} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  header: { fontSize: 36, fontWeight: "700", marginBottom: 24 },
  sectionTitle: {
    fontSize: 16,
    fontWeight: "600",
    textTransform: "uppercase",
    marginTop: 24,
    marginBottom: 8,
  },
  toggleLabel: {
    fontSize: 16,
    fontWeight: "600",
    marginTop: 24,
    marginBottom: 4,
  },
  hint: { fontSize: 14, marginBottom: 8 },
  section: {
    borderRadius: 12,
    overflow: "hidden",
    padding: 4,
  },
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 16,
    paddingHorizontal: 20,
  },
  label: {
    fontSize: 20,
  },
});
