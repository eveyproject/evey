import { useState, useEffect, useCallback } from "react";
import { View, Text, ScrollView, Pressable, ActivityIndicator, StyleSheet, Switch } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceGetCurrentUser,
  getUserServiceListProfilesQueryKey,
  eveyFetch,
} from "@evey/api";
import { useMutation } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useActiveProfile } from "../../../src/hooks/useActiveProfile";

// Values are ISO 639-1 (2-letter) to match the audio_languages/subtitle_languages
// convention written by web (frontend/apps/web/src/features/user/LanguageSettings.tsx)
// and read by HLSPlayer's track resolution.
const LANG_OPTIONS = [
  { value: "", label: "None" },
  { value: "en", label: "English" },
  { value: "fr", label: "Fran\u00e7ais" },
  { value: "es", label: "Espa\u00f1ol" },
  { value: "de", label: "Deutsch" },
  { value: "it", label: "Italiano" },
  { value: "pt", label: "Portugu\u00eas" },
  { value: "ja", label: "\u65e5\u672c\u8a9e" },
  { value: "ko", label: "\ud55c\uad6d\uc5b4" },
  { value: "zh", label: "\u4e2d\u6587" },
  { value: "ru", label: "\u0420\u0443\u0441\u0441\u043a\u0438\u0439" },
  { value: "ar", label: "\u0627\u0644\u0639\u0631\u0628\u064a\u0629" },
];

// Older mobile builds stored a single ISO 639-2/B code under
// default_audio_language/default_subtitle_language. Map the ones this screen
// ever wrote to their ISO 639-1 equivalent so upgrading users keep their pick
// selected until they re-save under the new key.
const LEGACY_LANG_MAP: Record<string, string> = {
  eng: "en", fre: "fr", spa: "es", ger: "de", ita: "it",
  por: "pt", jpn: "ja", kor: "ko", chi: "zh", rus: "ru", ara: "ar",
};

