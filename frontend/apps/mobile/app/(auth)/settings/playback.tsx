import { useState, useEffect } from "react";
import { View, Text, ScrollView, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  useUserServiceGetCurrentUser,
  useUserServiceListProfiles,
  getUserServiceListProfilesQueryKey,
  eveyFetch,
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

export default function PlaybackSettingsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();

  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", {
    query: { enabled: !!user?.id },
  });

  const profiles = profilesResp?.profiles;
  const defaultProfile = profiles?.find((p: any) => p.isDefault) ?? profiles?.[0];

  const [bitrate, setBitrate] = useState("0");
  const [subtitleLang, setSubtitleLang] = useState("");
  const [audioLang, setAudioLang] = useState("");

  useEffect(() => {
    const prefs = (defaultProfile as any)?.preferences;
    if (prefs?.max_bitrate_kbps) setBitrate(prefs.max_bitrate_kbps);
    if (prefs?.default_subtitle_language) setSubtitleLang(prefs.default_subtitle_language);
    if (prefs?.default_audio_language) setAudioLang(prefs.default_audio_language);
  }, [defaultProfile]);

  const updateProfile = useMutation({
    mutationFn: (bitrateKbps: string) =>
      eveyFetch({
        url: `/api/v1/profiles/${defaultProfile?.id}`,
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        data: {
          preferences: {
            ...(defaultProfile as any)?.preferences,
            max_bitrate_kbps: bitrateKbps,
          },
        },
      }),
    onSuccess: () => {
      if (user?.id) {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
      }
    },
  });

  const updatePreference = useMutation({
    mutationFn: (prefs: Record<string, string>) =>
      eveyFetch({
        url: `/api/v1/profiles/${defaultProfile?.id}`,
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        data: {
          preferences: {
            ...(defaultProfile as any)?.preferences,
            ...prefs,
          },
        },
      }),
    onSuccess: () => {
      if (user?.id) {
        void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(user.id) });
      }
    },
  });

  function handleSelect(value: string) {
    setBitrate(value);
    updateProfile.mutate(value);
  }

  function handleSubtitleLang(lang: string) {
    setSubtitleLang(lang);
    updatePreference.mutate({ default_subtitle_language: lang });
  }

  function handleAudioLang(lang: string) {
    setAudioLang(lang);
    updatePreference.mutate({ default_audio_language: lang });
  }

  const s = styles(theme);

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

      <Text style={s.sectionTitle}>{t("auth.settings.playback.bitrateTitle")}</Text>
      <View style={{ marginHorizontal: theme.spacing.md, marginBottom: theme.spacing.sm }}>
        <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>
          {t("auth.settings.playback.bitrateDesc")}
        </Text>
      </View>
      <View style={s.section}>
        {BITRATE_OPTIONS.map((opt) => (
          <Pressable
            key={opt.value}
            style={s.row}
            onPress={() => handleSelect(opt.value)}
            disabled={!defaultProfile || updateProfile.isPending}
          >
            <Text style={{ color: theme.colors.text, fontSize: 15 }}>{opt.label}</Text>
            {bitrate === opt.value && (
              <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>{"\u2713"}</Text>
            )}
          </Pressable>
        ))}
      </View>

      {/* Default Subtitle Language */}
      <Text style={s.sectionTitle}>{t("auth.settings.playback.subtitleLanguage") || "Default Subtitle Language"}</Text>
      <View style={s.section}>
        {LANG_OPTIONS.map((opt) => (
          <Pressable key={opt.value} style={s.row} onPress={() => handleSubtitleLang(opt.value)} disabled={!defaultProfile}>
            <Text style={{ color: theme.colors.text, fontSize: 15 }}>{opt.label}</Text>
            {subtitleLang === opt.value && <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {/* Default Audio Language */}
      <Text style={s.sectionTitle}>{t("auth.settings.playback.audioLanguage") || "Default Audio Language"}</Text>
      <View style={s.section}>
        {LANG_OPTIONS.map((opt) => (
          <Pressable key={opt.value} style={s.row} onPress={() => handleAudioLang(opt.value)} disabled={!defaultProfile}>
            <Text style={{ color: theme.colors.text, fontSize: 15 }}>{opt.label}</Text>
            {audioLang === opt.value && <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>{"\u2713"}</Text>}
          </Pressable>
        ))}
      </View>

      {(updateProfile.isPending || updatePreference.isPending) && (
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
  });
