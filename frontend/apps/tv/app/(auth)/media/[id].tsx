import { useCallback, useEffect, useState } from "react";
import { View, Text, ScrollView, Pressable, FlatList, ActivityIndicator, StyleSheet, Dimensions } from "react-native";
import { useLocalSearchParams, router } from "expo-router";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
} from "@evey/api";
import type { V1MediaItem } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useIsFavorite, useToggleFavorite } from "../../../src/hooks/useFavorite";
import { FocusableCard } from "../../../src/components/FocusableCard";

const { width: SCREEN_WIDTH } = Dimensions.get("window");

function formatRuntime(ms?: string): string {
  if (!ms) return "";
  const totalMin = Math.floor(Number(ms) / 60000);
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return h > 0 ? `${h}h ${m}m` : `${m}m`;
}

function formatProgress(ms?: string): string {
  if (!ms) return "";
  const totalSec = Math.floor(Number(ms) / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function isSeriesType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SERIES";
}

function isSeasonType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SEASON";
}

export default function MediaDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const [selectedSeason, setSelectedSeason] = useState<string | null>(null);
  const [focusedBtn, setFocusedBtn] = useState<string | null>(null);

  const { data: media, isLoading } = useLibraryServiceGetMediaItem(id ?? "");
  const { data: watchProgress } = usePlaybackServiceGetWatchProgress(id ?? "");
  const profileId = useAuthStore((s) => s.profileId);
  const qc = useQueryClient();

  const { data: isFavorite } = useIsFavorite(id);
  const toggleFavorite = useToggleFavorite(id);

  const [watchedOverride, setWatchedOverride] = useState<boolean | null>(null);
  const watched = watchedOverride ?? (watchProgress?.played === true);

  const invalidateWatched = useCallback(() => {
    if (id) void qc.invalidateQueries({ queryKey: [`/api/v1/playback/${id}/progress`] });
    void qc.invalidateQueries({ queryKey: ["/api/v1/playback/history"] });
    void qc.invalidateQueries({ queryKey: ["/api/v1/feeds/resume-playback"] });
  }, [id, qc]);

  const markWatched = usePlaybackServiceMarkAsWatched({
    mutation: {
      onSuccess: () => { setWatchedOverride(true); invalidateWatched(); },
    },
  });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({
    mutation: {
      onSuccess: () => { setWatchedOverride(false); invalidateWatched(); },
    },
  });

  const isSeries = isSeriesType(media?.type);
  const isSeason = isSeasonType(media?.type);

  const { data: childrenData } = useLibraryServiceListMediaItems(
    { parentId: id, "pagination.pageSize": 100 },
    { query: { enabled: !!(id && (isSeries || isSeason)) } },
  );

  const children = childrenData?.items ?? [];

  useEffect(() => {
    if (isSeries && children.length > 0 && !selectedSeason) {
      setSelectedSeason(children[0]?.id ?? null);
    }
  }, [isSeries, children, selectedSeason]);

  const { data: episodesData } = useLibraryServiceListMediaItems(
    { parentId: selectedSeason ?? "", "pagination.pageSize": 100 },
    { query: { enabled: !!(selectedSeason && isSeries) } },
  );

  const episodes = isSeason ? children : (episodesData?.items ?? []);
  const seasons = isSeries ? children : [];

  const handlePlay = useCallback(() => {
    if (!id) return;
    if (isSeries && episodes.length > 0) {
      router.push(`/(auth)/player/${episodes[0]!.id}`);
    } else if (isSeason && episodes.length > 0) {
      router.push(`/(auth)/player/${episodes[0]!.id}`);
    } else {
      router.push(`/(auth)/player/${id}`);
    }
  }, [id, isSeries, isSeason, episodes]);

  if (isLoading || !media) {
    return (
      <View style={[styles.center, { backgroundColor: theme.colors.bg }]}>
        <ActivityIndicator color={theme.colors.accent} size="large" />
      </View>
    );
  }

  const meta = [media.year, formatRuntime(media.durationMs), media.officialRating]
    .filter(Boolean)
    .join("  \u2022  ");

  const resumeMs = watchProgress?.positionMs;
  const hasProgress = resumeMs && Number(resumeMs) > 0 && !watchProgress?.played;

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ padding: 48 }}>
      {/* Title area */}
      <Text style={[styles.title, { color: theme.colors.text }]}>{media.title}</Text>
      {meta ? <Text style={[styles.meta, { color: theme.colors.textMuted }]}>{meta}</Text> : null}
      {media.genres && media.genres.length > 0 && (
        <Text style={[styles.genres, { color: theme.colors.textMuted }]}>{media.genres.join(", ")}</Text>
      )}

      {/* Action buttons */}
      <View style={styles.actions}>
        <Pressable
          onPress={handlePlay}
          onFocus={() => setFocusedBtn("play")}
          onBlur={() => setFocusedBtn(null)}
          hasTVPreferredFocus={true}
          accessible={true}
          accessibilityLabel={hasProgress ? "Resume" : "Play"}
          accessibilityRole="button"
          style={[
            styles.playButton,
            {
              backgroundColor: theme.colors.accent,
              borderColor: focusedBtn === "play" ? "#fff" : "transparent",
            },
          ]}
        >
          <Text style={styles.playText}>
            {hasProgress ? `Resume from ${formatProgress(resumeMs)}` : "Play"}
          </Text>
        </Pressable>

        <Pressable
          onPress={() => toggleFavorite.mutate()}
          disabled={toggleFavorite.isPending}
          onFocus={() => setFocusedBtn("fav")}
          onBlur={() => setFocusedBtn(null)}
          accessible={true}
          accessibilityLabel={isFavorite ? "Remove favorite" : "Add favorite"}
          accessibilityRole="button"
          style={[
            styles.actionBtn,
            {
              backgroundColor: theme.colors.surface,
              borderColor: focusedBtn === "fav" ? theme.colors.accent : "transparent",
            },
          ]}
        >
          <Text style={[styles.actionText, { color: isFavorite ? theme.colors.accent : theme.colors.text }]}>
            {isFavorite ? "Favorited" : "Favorite"}
          </Text>
        </Pressable>

        {!isSeries && !isSeason && (
          <Pressable
            onPress={() => {
              if (!profileId) return;
              if (watched) {
                markUnwatched.mutate({ mediaItemId: id!, params: { profileId } });
              } else {
                markWatched.mutate({ mediaItemId: id!, params: { profileId } });
              }
            }}
            disabled={markWatched.isPending || markUnwatched.isPending}
            onFocus={() => setFocusedBtn("watched")}
            onBlur={() => setFocusedBtn(null)}
            accessible={true}
            accessibilityLabel={watched ? "Mark unwatched" : "Mark watched"}
            accessibilityRole="button"
            style={[
              styles.actionBtn,
              {
                backgroundColor: theme.colors.surface,
                borderColor: focusedBtn === "watched" ? theme.colors.accent : "transparent",
              },
            ]}
          >
            <Text style={[styles.actionText, { color: watched ? theme.colors.accent : theme.colors.text }]}>
              {watched ? "Watched" : "Mark watched"}
            </Text>
          </Pressable>
        )}
      </View>

      {/* Overview */}
      {media.overview ? (
        <Text style={[styles.overview, { color: theme.colors.text }]}>{media.overview}</Text>
      ) : null}

      {/* Season picker */}
      {isSeries && seasons.length > 0 && (
        <View style={styles.seasonRow}>
          <ScrollView horizontal showsHorizontalScrollIndicator={false}>
            {seasons.map((season) => {
              const active = season.id === selectedSeason;
              return (
                <Pressable
                  key={season.id}
                  onPress={() => setSelectedSeason(season.id ?? null)}
                  onFocus={() => setFocusedBtn(`season-${season.id}`)}
                  onBlur={() => setFocusedBtn(null)}
                  style={[
                    styles.seasonPill,
                    {
                      backgroundColor: active ? theme.colors.accent : theme.colors.surface,
                      borderColor: focusedBtn === `season-${season.id}` ? "#fff" : "transparent",
                    },
                  ]}
                >
                  <Text style={{ color: active ? "#fff" : theme.colors.text, fontSize: 18, fontWeight: "600" }}>
                    {season.title}
                  </Text>
                </Pressable>
              );
            })}
          </ScrollView>
        </View>
      )}

      {/* Episodes */}
      {(isSeries || isSeason) && episodes.length > 0 && (
        <View style={{ marginTop: 24 }}>
          <Text style={[styles.sectionTitle, { color: theme.colors.text }]}>Episodes</Text>
          {episodes.map((ep, index) => (
            <FocusableCard
              key={ep.id}
              onPress={() => router.push(`/(auth)/media/${ep.id}`)}
              style={[styles.episodeRow, { borderBottomColor: theme.colors.border }]}
            >
              <View style={{ flex: 1 }}>
                <Text style={{ color: theme.colors.text, fontSize: 20, fontWeight: "600" }} numberOfLines={1}>
                  {ep.indexNumber != null ? `${ep.indexNumber}. ` : ""}{ep.title}
                </Text>
                {ep.durationMs && (
                  <Text style={{ color: theme.colors.textMuted, fontSize: 16, marginTop: 2 }}>
                    {formatRuntime(ep.durationMs)}
                  </Text>
                )}
              </View>
            </FocusableCard>
          ))}
        </View>
      )}

      <View style={{ height: 64 }} />
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  center: { flex: 1, justifyContent: "center", alignItems: "center" },
  title: { fontSize: 42, fontWeight: "bold" },
  meta: { fontSize: 20, marginTop: 8 },
  genres: { fontSize: 18, marginTop: 4 },
  actions: { flexDirection: "row", gap: 16, marginTop: 24 },
  playButton: {
    paddingVertical: 16,
    paddingHorizontal: 48,
    borderRadius: 8,
    borderWidth: 3,
  },
  playText: { color: "#fff", fontWeight: "700", fontSize: 22 },
  actionBtn: {
    paddingVertical: 16,
    paddingHorizontal: 32,
    borderRadius: 8,
    borderWidth: 3,
  },
  actionText: { fontWeight: "600", fontSize: 20 },
  overview: { fontSize: 20, lineHeight: 30, marginTop: 24, maxWidth: 800 },
  seasonRow: { marginTop: 32 },
  seasonPill: {
    paddingHorizontal: 24,
    paddingVertical: 12,
    marginRight: 12,
    borderRadius: 8,
    borderWidth: 3,
  },
  sectionTitle: { fontSize: 28, fontWeight: "600", marginBottom: 12 },
  episodeRow: {
    paddingVertical: 16,
    paddingHorizontal: 16,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
});