function legacyLangToIso1(code: string | undefined): string | undefined {
  if (code === undefined) return undefined;
  return LEGACY_LANG_MAP[code] ?? code;
}

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
  // The profile in front of the user, not the household's default one: the
  // player reads audio_languages / subtitle_languages off the ACTIVE profile,
  // so preferences written onto the default one looked saved and changed
  // nothing about playback.
  const { profile: editedProfile } = useActiveProfile();

  const [bitrate, setBitrate] = useState("0");
  const [cellularBitrate, setCellularBitrate] = useState("0");
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

  useEffect(() => {
    const prefs: PrefMap = editedProfile?.preferences ?? {};
    if (prefs.max_bitrate_kbps) setBitrate(prefs.max_bitrate_kbps);
    if (prefs.max_bitrate_cellular_kbps) setCellularBitrate(prefs.max_bitrate_cellular_kbps);
    // audio_languages/subtitle_languages is the shared web/tv convention (CSV
    // of ISO 639-1 codes in profile.preferences). Fall back to the legacy
    // mobile-only keys when the new ones are absent (soft migration on read).
    const rawAudio = prefs.audio_languages ?? legacyLangToIso1(prefs.default_audio_language);
    const rawSubtitle = prefs.subtitle_languages ?? legacyLangToIso1(prefs.default_subtitle_language);
    if (rawAudio !== undefined) setAudioLang(rawAudio.split(",")[0]?.trim() ?? "");
    if (rawSubtitle !== undefined) setSubtitleLang(rawSubtitle.split(",")[0]?.trim() ?? "");
    if (prefs.preferred_video_codec) setVideoCodec(prefs.preferred_video_codec);
    if (prefs.preferred_audio_codec) setAudioCodec(prefs.preferred_audio_codec);
    if (prefs.max_audio_channels) setAudioChannels(prefs.max_audio_channels);
    if (prefs.subtitle_burn_in) setSubtitleBurn(prefs.subtitle_burn_in);
    if (prefs.autoplay_next_episode !== undefined) setAutoplayNext(prefs.autoplay_next_episode !== "false");
    if (prefs.allow_direct_play !== undefined) setAllowDirectPlay(prefs.allow_direct_play !== "false");
    if (prefs.allow_direct_stream !== undefined) setAllowDirectStream(prefs.allow_direct_stream !== "false");
    if (prefs.allow_transcoding !== undefined) setAllowTranscoding(prefs.allow_transcoding !== "false");
  }, [editedProfile]);

  const updatePreference = useMutation({
    mutationFn: (patch: PrefMap) =>
      eveyFetch({
        url: `/api/v1/profiles/${editedProfile?.id}`,
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        data: {
          preferences: {
            ...editedProfile?.preferences,
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

  // Applies the value to the screen only once the server confirms it, so a
  // failed save (toasted by the global mutationErrorNet) never leaves the
  // check mark on a value the profile doesn't actually have.
  const savePref = useCallback(
    (patch: PrefMap) => {
      if (!editedProfile) return Promise.reject(new Error("no default profile"));
      return updatePreference.mutateAsync(patch);
    },
    [editedProfile, updatePreference],
  );

  async function handleBitrate(v: string) {
    try {
      await savePref({ max_bitrate_kbps: v });
      setBitrate(v);
    } catch {
      // mutationErrorNet already toasted the failure
    }
  }
  async function handleCellularBitrate(v: string) {
    try {
      await savePref({ max_bitrate_cellular_kbps: v });
      setCellularBitrate(v);
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
            disabled={!editedProfile || updatePreference.isPending}
            accessibilityRole="radio"
            accessibilityState={{ selected: bitrate === opt.value, disabled: !editedProfile || updatePreference.isPending }}
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
            disabled={!editedProfile || updatePreference.isPending}
            accessibilityRole="radio"
            accessibilityState={{ selected: cellularBitrate === opt.value, disabled: !editedProfile || updatePreference.isPending }}
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
            disabled={!editedProfile || updatePreference.isPending}
            accessibilityRole="radio"
            accessibilityState={{ selected: videoCodec === opt.value, disabled: !editedProfile || updatePreference.isPending }}
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
            disabled={!editedProfile || updatePreference.isPending}
            accessibilityRole="radio"
            accessibilityState={{ selected: audioCodec === opt.value, disabled: !editedProfile || updatePreference.isPending }}
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
            disabled={!editedProfile || updatePreference.isPending}
            accessibilityRole="radio"
            accessibilityState={{ selected: audioChannels === opt.value, disabled: !editedProfile || updatePreference.isPending }}
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
            disabled={!editedProfile}
            accessibilityRole="radio"
            accessibilityState={{ selected: subtitleLang === opt.value, disabled: !editedProfile }}
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
            disabled={!editedProfile}
            accessibilityRole="radio"
            accessibilityState={{ selected: audioLang === opt.value, disabled: !editedProfile }}
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
            disabled={!editedProfile || updatePreference.isPending}
            accessibilityRole="radio"
            accessibilityState={{ selected: subtitleBurn === opt.value, disabled: !editedProfile || updatePreference.isPending }}
          >
            <Text style={s.label}>{labelOf(opt)}</Text>
            {subtitleBurn === opt.value && <Text style={s.check}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Auto-play next episode */}
      <Text style={s.sectionTitle}>
        {t("auth.settings.playback.autoplayNextTitle", "Auto-play next episode")}
      </Text>
      <View style={s.section}>
        <View style={[s.row, { justifyContent: "space-between" }]}>
          <View style={{ flex: 1, paddingRight: 12 }}>
            <Text style={s.label}>
              {t("auth.settings.playback.autoplayNextTitle", "Auto-play next episode")}
            </Text>
            <Text style={s.sublabel}>
              {t(
                "auth.settings.playback.autoplayNextDesc",
                "Start the next episode automatically when one finishes.",
              )}
            </Text>
          </View>
          <Switch
            value={autoplayNext}
            onValueChange={handleAutoplayNext}
            disabled={!editedProfile}
            trackColor={{ true: theme.colors.accent, false: theme.colors.surfaceRaised }}
          />
        </View>
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
            disabled={!editedProfile}
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
            disabled={!editedProfile}
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
            disabled={!editedProfile}
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
    label: { color: t.colors.text, fontSize: 15, flex: 1, paddingRight: 12 },
    sublabel: { color: t.colors.textMuted, fontSize: 12, marginTop: 2 },
    check: { color: t.colors.accent, fontSize: 16, fontWeight: "600", flexShrink: 0 },
  });
