import { Text, Pressable, ScrollView, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { seasonLabel } from "@evey/api";

export interface EpisodeNavItem {
  id: string;
  title?: string;
  indexNumber?: number;
}

export interface SeasonNavItem {
  id: string;
  title?: string;
  indexNumber?: number;
}

interface PlayerEpisodesPanelProps {
  // Only rendered when there's more than one season (mirrors the web
  // EpisodesPanel), so a movie or a single-season show gets no selector.
  seasons: SeasonNavItem[];
  currentSeasonId: string;
  episodes: EpisodeNavItem[];
  currentEpisodeId: string;
  onSeasonChange: (id: string) => void;
  onEpisodeChange: (id: string) => void;
}

// Content of the player's Episodes BottomSheet: an optional season chip row
// on top of the current episode list, web parity's PlayerEpisodesPanel.tsx.
export function PlayerEpisodesPanel({
  seasons,
  currentSeasonId,
  episodes,
  currentEpisodeId,
  onSeasonChange,
  onEpisodeChange,
}: PlayerEpisodesPanelProps) {
  const { t } = useTranslation();
  const theme = useTheme();

  return (
    <>
      {seasons.length > 1 && (
        <ScrollView
          horizontal
          showsHorizontalScrollIndicator={false}
          style={styles.seasonRow}
          contentContainerStyle={styles.seasonRowContent}
        >
          {seasons.map((s) => {
            const active = s.id === currentSeasonId;
            return (
              <Pressable
                key={s.id}
                onPress={() => onSeasonChange(s.id)}
                style={[styles.chip, { backgroundColor: active ? theme.colors.accent : theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}
                accessible={true}
                accessibilityLabel={seasonLabel(s, t)}
                accessibilityRole="button"
                accessibilityState={{ selected: active }}
              >
                <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: FONT_SIZE.sm }}>
                  {seasonLabel(s, t)}
                </Text>
              </Pressable>
            );
          })}
        </ScrollView>
      )}
      {episodes.map((ep) => {
        const isActive = ep.id === currentEpisodeId;
        return (
          <Pressable
            key={ep.id}
            style={[styles.episodeRow, { borderBottomColor: theme.colors.border }]}
            onPress={() => onEpisodeChange(ep.id)}
            accessible={true}
            accessibilityLabel={ep.title || `E${ep.indexNumber ?? "?"}`}
            accessibilityRole="button"
            accessibilityState={{ selected: isActive }}
          >
            <Text style={[styles.episodeIndex, { color: theme.colors.textMuted }]}>
              E{ep.indexNumber ?? "?"}
            </Text>
            <Text
              style={[styles.episodeTitle, { color: isActive ? theme.colors.accent : theme.colors.text }]}
              numberOfLines={1}
            >
              {ep.title}
            </Text>
            {isActive && <Ionicons name="checkmark" size={ICON_SIZE.sm} color={theme.colors.accent} />}
          </Pressable>
        );
      })}
    </>
  );
}

const styles = StyleSheet.create({
  seasonRow: {
    flexGrow: 0,
    marginBottom: 8,
  },
  seasonRowContent: {
    flexDirection: "row",
    paddingVertical: 4,
  },
  chip: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    marginRight: 6,
  },
  episodeRow: {
    flexDirection: "row",
    alignItems: "center",
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
    gap: 10,
  },
  episodeIndex: {
    fontSize: FONT_SIZE.xs,
    fontVariant: ["tabular-nums"],
    width: 32,
  },
  episodeTitle: {
    fontSize: FONT_SIZE.sm,
    flex: 1,
  },
});
