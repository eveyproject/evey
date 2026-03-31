import { useEffect, useState, useCallback } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  Dimensions,
  FlatList,
  ActivityIndicator,
  StyleSheet,
} from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { Skeleton } from "../../../src/components/Skeleton";
import { useLocalSearchParams, router } from "expo-router";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
} from "@evey/api";
import type { V1MediaItem, V1MediaPerson } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { artworkUrl } from "../../../src/lib/artwork";
import { useIsFavorite, useToggleFavorite } from "../../../src/hooks/useFavorite";

const { width: SCREEN_WIDTH, height: SCREEN_HEIGHT } = Dimensions.get("window");
const HERO_HEIGHT = SCREEN_HEIGHT * 0.4;

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

function CastCard({ person, serverUrl }: { person: V1MediaPerson; serverUrl: string }) {
  const theme = useTheme();
  return (
    <View style={{ width: 80, marginRight: theme.spacing.sm, alignItems: "center" }}>
      {person.thumbUrl ? (
        <Image
          source={{ uri: person.thumbUrl }}
          style={{ width: 64, height: 64, borderRadius: 32, backgroundColor: theme.colors.surface }}
          contentFit="cover"
          transition={200}
        />
      ) : (
        <View style={{ width: 64, height: 64, borderRadius: 32, backgroundColor: theme.colors.surface, justifyContent: "center", alignItems: "center" }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 24 }}>?</Text>
        </View>
      )}
      <Text style={{ color: theme.colors.text, fontSize: 11, marginTop: 4, textAlign: "center" }} numberOfLines={1}>
        {person.name}
      </Text>
      {person.role ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 10, textAlign: "center" }} numberOfLines={1}>
          {person.role}
        </Text>
      ) : null}
    </View>
  );
}

function EpisodeRow({ item, serverUrl, theme, authHeaders }: { item: V1MediaItem; serverUrl: string; theme: ReturnType<typeof useTheme>; authHeaders?: { Authorization: string } }) {
  const episodeUri = item.id ? artworkUrl(serverUrl, item.id, "primary") : undefined;
  return (
    <Pressable
      onPress={() => router.push(`/(auth)/media/${item.id}`)}
      style={{
        flexDirection: "row",
        padding: theme.spacing.sm,
        borderBottomWidth: StyleSheet.hairlineWidth,
        borderBottomColor: theme.colors.border,
      }}
    >
      <Image
        source={episodeUri ? { uri: episodeUri, headers: authHeaders } : undefined}
        style={{ width: 120, height: 68, borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface }}
        contentFit="cover"
        transition={200}
      />
      <View style={{ flex: 1, marginLeft: theme.spacing.sm, justifyContent: "center" }}>
        <Text style={{ color: theme.colors.text, fontSize: 14, fontWeight: "600" }} numberOfLines={1}>
          {item.indexNumber != null ? `${item.indexNumber}. ` : ""}{item.title}
        </Text>
        {item.durationMs ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }}>
            {formatRuntime(item.durationMs)}
          </Text>
        ) : null}
        {item.overview ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: 2 }} numberOfLines={2}>
            {item.overview}
          </Text>
        ) : null}
      </View>
    </Pressable>
  );
}

