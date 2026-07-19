import { useState, useEffect, useCallback } from "react";
import { View, Text, ScrollView, Pressable, ActivityIndicator, StyleSheet, Switch } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  getUserServiceListProfilesQueryKey,
  eveyFetch,
  type V1Profile,
} from "@evey/api";
import { useMutation } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";

const LANG_OPTIONS = [
  { value: "", label: "None" },
  { value: "eng", label: "English" },
  { value: "fre", label: "Fran\u00e7ais" },
  { value: "spa", label: "Espa\u00f1ol" },
  { value: "ger", label: "Deutsch" },
  { value: "ita", label: "Italiano" },
  { value: "por", label: "Portugu\u00eas" },
  { value: "jpn", label: "\u65e5\u672c\u8a9e" },
  { value: "kor", label: "\ud55c\uad6d\uc5b4" },
  { value: "chi", label: "\u4e2d\u6587" },
  { value: "rus", label: "\u0420\u0443\u0441\u0441\u043a\u0438\u0439" },
  { value: "ara", label: "\u0627\u0644\u0639\u0631\u0628\u064a\u0629" },
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

const CELLULAR_BITRATE_OPTIONS = [
  { value: "0", label: "Unlimited" },
  { value: "500", label: "0.5 Mbps" },
  { value: "1000", label: "1 Mbps" },
  { value: "2000", label: "2 Mbps" },
  { value: "4000", label: "4 Mbps" },
  { value: "8000", label: "8 Mbps" },
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
  const [cellularBitrate, setCellularBitrate] = useState("0");
  const [subtitleLang, setSubtitleLang] = useState("");
  const [audioLang, setAudioLang] = useState("");
  const [videoCodec, setVideoCodec] = useState("auto");
  const [audioCodec, setAudioCodec] = useState("auto");
  const [audioChannels, setAudioChannels] = useState("auto");
  const [subtitleBurn, setSubtitleBurn] = useState("auto");
  const [allowDirectPlay, setAllowDirectPlay] = useState(true);
  const [allowDirectStream, setAllowDirectStream] = useState(true);
  const [allowTranscoding, setAllowTranscoding] = useState(true);

  useEffect(() => {
    const prefs: PrefMap = defaultProfile?.preferences ?? {};
    if (prefs.max_bitrate_kbps) setBitrate(prefs.max_bitrate_kbps);
    if (prefs.max_bitrate_cellular_kbps) setCellularBitrate(prefs.max_bitrate_cellular_kbps);
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

  const updatePreference = useMutation({
    mutationFn: (patch: PrefMap) =>
      eveyFetch({
        url: `/api/v1/profiles/${defaultProfile?.id}`,
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        data: {
          preferences: {
            ...defaultProfile?.preferences,
            ...patch,
          },
        },
      }),
    onSuccess: () => {
      if (user?.id) {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
      }
    },
  });

  const savePref = useCallback(
    (patch: PrefMap) => {
      if (!defaultProfile) return;
      updatePreference.mutate(patch);
    },
    [defaultProfile, updatePreference],
  );

  function handleBitrate(v: string) {
    setBitrate(v);
    savePref({ max_bitrate_kbps: v });
  }
  function handleCellularBitrate(v: string) {
    setCellularBitrate(v);
    savePref({ max_bitrate_cellular_kbps: v });
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

  const s = styles(theme);
  const labelOf = (o: { labelKey: string; fallback: string }) => (o.labelKey ? t(o.labelKey, o.fallback) : o.fallback);

  return (
    <ScrollView style={s.container} contentContainerStyle={{ paddingBottom: 40 }}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("auth.settings.tabs.playback"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      {/* Max Bitrate WiFi */}
      <Text style={s.sectionTitle}>{t("auth.settings.playback.bitrateTitle")}</Text>
      <View style={s.hint}>
        <Text style={s.hintText}>{t("auth.settings.playback.bitrateDesc")}</Text>
      </View>
      <View style={s.section}>
        {BITRATE_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={s.row}
            onPress={() => handleBitrate(opt.value)}
            disabled={!defaultProfile || updatePreference.isPending}
            accessibilityRole="radio"
          >
            <Text style={s.label}>{opt.label}</Text>
            {bitrate === opt.value && <Text style={s.check}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Max Bitrate Cellular */}
      <Text style={s.sectionTitle}>
        {t("auth.settings.playback.cellularBitrateTitle", "Max Cellular Bitrate")}
      </Text>
      <View style={s.hint}>
        <Text style={s.hintText}>
          {t(
            "auth.settings.playback.cellularBitrateDesc",
            "Lower cap applied when using mobile data.",
          )}
        </Text>
      </View>
      <View style={s.section}>
        {CELLULAR_BITRATE_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={s.row}
            onPress={() => handleCellularBitrate(opt.value)}
            disabled={!defaultProfile || updatePreference.isPending}
            accessibilityRole="radio"
          >
            <Text style={s.label}>{opt.label}</Text>
            {cellularBitrate === opt.value && <Text style={s.check}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Preferred Video Codec */}
      <Text style={s.sectionTitle}>
        {t("auth.settings.playback.preferredVideoCodec", "Preferred Video Codec")}
      </Text>
      <View style={s.section}>
        {VIDEO_CODEC_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={s.row}
            onPress={() => handleVideoCodec(opt.value)}
            disabled={!defaultProfile || updatePreference.isPending}
            accessibilityRole="radio"
          >
            <Text style={s.label}>{labelOf(opt)}</Text>
            {videoCodec === opt.value && <Text style={s.check}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Preferred Audio Codec */}
      <Text style={s.sectionTitle}>
        {t("auth.settings.playback.preferredAudioCodec", "Preferred Audio Codec")}
      </Text>
      <View style={s.section}>
        {AUDIO_CODEC_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={s.row}
            onPress={() => handleAudioCodec(opt.value)}
            disabled={!defaultProfile || updatePreference.isPending}
            accessibilityRole="radio"
          >
            <Text style={s.label}>{labelOf(opt)}</Text>
            {audioCodec === opt.value && <Text style={s.check}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Max Audio Channels */}
      <Text style={s.sectionTitle}>
        {t("auth.settings.playback.maxAudioChannels", "Max Audio Channels")}
      </Text>
      <View style={s.section}>
        {AUDIO_CHANNELS_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={s.row}
            onPress={() => handleAudioChannels(opt.value)}
            disabled={!defaultProfile || updatePreference.isPending}
            accessibilityRole="radio"
          >
            <Text style={s.label}>{labelOf(opt)}</Text>
            {audioChannels === opt.value && <Text style={s.check}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Default Subtitle Language */}
      <Text style={s.sectionTitle}>{t("auth.settings.playback.subtitleLanguage")}</Text>
      <View style={s.section}>
        {LANG_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={s.row}
            onPress={() => handleSubtitleLang(opt.value)}
            disabled={!defaultProfile}
            accessibilityRole="radio"
          >
            <Text style={s.label}>{opt.label}</Text>
            {subtitleLang === opt.value && <Text style={s.check}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Default Audio Language */}
      <Text style={s.sectionTitle}>{t("auth.settings.playback.audioLanguage")}</Text>
      <View style={s.section}>
        {LANG_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={s.row}
            onPress={() => handleAudioLang(opt.value)}
            disabled={!defaultProfile}
            accessibilityRole="radio"
          >
            <Text style={s.label}>{opt.label}</Text>
            {audioLang === opt.value && <Text style={s.check}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Subtitle Burn-in Mode */}
      <Text style={s.sectionTitle}>
        {t("auth.settings.playback.subtitleBurnIn", "Subtitle Burn-in")}
      </Text>
      <View style={s.hint}>
        <Text style={s.hintText}>
          {t(
            "auth.settings.playback.subtitleBurnInDesc",
            "Burn subtitles directly into the video stream when needed.",
          )}
        </Text>
      </View>
      <View style={s.section}>
        {SUBTITLE_BURN_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={s.row}
            onPress={() => handleSubtitleBurn(opt.value)}
            disabled={!defaultProfile || updatePreference.isPending}
            accessibilityRole="radio"
          >
            <Text style={s.label}>{labelOf(opt)}</Text>
            {subtitleBurn === opt.value && <Text style={s.check}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Transcoding toggles */}
      <Text style={s.sectionTitle}>{t("auth.settings.playback.transcodeTitle", "Transcoding")}</Text>
      <View style={s.section}>
        <View style={[s.row, { justifyContent: "space-between" }]}>
          <View style={{ flex: 1, paddingRight: 12 }}>
            <Text style={s.label}>
              {t("auth.settings.playback.allowDirectPlay", "Allow direct play")}
            </Text>
            <Text style={s.sublabel}>
              {t(
                "auth.settings.playback.allowDirectPlayDesc",
                "Play original file when device supports it.",
              )}
            </Text>
          </View>
          <Switch
            value={allowDirectPlay}
            onValueChange={handleAllowDirectPlay}
            disabled={!defaultProfile}
            trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
          />
        </View>
        <View style={[s.row, { justifyContent: "space-between" }]}>
          <View style={{ flex: 1, paddingRight: 12 }}>
            <Text style={s.label}>
              {t("auth.settings.playback.allowDirectStream", "Allow direct stream")}
            </Text>
            <Text style={s.sublabel}>
              {t(
                "auth.settings.playback.allowDirectStreamDesc",
                "Remux container without re-encoding when possible.",
              )}
            </Text>
          </View>
          <Switch
            value={allowDirectStream}
            onValueChange={handleAllowDirectStream}
            disabled={!defaultProfile}
            trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
          />
        </View>
        <View style={[s.row, { justifyContent: "space-between" }]}>
          <View style={{ flex: 1, paddingRight: 12 }}>
            <Text style={s.label}>
              {t("auth.settings.playback.allowTranscoding", "Allow transcoding")}
            </Text>
            <Text style={s.sublabel}>
              {t(
                "auth.settings.playback.allowTranscodingDesc",
                "Fall back to transcoding if direct play/stream is unavailable.",
              )}
            </Text>
          </View>
          <Switch
            value={allowTranscoding}
            onValueChange={handleAllowTranscoding}
            disabled={!defaultProfile}
            trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
          />
        </View>
      </View>

      {updatePreference.isPending && (
        <View style={{ padding: theme.spacing.md, alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} size="small" />
        </View>
      )}
    </ScrollView>
  );
}

const styles = (t: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    container: { flex: 1, backgroundColor: t.colors.bg },
    sectionTitle: {
      color: t.colors.textMuted,
      fontSize: 13,
      fontWeight: "600",
      textTransform: "uppercase",
      marginTop: t.spacing.lg,
      marginBottom: t.spacing.xs,
      marginHorizontal: t.spacing.md,
    },
    hint: { marginHorizontal: t.spacing.md, marginBottom: t.spacing.sm },
    hintText: { color: t.colors.textMuted, fontSize: 13 },
    section: {
      backgroundColor: t.colors.surface,
      borderRadius: t.radii.md,
      marginHorizontal: t.spacing.md,
      overflow: "hidden",
    },
    row: {
      flexDirection: "row",
      alignItems: "center",
      justifyContent: "space-between",
      paddingVertical: 14,
      paddingHorizontal: t.spacing.md,
      borderBottomWidth: StyleSheet.hairlineWidth,
      borderBottomColor: t.colors.border,
    },
    label: { color: t.colors.text, fontSize: 15 },
    sublabel: { color: t.colors.textMuted, fontSize: 12, marginTop: 2 },
    check: { color: t.colors.accent, fontSize: 16, fontWeight: "600" },
  });
