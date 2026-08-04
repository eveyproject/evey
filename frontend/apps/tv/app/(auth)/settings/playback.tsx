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

// ISO 639-1 (2-letter) to match the audio_languages/subtitle_languages
// convention shared with web (LanguageSettings.tsx) and mobile (settings/
// playback.tsx) - the preference is stored per-profile and read by every
// client's track resolution, so all three must agree on the same codes.
// Language names are each language's own endonym (not translated) - same
// choice web/mobile made, and consistent with the untranslated codec proper
// nouns (H.264, AC3, ...) below. "None" reuses web's common.noPreference key.
const LANG_OPTIONS = [
  { value: "", labelKey: "common.noPreference", fallback: "No preference" },
  { value: "en", labelKey: "", fallback: "English" },
  { value: "fr", labelKey: "", fallback: "Français" },
  { value: "es", labelKey: "", fallback: "Español" },
  { value: "de", labelKey: "", fallback: "Deutsch" },
  { value: "it", labelKey: "", fallback: "Italiano" },
  { value: "pt", labelKey: "", fallback: "Português" },
  { value: "ja", labelKey: "", fallback: "日本語" },
  { value: "ko", labelKey: "", fallback: "한국어" },
  { value: "zh", labelKey: "", fallback: "中文" },
  { value: "ru", labelKey: "", fallback: "Русский" },
  { value: "ar", labelKey: "", fallback: "العربية" },
];

// This screen used to save ISO 639-2/B codes ("fre", "ger", ...) under these
// same preference keys. Map them to 639-1 on read so a value saved before
// this fix still highlights the matching option instead of showing none
// selected, until the user re-saves under the new 2-letter code.
const LEGACY_LANG_MAP: Record<string, string> = {
  eng: "en", fre: "fr", spa: "es", ger: "de", ita: "it",
  por: "pt", jpn: "ja", kor: "ko", chi: "zh", rus: "ru", ara: "ar",
};

function legacyLangToIso1(code: string | undefined): string | undefined {
  if (code === undefined) return undefined;
  return LEGACY_LANG_MAP[code] ?? code;
}

// Resolves one language preference for this profile. audio_languages/
// subtitle_languages is the shared web/mobile/tv convention (CSV of ISO 639-1
// codes, first entry = highest priority); this screen only ever writes a
// single code, so we read just the first entry. Falls back to the TV-only
// default_audio_language/default_subtitle_language key (soft migration on
// read) so a language picked before this fix stays selected until re-saved.
export function resolveLangPref(
  prefs: PrefMap,
  sharedKey: "audio_languages" | "subtitle_languages",
  legacyKey: "default_audio_language" | "default_subtitle_language",
): string {
  const raw = prefs[sharedKey] ?? legacyLangToIso1(prefs[legacyKey]);
  if (raw === undefined) return "";
  return raw.split(",")[0]?.trim() ?? "";
}

const BITRATE_OPTIONS = [
  { value: "0", labelKey: "auth.settings.playback.bitrateUnlimited", fallback: "Unlimited" },
  { value: "2000", labelKey: "", fallback: "2 Mbps" },
  { value: "4000", labelKey: "", fallback: "4 Mbps" },
  { value: "8000", labelKey: "", fallback: "8 Mbps" },
  { value: "10000", labelKey: "", fallback: "10 Mbps" },
  { value: "15000", labelKey: "", fallback: "15 Mbps" },
  { value: "20000", labelKey: "", fallback: "20 Mbps" },
  { value: "40000", labelKey: "", fallback: "40 Mbps" },
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
    setSubtitleLang(resolveLangPref(prefs, "subtitle_languages", "default_subtitle_language"));
    setAudioLang(resolveLangPref(prefs, "audio_languages", "default_audio_language"));
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

  // Applies the value to the screen only once the server confirms it, so a
  // failed save (toasted by the global mutationErrorNet) never leaves the
  // check mark on a value the profile doesn't actually have.
  const savePref = useCallback(
    (patch: PrefMap) => {
      if (!defaultProfile?.id) return Promise.reject(new Error("no default profile"));
      return updateProfile.mutateAsync({
        id: defaultProfile.id,
        data: { preferences: { ...defaultProfile.preferences, ...patch } },
      });
    },
    [defaultProfile, updateProfile],
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
        disabled={!defaultProfile || updateProfile.isPending}
        accessible={true}
        accessibilityLabel={label}
        accessibilityRole="radio"
        accessibilityState={{ selected }}
        hasTVPreferredFocus={hasTVPreferredFocus}
      >
        <Text style={[styles.label, { color: theme.colors.text }]}>{label}</Text>
        {selected && (
          <Text style={{ color: theme.colors.accent, fontSize: 22, fontWeight: "600" }}>{"✓"}</Text>
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
