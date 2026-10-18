import { useState, useEffect, useCallback, useMemo } from "react";
import { View, Text, ScrollView, Pressable, TextInput, ActivityIndicator, StyleSheet, Switch } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { BottomSheet } from "../../../src/components/BottomSheet";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceGetCurrentUser,
  getUserServiceListProfilesQueryKey,
  eveyFetch,
} from "@evey/api";
import { useMutation } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useActiveProfile } from "../../../src/hooks/useActiveProfile";
import { AUDIO_CHANNELS_OPTIONS, AUDIO_CODEC_OPTIONS, BITRATE_OPTIONS, SUBTITLE_BURN_OPTIONS, VIDEO_CODEC_OPTIONS } from "@evey/client";
import { STILL_WATCHING_DEFAULT, STILL_WATCHING_PREF_KEY, STILL_WATCHING_STEPS, stillWatchingLimit } from "@evey/client";

// Values are ISO 639-1 (2-letter) to match the audio_languages/subtitle_languages
// convention written by web (frontend/apps/web/src/features/user/LanguageSettings.tsx)
// and read by HLSPlayer's track resolution.
// The media (audio/subtitle) language set, ISO 639-1, matching the web
// LanguageSettings MEDIA_LANG_CODES so the two apps offer the same choices.
const MEDIA_LANG_CODES = [
  "af", "ar", "bg", "bn", "ca", "cs", "cy", "da", "de", "el",
  "en", "es", "et", "eu", "fa", "fi", "fil", "fr", "ga", "gl",
  "gu", "he", "hi", "hr", "hu", "hy", "id", "is", "it", "ja",
  "ka", "kn", "ko", "lt", "lv", "mk", "ml", "mr", "ms", "mt",
  "nl", "no", "pa", "pl", "pt", "ro", "ru", "sk", "sl", "sq",
  "sr", "sv", "sw", "ta", "te", "th", "tr", "uk", "ur", "vi",
  "zh",
];

// Localize each code to the current UI language via Intl.DisplayNames, with a
// leading "no preference" entry (mirror web buildMediaLangOptions).
function buildMediaLangOptions(locale: string, noneLabel: string): { value: string; label: string }[] {
  let names: Intl.DisplayNames | undefined;
  try { names = new Intl.DisplayNames([locale], { type: "language" }); } catch { names = undefined; }
  const opts = MEDIA_LANG_CODES
    .map((c) => ({ value: c, label: names?.of(c) ?? c }))
    .sort((a, b) => a.label.localeCompare(b.label, locale));
  return [{ value: "", label: noneLabel }, ...opts];
}

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

// The phone alone caps the cellular bitrate; the other lists are shared with
// the televisions and the web (@evey/client playbackPrefs). The phone's own
// copies printed "Unlimited" in English whatever the language.
const CELLULAR_BITRATE_OPTIONS = [
  { value: "0", labelKey: "auth.settings.playback.bitrateUnlimited", fallback: "Unlimited" },
  { value: "500", labelKey: "", fallback: "0.5 Mbps" },
  { value: "1000", labelKey: "", fallback: "1 Mbps" },
  { value: "2000", labelKey: "", fallback: "2 Mbps" },
  { value: "4000", labelKey: "", fallback: "4 Mbps" },
  { value: "8000", labelKey: "", fallback: "8 Mbps" },
];

type PrefMap = Record<string, string>;

