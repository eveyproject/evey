import { View, Text, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { useNextEpisode, COUNTDOWN_SEC } from "../../hooks/useNextEpisode";
import { FONT_SIZE } from "@evey/theme";

interface NextEpisodeOverlayProps {
  nextEp: ReturnType<typeof useNextEpisode>;
}

export function NextEpisodeOverlay({ nextEp }: NextEpisodeOverlayProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  if (!nextEp.nextEpisode) return null;
  return (
    <View style={styles.nextEpOverlay}>
      <Text style={styles.nextEpLabel}>
        {nextEp.ended
          ? t("player.nextEpisodeOverlay.nextIn", { seconds: Math.max(nextEp.countdown, 0) })
          : t("player.nextEpisodeOverlay.upNext")}
      </Text>
      <Text style={styles.nextEpTitle} numberOfLines={1}>
        {nextEp.nextEpisode.indexNumber != null
          ? t("player.nextEpisodeOverlay.episode", { number: nextEp.nextEpisode.indexNumber })
          : t("player.nextEpisodeOverlay.nextEpisode")}
        {nextEp.nextEpisode.title ? ` - ${nextEp.nextEpisode.title}` : ""}
      </Text>
      <View style={styles.nextEpButtons}>
        <Pressable onPress={nextEp.playNow} style={[styles.nextEpPlayBtn, { backgroundColor: theme.colors.accent }]}>
          <Text style={styles.nextEpPlayText}>{t("player.nextEpisodeOverlay.playNow")}</Text>
        </Pressable>
        <Pressable onPress={nextEp.cancel} style={styles.nextEpCancelBtn}>
          <Text style={styles.nextEpCancelText}>{t("common.cancel")}</Text>
        </Pressable>
      </View>
      {nextEp.ended && (
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
    bottom: 80,
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