export default function MediaDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const [selectedSeason, setSelectedSeason] = useState<string | null>(null);

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

  // Fetch seasons for series
  const isSeries = isSeriesType(media?.type);
  const isSeason = isSeasonType(media?.type);

  const { data: childrenData } = useLibraryServiceListMediaItems(
    { parentId: id, "pagination.pageSize": 100 },
    { query: { enabled: !!(id && (isSeries || isSeason)) } },
  );

  const children = childrenData?.items ?? [];

  // For series: list seasons; auto-select first
  useEffect(() => {
    if (isSeries && children.length > 0 && !selectedSeason) {
      setSelectedSeason(children[0]?.id ?? null);
    }
  }, [isSeries, children, selectedSeason]);

  // Fetch episodes for selected season
  const { data: episodesData } = useLibraryServiceListMediaItems(
    { parentId: selectedSeason ?? "", "pagination.pageSize": 100 },
    { query: { enabled: !!(selectedSeason && isSeries) } },
  );

  const episodes = isSeason ? children : (episodesData?.items ?? []);
  const seasons = isSeries ? children : [];

  const people = media?.people ?? [];
  const actors = people.filter((p) => p.type === "PERSON_TYPE_ACTOR" || p.type === "PERSON_TYPE_GUEST_STAR");

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
      <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
        <Skeleton width={SCREEN_WIDTH} height={HERO_HEIGHT} borderRadius={0} />
        <View style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.md }}>
          <Skeleton width={SCREEN_WIDTH * 0.6} height={24} borderRadius={theme.radii.sm} />
          <Skeleton width={SCREEN_WIDTH * 0.4} height={14} borderRadius={theme.radii.sm} style={{ marginTop: theme.spacing.sm }} />
          <Skeleton width={SCREEN_WIDTH - theme.spacing.md * 2} height={44} borderRadius={theme.radii.md} style={{ marginTop: theme.spacing.md }} />
          <Skeleton width={SCREEN_WIDTH - theme.spacing.md * 2} height={60} borderRadius={theme.radii.sm} style={{ marginTop: theme.spacing.md }} />
        </View>
      </View>
    );
  }

  const meta = [media.year, formatRuntime(media.durationMs), media.officialRating]
    .filter(Boolean)
    .join("  \u2022  ");

  const resumeMs = watchProgress?.positionMs;
  const hasProgress = resumeMs && Number(resumeMs) > 0 && !watchProgress?.played;

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      {/* Hero backdrop */}
      <View style={{ width: SCREEN_WIDTH, height: HERO_HEIGHT }}>
        {serverUrl && id ? (
          <Image
            source={{ uri: artworkUrl(serverUrl, id, "backdrop"), headers: authHeaders }}
            style={{ width: SCREEN_WIDTH, height: HERO_HEIGHT }}
            contentFit="cover"
            transition={300}
          />
        ) : (
          <View style={{ width: SCREEN_WIDTH, height: HERO_HEIGHT, backgroundColor: theme.colors.surface }} />
        )}
        {/* Gradient overlay (stacked opacity layers) */}
        <View style={{ position: "absolute", bottom: 0, left: 0, right: 0, height: HERO_HEIGHT * 0.5 }}>
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.2 }} />
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.5 }} />
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.8 }} />
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 1 }} />
        </View>
      </View>

      {/* Content */}
      <View style={{ paddingHorizontal: theme.spacing.md, marginTop: -theme.spacing.xl }}>
        {/* Title */}
        <Text style={{ color: theme.colors.text, fontSize: 24, fontWeight: "bold" }}>
          {media.title}
        </Text>

        {/* Meta line */}
        {meta ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: theme.spacing.xs }}>
            {meta}
          </Text>
        ) : null}

        {/* Genres */}
        {media.genres && media.genres.length > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 12, marginTop: theme.spacing.xs }}>
            {media.genres.join(", ")}
          </Text>
        ) : null}

        {/* Play button */}
        <Pressable
          onPress={handlePlay}
          style={{
            backgroundColor: theme.colors.accent,
            borderRadius: theme.radii.md,
            paddingVertical: theme.spacing.sm + 4,
            alignItems: "center",
            marginTop: theme.spacing.md,
          }}
        >
          <Text style={{ color: "#fff", fontWeight: "700", fontSize: 16 }}>
            {hasProgress ? `Resume from ${formatProgress(resumeMs)}` : "Play"}
          </Text>
        </Pressable>

        {/* Action buttons */}
        <View style={{ flexDirection: "row", gap: theme.spacing.md, marginTop: theme.spacing.md }}>
          {/* Favorite toggle */}
          <Pressable
            onPress={() => toggleFavorite.mutate()}
            disabled={toggleFavorite.isPending}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
              opacity: toggleFavorite.isPending ? 0.5 : 1,
            }}
          >
            <Ionicons
              name={isFavorite ? "heart" : "heart-outline"}
              size={20}
              color={isFavorite ? theme.colors.accent : theme.colors.text}
            />
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
              {isFavorite ? "Favorited" : "Favorite"}
            </Text>
          </Pressable>

          {/* Watched toggle */}
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
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
                opacity: (markWatched.isPending || markUnwatched.isPending) ? 0.5 : 1,
              }}
            >
              {(markWatched.isPending || markUnwatched.isPending) ? (
                <ActivityIndicator size={18} color={theme.colors.text} />
              ) : (
                <Ionicons
                  name={watched ? "checkmark-circle" : "checkmark-circle-outline"}
                  size={20}
                  color={watched ? theme.colors.accent : theme.colors.text}
                />
              )}
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {watched ? "Watched" : "Mark watched"}
              </Text>
            </Pressable>
          )}
        </View>

        {/* Overview */}
        {media.overview ? (
          <Text style={{ color: theme.colors.text, fontSize: 14, lineHeight: 20, marginTop: theme.spacing.md }}>
            {media.overview}
          </Text>
        ) : null}

        {/* Tagline */}
        {media.tagline ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, fontStyle: "italic", marginTop: theme.spacing.sm }}>
            {media.tagline}
          </Text>
        ) : null}

        {/* Season picker for series */}
        {isSeries && seasons.length > 0 ? (
          <View style={{ marginTop: theme.spacing.lg }}>
            <ScrollView horizontal showsHorizontalScrollIndicator={false}>
              {seasons.map((season) => {
                const active = season.id === selectedSeason;
                return (
                  <Pressable
                    key={season.id}
                    onPress={() => setSelectedSeason(season.id ?? null)}
                    style={{
                      paddingHorizontal: theme.spacing.md,
                      paddingVertical: theme.spacing.sm,
                      marginRight: theme.spacing.xs,
                      borderRadius: theme.radii.md,
                      backgroundColor: active ? theme.colors.accent : theme.colors.surface,
                    }}
                  >
                    <Text style={{ color: active ? "#fff" : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                      {season.title}
                    </Text>
                  </Pressable>
                );
              })}
            </ScrollView>
          </View>
        ) : null}

        {/* Episodes list */}
        {(isSeries || isSeason) && episodes.length > 0 ? (
          <View style={{ marginTop: theme.spacing.md }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
              Episodes
            </Text>
            {episodes.map((ep) => (
              <EpisodeRow key={ep.id} item={ep} serverUrl={serverUrl} theme={theme} authHeaders={authHeaders} />
            ))}
          </View>
        ) : null}

        {/* Cast */}
        {actors.length > 0 ? (
          <View style={{ marginTop: theme.spacing.lg }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
              Cast
            </Text>
            <FlatList
              horizontal
              data={actors}
              keyExtractor={(p) => p.id ?? p.name ?? ""}
              renderItem={({ item }) => <CastCard person={item} serverUrl={serverUrl} />}
              showsHorizontalScrollIndicator={false}
            />
          </View>
        ) : null}

        {/* Bottom spacing */}
        <View style={{ height: theme.spacing.xl * 2 }} />
      </View>
    </ScrollView>
  );
}
