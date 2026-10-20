import { View, Text, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { episodeLabel } from "@evey/api";
import { useTheme } from "../../providers/ThemeProvider";
import { useNextEpisode, COUNTDOWN_SEC } from "../../hooks/useNextEpisode";
import { FONT_SIZE } from "@evey/theme";

interface NextEpisodeOverlayProps {
  nextEp: ReturnType<typeof useNextEpisode>;
  // Distance from the screen bottom, in px. Defaults to the old hardcoded
  // 80 - callers with a taller bottom control bar pass a bigger value so
  // the card doesn't sit on top of the seek bar.
  bottomOffset?: number;
}

export function NextEpisodeOverlay({ nextEp, bottomOffset = 80 }: NextEpisodeOverlayProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  if (!nextEp.nextEpisode) return null;
  const next = nextEp.nextEpisode;
  // The series is obvious while its next episode is announced: code and title.
  const nextLine = next.indexNumber != null
    ? episodeLabel({ seasonNumber: next.seasonIndexNumber, episodeNumber: next.indexNumber, title: next.title }) ||
      t("player.nextEpisodeOverlay.episode", { number: next.indexNumber })
    : [t("player.nextEpisodeOverlay.nextEpisode"), next.title].filter(Boolean).join(" · ");

  return (
    <View style={[styles.nextEpOverlay, { bottom: bottomOffset }]}>
      <Text
        style={styles.nextEpLabel}
        // Assertive: this counts down to an action that happens WITHOUT the
        // user, so a screen reader must interrupt to give them a chance to
        // cancel rather than queue the announcement behind whatever it was
        // reading.
        accessibilityLiveRegion={nextEp.ended ? "assertive" : "polite"}
      >
        {nextEp.stillWatching
          ? t("player.nextEpisodeOverlay.stillWatching")
          : nextEp.ended
            ? t("player.nextEpisodeOverlay.nextIn", { seconds: Math.max(nextEp.countdown, 0) })
            : t("player.nextEpisodeOverlay.upNext")}
      </Text>
      <Text style={styles.nextEpTitle} numberOfLines={1}>
        {nextLine}
      </Text>
      <View style={styles.nextEpButtons}>
        <Pressable
          onPress={nextEp.playNow}
          accessibilityRole="button"
          accessibilityLabel={t("player.nextEpisodeOverlay.playNow")}
          style={[styles.nextEpPlayBtn, { backgroundColor: theme.colors.accent }]}
        >
          <Text style={styles.nextEpPlayText}>{t("player.nextEpisodeOverlay.playNow")}</Text>
        </Pressable>
        <Pressable
          onPress={nextEp.cancel}
          accessibilityRole="button"
          // The cancel button has a 5s countdown behind it. Without an explicit
          // role a screen reader announces its child Text as static content, so
          // a TalkBack user cannot find and activate it before autoplay fires.
          accessibilityLabel={t("common.cancel")}
          style={styles.nextEpCancelBtn}
        >
          <Text style={styles.nextEpCancelText}>{t("common.cancel")}</Text>
        </Pressable>
      </View>
      {nextEp.ended && !nextEp.stillWatching && (
        <View style={styles.nextEpProgressBg}>
          <View
            style={[
              styles.nextEpProgressFill,
              { width: `${((COUNTDOWN_SEC - Math.max(nextEp.countdown, 0)) / COUNTDOWN_SEC) * 100}%`, backgroundColor: theme.colors.accent },
            ]}
          />
        </View>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  nextEpOverlay: {
    position: "absolute",
    right: 16,
    backgroundColor: "rgba(0,0,0,0.85)",
    borderRadius: 12,
    padding: 14,
    maxWidth: 260,
    borderWidth: 1,
    borderColor: "rgba(255,255,255,0.15)",
  },
  nextEpLabel: {
    color: "rgba(255,255,255,0.6)",
    fontSize: FONT_SIZE.xs,
    textTransform: "uppercase",
    letterSpacing: 1,
    marginBottom: 4,
  },
  nextEpTitle: {
    color: "#fff",
    fontSize: FONT_SIZE.sm,
    fontWeight: "600",
    marginBottom: 10,
  },
  nextEpButtons: {
    flexDirection: "row",
    gap: 8,
  },
  nextEpPlayBtn: {
    paddingHorizontal: 14,
    paddingVertical: 6,
    borderRadius: 6,
  },
  nextEpPlayText: {
    color: "#000",
    fontSize: FONT_SIZE.xs,
    fontWeight: "600",
  },
  nextEpCancelBtn: {
    backgroundColor: "rgba(255,255,255,0.15)",
    paddingHorizontal: 14,
    paddingVertical: 6,
    borderRadius: 6,
  },
  nextEpCancelText: {
    color: "#fff",
    fontSize: FONT_SIZE.xs,
    fontWeight: "600",
  },
  nextEpProgressBg: {
    marginTop: 8,
    height: 3,
    borderRadius: 2,
    backgroundColor: "rgba(255,255,255,0.2)",
    overflow: "hidden",
  },
  nextEpProgressFill: {
    height: "100%",
    borderRadius: 2,
  },
});
