import { View, Text, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { episodeLabel as sharedEpisodeLabel, type V1MediaItem } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import { FONT_SIZE } from "@evey/theme";

interface NextEpisodeOverlayProps {
  visible: boolean;
  ended: boolean;
  countdown: number;
  /** Past the "still watching?" threshold: the card waits for the viewer. */
  stillWatching?: boolean;
  nextEpisode: V1MediaItem | null | undefined;
  onPlayNow: () => void;
  onCancel: () => void;
}

export function NextEpisodeOverlay({ visible, ended, countdown, stillWatching = false, nextEpisode, onPlayNow, onCancel }: NextEpisodeOverlayProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  if (!visible || !nextEpisode) return null;

  // The series is obvious while its next episode is announced: code and title.
  const episodeLine =
    nextEpisode.indexNumber != null
      ? sharedEpisodeLabel({
          seasonNumber: nextEpisode.seasonIndexNumber,
          episodeNumber: nextEpisode.indexNumber,
          title: nextEpisode.title,
        }) || t("player.nextEpisodeOverlay.episode", { number: nextEpisode.indexNumber })
      : [t("player.nextEpisodeOverlay.nextEpisode", "Next episode"), nextEpisode.title].filter(Boolean).join(" · ");

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.surface, borderColor: theme.colors.border }]}>
      <Text style={[styles.heading, { color: theme.colors.textMuted }]}>
        {stillWatching
          ? t("player.nextEpisodeOverlay.stillWatching", "Still watching?")
          : ended
            ? t("player.nextEpisodeOverlay.nextIn", { seconds: Math.max(countdown, 0) })
            : t("player.nextEpisodeOverlay.upNext", "Up next")}
      </Text>
      {episodeLine ? (
        <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={1}>
          {episodeLine}
        </Text>
      ) : null}
      <View style={styles.actions}>
        <FocusableCard
          hasTVPreferredFocus={true}
          onPress={onPlayNow}
          accessibilityLabel={t("player.nextEpisodeOverlay.playNow", "Play now")}
          style={[styles.button, { backgroundColor: theme.colors.accent }]}
        >
          <Text style={[styles.playText, { color: theme.colors.onAccent }]}>{t("player.nextEpisodeOverlay.playNow", "Play now")}</Text>
        </FocusableCard>
        <FocusableCard
          onPress={onCancel}
          accessibilityLabel={t("common.cancel", "Cancel")}
          style={[styles.button, { backgroundColor: theme.colors.bg }]}
        >
          <Text style={[styles.cancelText, { color: theme.colors.text }]}>{t("common.cancel", "Cancel")}</Text>
        </FocusableCard>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    position: "absolute",
    bottom: 60,
    right: 40,
    width: 420,
    borderRadius: 16,
    borderWidth: 1,
    padding: 24,
  },
  heading: {
    fontSize: FONT_SIZE.sm,
    fontWeight: "600",
    textTransform: "uppercase",
    letterSpacing: 1,
  },
  title: {
    fontSize: FONT_SIZE.xl,
    fontWeight: "700",
    marginTop: 4,
    marginBottom: 16,
  },
  actions: {
    flexDirection: "row",
    gap: 10,
  },
  button: {
    flex: 1,
    paddingVertical: 9,
    alignItems: "center",
    justifyContent: "center",
  },
  playText: {
    fontWeight: "700",
    fontSize: FONT_SIZE.base,
  },
  cancelText: {
    fontWeight: "600",
    fontSize: FONT_SIZE.base,
  },
});