export default function PlaybackSettingsScreen() {
  const theme = useTheme();
  const { t, i18n } = useTranslation();
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
  const [subSheetOpen, setSubSheetOpen] = useState(false);
  const [audSheetOpen, setAudSheetOpen] = useState(false);
  const [langQuery, setLangQuery] = useState("");

  const langOptions = useMemo(
    () => buildMediaLangOptions(i18n.language, t("common.noPreference", "No preference")),
    [i18n.language, t],
  );
  const langLabel = useCallback((v: string) => langOptions.find((o) => o.value === v)?.label ?? (v || t("common.noPreference", "No preference")), [langOptions, t]);
  const filteredLangs = useMemo(() => {
    const q = langQuery.trim().toLowerCase();
    if (!q) return langOptions;
    return langOptions.filter((o) => o.label.toLowerCase().includes(q) || o.value.toLowerCase().includes(q));
  }, [langOptions, langQuery]);
  const [videoCodec, setVideoCodec] = useState("auto");
  const [audioCodec, setAudioCodec] = useState("auto");
  const [audioChannels, setAudioChannels] = useState("auto");
  const [subtitleBurn, setSubtitleBurn] = useState("auto");
  const [autoplayNext, setAutoplayNext] = useState(true);
  const [stillWatchingAfter, setStillWatchingAfter] = useState(STILL_WATCHING_DEFAULT);
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
    setStillWatchingAfter(stillWatchingLimit(prefs));
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
  async function handleStillWatching(n: number) {
    try {
      await savePref({ [STILL_WATCHING_PREF_KEY]: String(n) });
      setStillWatchingAfter(n);
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

  const renderLangSheet = (open: boolean, close: () => void, current: string, onPick: (v: string) => void, title: string) => (
    <BottomSheet visible={open} onClose={close} title={title}>
      <TextInput
        value={langQuery}
        onChangeText={setLangQuery}
        placeholder={t("common.search", "Search") || "Search"}
        placeholderTextColor={theme.colors.textMuted}
        style={[s.langSearch, { backgroundColor: theme.colors.surfaceRaised, color: theme.colors.text, borderRadius: theme.radii.md }]}
      />
      {filteredLangs.map((opt) => {
        const activeOpt = current === opt.value;
        return (
          <Pressable
            key={opt.value || "none"}
            style={[s.langSheetRow, { borderBottomColor: theme.colors.border }]}
            onPress={() => { onPick(opt.value); close(); }}
            accessibilityRole="radio"
            accessibilityState={{ selected: activeOpt }}
            accessibilityLabel={opt.label}
          >
            <Text style={{ color: activeOpt ? theme.colors.accent : theme.colors.text, fontSize: 15, flex: 1 }}>{opt.label}</Text>
            {activeOpt && <Text style={{ color: theme.colors.accent, fontSize: 18, fontWeight: "600" }}>{"✓"}</Text>}
          </Pressable>
        );
      })}
    </BottomSheet>
  );

  return (
    <>
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
            <Text style={s.label}>{labelOf(opt)}</Text>
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
            <Text style={s.label}>{labelOf(opt)}</Text>
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
        <Pressable
          style={s.row}
          onPress={() => { setLangQuery(""); setSubSheetOpen(true); }}
          disabled={!editedProfile}
          accessibilityRole="button"
          accessibilityLabel={t("auth.settings.playback.subtitleLanguage")}
        >
          <Text style={s.label}>{langLabel(subtitleLang)}</Text>
          <Text style={s.check}>{"\u203a"}</Text>
        </Pressable>
      </View>

      {/* Default Audio Language */}
      <Text style={s.sectionTitle}>{t("auth.settings.playback.audioLanguage")}</Text>
      <View style={s.section}>
        <Pressable
          style={s.row}
          onPress={() => { setLangQuery(""); setAudSheetOpen(true); }}
          disabled={!editedProfile}
          accessibilityRole="button"
          accessibilityLabel={t("auth.settings.playback.audioLanguage")}
        >
          <Text style={s.label}>{langLabel(audioLang)}</Text>
          <Text style={s.check}>{"\u203a"}</Text>
        </Pressable>
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

      {/* "Still watching?": only meaningful while episodes follow on their own. */}
      {autoplayNext && (
        <>
          <Text style={s.sectionTitle}>{t("auth.settings.playback.stillWatchingTitle", "Ask if I am still watching")}</Text>
          <View style={s.hint}>
            <Text style={s.hintText}>{t("auth.settings.playback.stillWatchingDesc", "After this many episodes in a row with no touch, the next one waits for you.")}</Text>
          </View>
          <View style={s.section}>
            {STILL_WATCHING_STEPS.map((n) => (
              <Pressable
                key={n}
                style={s.row}
                onPress={() => void handleStillWatching(n)}
                disabled={!editedProfile || updatePreference.isPending}
                accessibilityRole="radio"
                accessibilityState={{ selected: stillWatchingAfter === n, disabled: !editedProfile || updatePreference.isPending }}
              >
                <Text style={s.label}>
                  {n === 0 ? t("auth.settings.playback.stillWatchingNever", "Never") : t("auth.settings.playback.stillWatchingAfter", "After {{count}} episodes", { count: n })}
                </Text>
                {stillWatchingAfter === n && <Text style={s.check}>{"\u2713"}</Text>}
              </Pressable>
            ))}
          </View>
        </>
      )}

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
              {t("auth.settings.playback.allowDirectStream", "Allow remux")}
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
                "Fall back to transcoding when neither Direct Play nor remux is possible.",
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
    {renderLangSheet(subSheetOpen, () => setSubSheetOpen(false), subtitleLang, handleSubtitleLang, t("auth.settings.playback.subtitleLanguage"))}
    {renderLangSheet(audSheetOpen, () => setAudSheetOpen(false), audioLang, handleAudioLang, t("auth.settings.playback.audioLanguage"))}
    </>
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
    langSearch: { paddingHorizontal: 12, paddingVertical: 10, fontSize: 14, marginBottom: 8 },
    langSheetRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingVertical: 14, borderBottomWidth: StyleSheet.hairlineWidth },
    sublabel: { color: t.colors.textMuted, fontSize: 12, marginTop: 2 },
    check: { color: t.colors.accent, fontSize: 16, fontWeight: "600", flexShrink: 0 },
  });
