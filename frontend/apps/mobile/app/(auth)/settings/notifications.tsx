import { useState, useEffect } from "react";
import { View, Text, ScrollView, Pressable, Alert, ActivityIndicator, StyleSheet } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useNotificationServiceGetNotificationSettings,
  useNotificationServiceUpdateNotificationSettings,
  apiErrorMessage,
} from "@evey/api";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { LoadFailed } from "../../../src/components/LoadFailed";

type NotificationLevel =
  | "NOTIFICATION_LEVEL_UNSPECIFIED"
  | "NOTIFICATION_LEVEL_DISABLED"
  | "NOTIFICATION_LEVEL_ALL"
  | "NOTIFICATION_LEVEL_RELEVANT"
  | "NOTIFICATION_LEVEL_HIGHLY_RELEVANT";

const LEVELS: NotificationLevel[] = [
  "NOTIFICATION_LEVEL_DISABLED",
  "NOTIFICATION_LEVEL_HIGHLY_RELEVANT",
  "NOTIFICATION_LEVEL_RELEVANT",
  "NOTIFICATION_LEVEL_ALL",
];

const LEVEL_LABEL_KEYS: Record<NotificationLevel, string> = {
  NOTIFICATION_LEVEL_UNSPECIFIED: "auth.notificationSettings.levelDefault",
  NOTIFICATION_LEVEL_DISABLED: "auth.notificationSettings.levelDisabled",
  NOTIFICATION_LEVEL_ALL: "auth.notificationSettings.levelAll",
  NOTIFICATION_LEVEL_RELEVANT: "auth.notificationSettings.levelRelevant",
  NOTIFICATION_LEVEL_HIGHLY_RELEVANT: "auth.notificationSettings.levelImportant",
};

function LevelPicker({
  label,
  value,
  onChange,
  theme,
}: {
  label: string;
  value: NotificationLevel;
  onChange: (v: NotificationLevel) => void;
  theme: ReturnType<typeof useTheme>;
}) {
  const { t } = useTranslation();
  return (
    <View>
      <Text style={{ color: theme.colors.text, fontSize: 15, fontWeight: "600", marginBottom: 8 }}>
        {label}
      </Text>
      {LEVELS.map((level) => (
        <Pressable
          key={level}
          onPress={() => onChange(level)}
          style={{
            flexDirection: "row",
            alignItems: "center",
            justifyContent: "space-between",
            paddingVertical: 12,
            paddingHorizontal: theme.spacing.md,
            borderBottomWidth: StyleSheet.hairlineWidth,
            borderBottomColor: theme.colors.border,
          }}
        >
          <Text style={{ color: theme.colors.text, fontSize: 15 }}>
            {t(LEVEL_LABEL_KEYS[level])}
          </Text>
          {value === level && (
            <Text style={{ color: theme.colors.accent, fontSize: 16, fontWeight: "600" }}>{"\u2713"}</Text>
          )}
        </Pressable>
      ))}
    </View>
  );
}

export default function NotificationSettingsScreen() {
  const theme = useTheme();
  const { t } = useTranslation();

  const { data: settings, isLoading, isError, refetch } = useNotificationServiceGetNotificationSettings();

  const [movieLevel, setMovieLevel] = useState<NotificationLevel>("NOTIFICATION_LEVEL_UNSPECIFIED");
  const [seriesLevel, setSeriesLevel] = useState<NotificationLevel>("NOTIFICATION_LEVEL_UNSPECIFIED");
  const [musicLevel, setMusicLevel] = useState<NotificationLevel>("NOTIFICATION_LEVEL_UNSPECIFIED");

  useEffect(() => {
    if (settings) {
      if (settings.movieLevel) setMovieLevel(settings.movieLevel as NotificationLevel);
      if (settings.seriesLevel) setSeriesLevel(settings.seriesLevel as NotificationLevel);
      if (settings.musicLevel) setMusicLevel(settings.musicLevel as NotificationLevel);
    }
  }, [settings]);

  const update = useNotificationServiceUpdateNotificationSettings({
    mutation: {
      onSuccess: () => Alert.alert(t("auth.notificationSettings.saved")),
      onError: (err: Error) => Alert.alert(t("auth.notificationSettings.saveError"), apiErrorMessage(err, t, t("auth.notificationSettings.saveError"))),
    },
  });

  function handleSave() {
    // A failed GET leaves the three levels on their UNSPECIFIED default;
    // saving those would overwrite the real settings already on the server.
    if (isError) return;
    update.mutate({ data: { movieLevel, seriesLevel, musicLevel } });
  }

  const s = styles(theme);

  return (
    <ScrollView style={s.container} contentContainerStyle={{ paddingBottom: 40 }}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("auth.settings.tabs.notifications"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      {isLoading ? (
        <View style={{ padding: theme.spacing.xl, alignItems: "center" }}>
          <ActivityIndicator color={theme.colors.accent} />
        </View>
      ) : isError ? (
        <LoadFailed onRetry={() => void refetch()} />
      ) : (
        <>
          <Text style={s.sectionTitle}>{t("auth.notificationSettings.movies")}</Text>
          <View style={s.section}>
            <LevelPicker label="" value={movieLevel} onChange={setMovieLevel} theme={theme} />
          </View>

          <Text style={s.sectionTitle}>{t("auth.notificationSettings.series")}</Text>
          <View style={s.section}>
            <LevelPicker label="" value={seriesLevel} onChange={setSeriesLevel} theme={theme} />
          </View>

          <Text style={s.sectionTitle}>{t("auth.notificationSettings.music")}</Text>
          <View style={s.section}>
            <LevelPicker label="" value={musicLevel} onChange={setMusicLevel} theme={theme} />
          </View>

          <View style={{ marginHorizontal: theme.spacing.md, marginTop: theme.spacing.lg }}>
            <Pressable
              onPress={handleSave}
              disabled={update.isPending}
              style={{
                backgroundColor: theme.colors.accent,
                borderRadius: theme.radii.md,
                paddingVertical: 14,
                alignItems: "center",
              }}
            >
              {update.isPending ? (
                <ActivityIndicator color={theme.colors.onAccent} size="small" />
              ) : (
                <Text style={{ color: theme.colors.onAccent, fontWeight: "600", fontSize: 16 }}>
                  {t("common.save")}
                </Text>
              )}
            </Pressable>
          </View>
        </>
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
  });
