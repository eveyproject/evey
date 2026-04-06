import { useEffect, useState, useCallback, useRef } from "react";
import {
  View,
  Text,
  ScrollView,
  Pressable,
  FlatList,
  ActivityIndicator,
  StyleSheet,
  Linking,
  Dimensions,
} from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { Skeleton } from "../../../src/components/Skeleton";
import { ProgressBar } from "../../../src/components/ProgressBar";
import { MediaCard } from "../../../src/components/MediaCard";
import { useLocalSearchParams, router } from "expo-router";
import { useSafeAreaInsets } from "react-native-safe-area-context";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetMediaItem,
  useLibraryServiceListMediaItems,
  usePlaybackServiceGetWatchProgress,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
  useFeedServiceSimilarItems,
} from "@evey/api";
import type { V1MediaItem, V1MediaPerson, V1Trailer } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { artworkUrl } from "../../../src/lib/artwork";
import { useIsFavorite, useToggleFavorite } from "../../../src/hooks/useFavorite";
import { startDownload, getDownloadPath } from "../../../src/lib/downloads";
import { toast } from "@evey/store";
import { AddToPlaylistSheet } from "../../../src/components/AddToPlaylistSheet";
import { useParentalGate } from "../../../src/hooks/useParentalGate";
import { ParentalPINModal } from "../../../src/components/ParentalPINModal";

const POSTER_WIDTH = 100;
const POSTER_HEIGHT = POSTER_WIDTH * 1.5;

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

function formatFileSize(bytes?: string): string {
  if (!bytes) return "";
  const n = parseInt(bytes, 10);
  if (isNaN(n)) return "";
  if (n >= 1_073_741_824) return `${(n / 1_073_741_824).toFixed(1)} GB`;
  if (n >= 1_048_576) return `${(n / 1_048_576).toFixed(0)} MB`;
  return `${(n / 1024).toFixed(0)} KB`;
}

function isSeriesType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SERIES";
}

function isSeasonType(type?: string) {
  return type === "MEDIA_ITEM_TYPE_SEASON";
}

function StarIcons({ rating }: { rating: number }) {
  const theme = useTheme();
  // rating out of 10, show 5 stars
  const filled = Math.round(rating / 2);
  return (
    <View style={{ flexDirection: "row", gap: 1 }}>
      {[1, 2, 3, 4, 5].map((i) => (
        <Ionicons
          key={i}
          name={i <= filled ? "star" : "star-outline"}
          size={12}
          color={i <= filled ? "#f5c518" : theme.colors.textMuted}
        />
      ))}
    </View>
  );
}

function CastCard({ person }: { person: V1MediaPerson }) {
  const theme = useTheme();
  const personType = person.type?.replace("PERSON_TYPE_", "") ?? "";
  const roleLabel = personType.charAt(0) + personType.slice(1).toLowerCase();

  return (
    <Pressable
      onPress={() => person.id && router.push(`/(auth)/person/${person.id}`)}
      style={{ width: 80, marginRight: theme.spacing.sm, alignItems: "center" }}
      accessible={true}
      accessibilityLabel={person.name ?? "Person"}
      accessibilityRole="button"
    >
      {person.thumbUrl ? (
        <Image
          source={{ uri: person.thumbUrl }}
          style={{ width: 64, height: 64, borderRadius: 32, backgroundColor: theme.colors.surface }}
          contentFit="cover"
          transition={200}
        />
      ) : (
        <View style={{ width: 64, height: 64, borderRadius: 32, backgroundColor: theme.colors.surface, justifyContent: "center", alignItems: "center" }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 24 }}>
            {person.name ? person.name[0].toUpperCase() : "?"}
          </Text>
        </View>
      )}
      <Text style={{ color: theme.colors.text, fontSize: 11, marginTop: 4, textAlign: "center" }} numberOfLines={1}>
        {person.name}
      </Text>
      {person.role ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 10, textAlign: "center" }} numberOfLines={1}>
          {person.role}
        </Text>
      ) : personType && personType !== "ACTOR" ? (
        <Text style={{ color: theme.colors.textMuted, fontSize: 10, textAlign: "center" }} numberOfLines={1}>
          {roleLabel}
        </Text>
      ) : null}
    </Pressable>
  );
}

function EpisodeRow({ item, serverUrl, theme, authHeaders }: { item: V1MediaItem; serverUrl: string; theme: ReturnType<typeof useTheme>; authHeaders?: { Authorization: string } }) {
  const episodeUri = item.id ? artworkUrl(serverUrl, item.id, "poster") : undefined;
  const { data: progress } = usePlaybackServiceGetWatchProgress(item.id ?? "", undefined, {
    query: { enabled: !!item.id, retry: false },
  });
  const posMs = Number(progress?.positionMs ?? 0);
  const durMs = Number(progress?.durationMs ?? 0);
  const percent = durMs > 0 ? (posMs / durMs) * 100 : 0;

  return (
    <Pressable
      onPress={() => item.id && router.push(`/(auth)/player/${item.id}`)}
      style={{
        flexDirection: "row",
        padding: theme.spacing.sm,
        borderBottomWidth: StyleSheet.hairlineWidth,
        borderBottomColor: theme.colors.border,
      }}
      accessible={true}
      accessibilityLabel={item.title ?? "Episode"}
      accessibilityRole="button"
    >
      <View style={{ width: 120, height: 68, borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface, overflow: "hidden" }}>
        <Image
          source={episodeUri ? { uri: episodeUri, headers: authHeaders } : undefined}
          style={{ width: 120, height: 68 }}
          contentFit="cover"
          transition={200}
        />
        {percent > 0 && (
          <View style={{ position: "absolute", bottom: 0, left: 0, right: 0, padding: 2 }}>
            <ProgressBar percent={percent} />
          </View>
        )}
      </View>
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

function TrailerButton({ trailer }: { trailer: V1Trailer }) {
  const { t } = useTranslation();
  const theme = useTheme();

  const url = (() => {
    if (!trailer.key || !trailer.site) return null;
    switch (trailer.site.toLowerCase()) {
      case "youtube":
        return `https://www.youtube.com/watch?v=${trailer.key}`;
      case "vimeo":
        return `https://vimeo.com/${trailer.key}`;
      default:
        return null;
    }
  })();

  if (!url) return null;

  return (
    <Pressable
      onPress={() => Linking.openURL(url)}
      style={{
        flexDirection: "row",
        alignItems: "center",
        gap: 6,
        paddingHorizontal: 12,
        paddingVertical: 8,
        borderRadius: theme.radii.sm,
        backgroundColor: theme.colors.surface,
        borderWidth: StyleSheet.hairlineWidth,
        borderColor: theme.colors.border,
      }}
      accessible={true}
      accessibilityLabel={t("media.detail.trailer")}
      accessibilityRole="link"
    >
      <Ionicons name="play" size={14} color={theme.colors.text} />
      <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "500" }}>
        {t("media.detail.trailer")}
      </Text>
    </Pressable>
  );
}

function SimilarSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const screenW = Dimensions.get("window").width;
  const { data: similarData } = useFeedServiceSimilarItems(item.id ?? "");
  const firstGenre = item.genres?.[0];
  const hasSimilar = (similarData?.items?.length ?? 0) > 0;
  const { data: byGenreData } = useLibraryServiceListMediaItems(
    { genre: firstGenre, type: item.type as never },
    { query: { enabled: !hasSimilar && !!firstGenre } } as never,
  );

  const fromFeed = (similarData?.items ?? [])
    .filter((fi: any) => !!fi.mediaItemId)
    .map((fi: any) => ({ id: fi.mediaItemId!, title: fi.title }));

  const fromGenre = (byGenreData?.items ?? [])
    .filter((m) => m.id && m.id !== item.id)
    .map((m) => ({ id: m.id!, title: m.title, year: m.year, type: m.type }));

  const cards: V1MediaItem[] = (fromFeed.length ? fromFeed : fromGenre) as V1MediaItem[];
  if (!cards.length) return null;

  return (
    <View style={{ marginTop: theme.spacing.lg }}>
      <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
        {t("media.detail.similarTitles")}
      </Text>
      <FlatList
        horizontal
        data={cards}
        keyExtractor={(c) => c.id ?? ""}
        renderItem={({ item: c }) => (
          <MediaCard
            item={c}
            onPress={() => c.id && router.push(`/(auth)/media/${c.id}`)}
            width={screenW * 0.28}
          />
        )}
        showsHorizontalScrollIndicator={false}
      />
    </View>
  );
}

function FileInfoSection({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const theme = useTheme();
  const [open, setOpen] = useState(false);

  if (!item.filePath && !item.fileSize && !item.containerFormat && !(item.streams && item.streams.length > 0)) {
    return null;
  }

  return (
    <View style={{ marginTop: theme.spacing.lg }}>
      <Pressable
        onPress={() => setOpen((v) => !v)}
        style={{ flexDirection: "row", alignItems: "center", gap: 6, marginBottom: theme.spacing.sm }}
        accessible={true}
        accessibilityLabel={t("media.detail.fileInfo")}
        accessibilityRole="button"
      >
        <Ionicons
          name={open ? "chevron-down" : "chevron-forward"}
          size={16}
          color={theme.colors.textMuted}
        />
        <Text style={{ color: theme.colors.textMuted, fontSize: 14, fontWeight: "600" }}>
          {t("media.detail.fileInfo")}
        </Text>
      </Pressable>
      {open && (
        <View style={{ gap: 8 }}>
          {item.filePath ? (
            <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontFamily: "monospace" }} numberOfLines={2}>
              {item.filePath}
            </Text>
          ) : null}
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8 }}>
            {item.fileSize ? <InfoBadge label={formatFileSize(item.fileSize)} /> : null}
            {item.containerFormat ? <InfoBadge label={item.containerFormat.toUpperCase()} /> : null}
            {item.bitrate ? <InfoBadge label={`${Math.round(parseInt(item.bitrate) / 1000)} kbps`} /> : null}
            {item.durationMs ? <InfoBadge label={formatRuntime(item.durationMs)} /> : null}
          </View>
          {item.streams && item.streams.length > 0 ? (
            <View style={{ gap: 4, marginTop: 4 }}>
              {item.streams.map((s, i) => {
                const typeLabel = s.type?.replace("MEDIA_STREAM_TYPE_", "") ?? "";
                const res = s.width && s.height ? `${s.width}x${s.height}` : undefined;
                return (
                  <View key={i} style={{ flexDirection: "row", gap: 8 }}>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "600", width: 60 }}>
                      {typeLabel}
                    </Text>
                    <Text style={{ color: theme.colors.textMuted, fontSize: 11, flex: 1 }} numberOfLines={1}>
                      {[s.codec, s.language, s.title, res].filter(Boolean).join(" - ")}
                    </Text>
                  </View>
                );
              })}
            </View>
          ) : null}
        </View>
      )}
    </View>
  );
}

function InfoBadge({ label }: { label: string }) {
  const theme = useTheme();
  return (
    <View style={{
      paddingHorizontal: 8,
      paddingVertical: 3,
      borderRadius: theme.radii.sm,
      backgroundColor: theme.colors.surface,
    }}>
      <Text style={{ color: theme.colors.textMuted, fontSize: 11, fontWeight: "500" }}>{label}</Text>
    </View>
  );
}

export default function MediaDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const { t } = useTranslation();
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const insets = useSafeAreaInsets();
  const [screenW, setScreenW] = useState(() => Dimensions.get("window").width);
  const [screenH, setScreenH] = useState(() => Dimensions.get("window").height);
  const dimsTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => {
    const sub = Dimensions.addEventListener("change", ({ window }) => {
      if (dimsTimer.current) clearTimeout(dimsTimer.current);
      dimsTimer.current = setTimeout(() => {
        setScreenW(window.width);
        setScreenH(window.height);
      }, 300);
    });
    return () => { sub.remove(); if (dimsTimer.current) clearTimeout(dimsTimer.current); };
  }, []);
  const isLandscape = screenW > screenH;
  const heroHeight = isLandscape ? screenH * 0.55 : screenH * 0.4;
  const [selectedSeason, setSelectedSeason] = useState<string | null>(null);

  const { data: media, isLoading } = useLibraryServiceGetMediaItem(id ?? "");

  const { data: watchProgress } = usePlaybackServiceGetWatchProgress(id ?? "", undefined, {
    query: { enabled: !!id, retry: false },
  });
  const profileId = useAuthStore((s) => s.profileId);
  const qc = useQueryClient();

  const { data: isFavorite } = useIsFavorite(id);
  const toggleFavorite = useToggleFavorite(id);

  const [playlistSheetOpen, setPlaylistSheetOpen] = useState(false);
  const [watchedOverride, setWatchedOverride] = useState<boolean | null>(null);
  const watched = watchedOverride ?? (watchProgress?.played === true);

  const { needsPin, profileId: gateProfileId } = useParentalGate(media?.officialRating);
  const [showPinModal, setShowPinModal] = useState(false);
  const [pendingPlayTarget, setPendingPlayTarget] = useState<string | null>(null);

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
      const sorted = [...children].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0));
      setSelectedSeason(sorted[0]?.id ?? null);
    }
  }, [isSeries, children, selectedSeason]);

  const { data: episodesData } = useLibraryServiceListMediaItems(
    { parentId: selectedSeason ?? "", "pagination.pageSize": 100 },
    { query: { enabled: !!(selectedSeason && isSeries) } },
  );

  const episodes = [...(isSeason ? children : (episodesData?.items ?? []))].sort(
    (a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0),
  );
  const seasons = isSeries
    ? [...children].sort((a, b) => (a.indexNumber ?? 0) - (b.indexNumber ?? 0))
    : [];

  const people = media?.people ?? [];
  const actors = people.filter((p) => p.type === "PERSON_TYPE_ACTOR" || p.type === "PERSON_TYPE_GUEST_STAR");
  const directors = people.filter((p) => p.type === "PERSON_TYPE_DIRECTOR");
  const writers = people.filter((p) => p.type === "PERSON_TYPE_WRITER");
  const producers = people.filter((p) => p.type === "PERSON_TYPE_PRODUCER");
  const crew = [...directors, ...writers, ...producers];

  const getPlayTarget = useCallback((): string | null => {
    if (!id) return null;
    if (isSeries && episodes.length > 0) return episodes[0]!.id!;
    if (isSeason && episodes.length > 0) return episodes[0]!.id!;
    return id;
  }, [id, isSeries, isSeason, episodes]);

  const handlePlay = useCallback(() => {
    const target = getPlayTarget();
    if (!target) return;
    if (needsPin()) {
      setPendingPlayTarget(target);
      setShowPinModal(true);
      return;
    }
    router.push(`/(auth)/player/${target}`);
  }, [getPlayTarget, needsPin]);

  const handlePinVerified = useCallback(() => {
    setShowPinModal(false);
    if (pendingPlayTarget) {
      router.push(`/(auth)/player/${pendingPlayTarget}`);
      setPendingPlayTarget(null);
    }
  }, [pendingPlayTarget]);

  if (isLoading || !media) {
    return (
      <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
        <Skeleton width={screenW} height={heroHeight} borderRadius={0} />
        <View style={{ paddingHorizontal: theme.spacing.md, marginTop: theme.spacing.md }}>
          <Skeleton width={screenW * 0.6} height={24} borderRadius={theme.radii.sm} />
          <Skeleton width={screenW * 0.4} height={14} borderRadius={theme.radii.sm} style={{ marginTop: theme.spacing.sm }} />
          <Skeleton width={screenW - theme.spacing.md * 2} height={44} borderRadius={theme.radii.md} style={{ marginTop: theme.spacing.md }} />
          <Skeleton width={screenW - theme.spacing.md * 2} height={60} borderRadius={theme.radii.sm} style={{ marginTop: theme.spacing.md }} />
        </View>
      </View>
    );
  }

  const resumeMs = watchProgress?.positionMs;
  const hasProgress = resumeMs && Number(resumeMs) > 0 && !watchProgress?.played;

  const trailers = media.trailers ?? [];
  const sortedTrailers = [...trailers].sort((a, b) => {
    if (a.official && !b.official) return -1;
    if (!a.official && b.official) return 1;
    return 0;
  }).slice(0, 3);

  return (
    <ScrollView style={{ flex: 1, backgroundColor: theme.colors.bg }} contentContainerStyle={{ paddingLeft: insets.left, paddingRight: insets.right, paddingBottom: insets.bottom }}>
      {/* Hero backdrop */}
      <View style={{ width: "100%", height: heroHeight }}>
        {serverUrl && id ? (
          <Image
            source={{ uri: artworkUrl(serverUrl, id, "backdrop"), headers: authHeaders }}
            style={{ width: "100%", height: "100%" }}
            contentFit="cover"
            transition={300}
          />
        ) : (
          <View style={{ width: "100%", height: "100%", backgroundColor: theme.colors.surface }} />
        )}
        {/* Gradient overlay (stacked opacity layers) */}
        <View style={{ position: "absolute", bottom: 0, left: 0, right: 0, height: heroHeight * 0.6 }}>
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.2 }} />
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.5 }} />
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 0.8 }} />
          <View style={{ flex: 1, backgroundColor: theme.colors.bg, opacity: 1 }} />
        </View>
        {/* Back button */}
        <Pressable
          onPress={() => router.back()}
          hitSlop={12}
          accessible={true}
          accessibilityLabel={t("common.back")}
          accessibilityRole="button"
          style={{ position: "absolute", top: insets.top + 8, left: insets.left + 8, width: 36, height: 36, borderRadius: 18, backgroundColor: "rgba(0,0,0,0.5)", justifyContent: "center", alignItems: "center" }}
        >
          <Ionicons name="chevron-back" size={22} color="#fff" />
        </Pressable>
      </View>

      {/* Content overlapping hero */}
      <View style={{ paddingHorizontal: theme.spacing.md, marginTop: -POSTER_HEIGHT * 0.5, ...(isLandscape ? { flexDirection: "row", gap: theme.spacing.lg } : {}) }}>
        {/* Left column (landscape) / full width (portrait) */}
        <View style={isLandscape ? { flex: 3 } : {}}>
        {/* Poster + title row */}
        <View style={{ flexDirection: "row", gap: theme.spacing.md }}>
          {serverUrl && id ? (
            <Image
              source={{ uri: artworkUrl(serverUrl, id, "poster"), headers: authHeaders }}
              style={{
                width: POSTER_WIDTH,
                height: POSTER_HEIGHT,
                borderRadius: theme.radii.md,
                backgroundColor: theme.colors.surface,
              }}
              contentFit="cover"
              transition={200}
            />
          ) : (
            <View style={{
              width: POSTER_WIDTH,
              height: POSTER_HEIGHT,
              borderRadius: theme.radii.md,
              backgroundColor: theme.colors.surface,
              justifyContent: "center",
              alignItems: "center",
            }}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 28, fontWeight: "bold" }}>
                {media.title?.[0]?.toUpperCase() ?? "?"}
              </Text>
            </View>
          )}

          <View style={{ flex: 1, justifyContent: "flex-end" }}>
            <Text style={{ color: theme.colors.text, fontSize: 22, fontWeight: "bold" }}>
              {media.title}
            </Text>

            {/* Meta: year, duration, rating */}
            <View style={{ flexDirection: "row", flexWrap: "wrap", alignItems: "center", gap: 6, marginTop: 4 }}>
              {media.year ? (
                <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{media.year}</Text>
              ) : null}
              {media.durationMs ? (
                <Text style={{ color: theme.colors.textMuted, fontSize: 13 }}>{formatRuntime(media.durationMs)}</Text>
              ) : null}
              {media.officialRating ? (
                <View style={{
                  borderWidth: 1,
                  borderColor: theme.colors.textMuted,
                  borderRadius: theme.radii.sm,
                  paddingHorizontal: 5,
                  paddingVertical: 1,
                }}>
                  <Text style={{ color: theme.colors.text, fontSize: 11, fontWeight: "500" }}>
                    {media.officialRating}
                  </Text>
                </View>
              ) : null}
            </View>

            {/* Community rating stars */}
            {media.communityRating != null && media.communityRating > 0 ? (
              <View style={{ flexDirection: "row", alignItems: "center", gap: 4, marginTop: 4 }}>
                <StarIcons rating={media.communityRating} />
                <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                  {media.communityRating.toFixed(1)}
                </Text>
              </View>
            ) : null}

            {/* Provider badges: IMDb, TMDB */}
            <View style={{ flexDirection: "row", gap: 6, marginTop: 4 }}>
              {media.providerIds?.imdb ? (
                <Pressable onPress={() => Linking.openURL(`https://www.imdb.com/title/${media.providerIds!.imdb}/`)} accessible={true} accessibilityLabel="IMDb" accessibilityRole="link">
                  <View style={{ backgroundColor: "#f5c518", borderRadius: 3, paddingHorizontal: 5, paddingVertical: 2 }}>
                    <Text style={{ color: "#000", fontSize: 10, fontWeight: "800" }}>IMDb</Text>
                  </View>
                </Pressable>
              ) : null}
              {media.providerIds?.tmdb ? (
                <Pressable onPress={() => Linking.openURL(`https://www.themoviedb.org/${media.type === "MEDIA_ITEM_TYPE_MOVIE" ? "movie" : "tv"}/${media.providerIds!.tmdb}`)} accessible={true} accessibilityLabel="TMDB" accessibilityRole="link">
                  <View style={{ backgroundColor: "#01b4e4", borderRadius: 3, paddingHorizontal: 5, paddingVertical: 2 }}>
                    <Text style={{ color: "#fff", fontSize: 10, fontWeight: "800" }}>TMDB</Text>
                  </View>
                </Pressable>
              ) : null}
            </View>
          </View>
        </View>

        {/* Genres */}
        {media.genres && media.genres.length > 0 ? (
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 6, marginTop: theme.spacing.sm }}>
            {media.genres.map((g) => (
              <View key={g} style={{
                paddingHorizontal: 10,
                paddingVertical: 4,
                borderRadius: 16,
                backgroundColor: theme.colors.surface,
              }}>
                <Text style={{ color: theme.colors.text, fontSize: 12, fontWeight: "500" }}>{g}</Text>
              </View>
            ))}
          </View>
        ) : null}

        {/* Play / Resume button */}
        <Pressable
          onPress={handlePlay}
          style={{
            backgroundColor: theme.colors.accent,
            borderRadius: theme.radii.md,
            paddingVertical: theme.spacing.sm + 4,
            flexDirection: "row",
            alignItems: "center",
            justifyContent: "center",
            gap: 8,
            marginTop: theme.spacing.md,
          }}
          accessible={true}
          accessibilityLabel={hasProgress ? t("media.detail.resume") : t("media.detail.play")}
          accessibilityRole="button"
        >
          <Ionicons name="play" size={18} color="#fff" />
          <Text style={{ color: "#fff", fontWeight: "700", fontSize: 16 }}>
            {hasProgress
              ? `${t("media.detail.resume")} ${formatProgress(resumeMs)}`
              : t("media.detail.play")}
          </Text>
        </Pressable>

        {/* Action row: favorite, watched, playlist */}
        <View style={{ flexDirection: "row", gap: theme.spacing.sm, marginTop: theme.spacing.md, flexWrap: "wrap" }}>
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
            accessible={true}
            accessibilityLabel={isFavorite ? t("media.detail.favorited") : t("media.detail.favorite")}
            accessibilityRole="button"
          >
            <Ionicons
              name={isFavorite ? "heart" : "heart-outline"}
              size={20}
              color={isFavorite ? theme.colors.accent : theme.colors.text}
            />
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
              {isFavorite ? t("media.detail.favorited") : t("media.detail.favorite")}
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
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
                opacity: (markWatched.isPending || markUnwatched.isPending) ? 0.5 : 1,
              }}
              accessible={true}
              accessibilityLabel={watched ? t("media.detail.markUnwatched") : t("media.detail.markWatched")}
              accessibilityRole="button"
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
                {watched ? t("media.detail.markUnwatched") : t("media.detail.markWatched")}
              </Text>
            </Pressable>
          )}

          <Pressable
            onPress={() => setPlaylistSheetOpen(true)}
            style={{
              flexDirection: "row",
              alignItems: "center",
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
              paddingVertical: theme.spacing.sm,
              paddingHorizontal: theme.spacing.md,
            }}
            accessible={true}
            accessibilityLabel={t("media.detail.addToPlaylist")}
            accessibilityRole="button"
          >
            <Ionicons name="add" size={20} color={theme.colors.text} />
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
              {t("media.detail.addToPlaylist")}
            </Text>
          </Pressable>
          {!isSeries && !isSeason && (
            <Pressable
              onPress={async () => {
                if (!id || !media?.title) return;
                const existing = await getDownloadPath(id);
                if (existing) { toast.info(t("media.detail.alreadyDownloaded")); return; }
                toast.info(t("media.detail.downloadStarted"));
                try {
                  await startDownload(id, media.title);
                  toast.success(t("media.detail.downloadComplete"));
                } catch { toast.error(t("media.detail.downloadFailed")); }
              }}
              style={{
                flexDirection: "row",
                alignItems: "center",
                backgroundColor: theme.colors.surface,
                borderRadius: theme.radii.md,
                paddingVertical: theme.spacing.sm,
                paddingHorizontal: theme.spacing.md,
              }}
              accessible={true}
              accessibilityLabel={t("common.download") || "Download"}
              accessibilityRole="button"
            >
              <Ionicons name="download-outline" size={20} color={theme.colors.text} />
              <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600", marginLeft: 6 }}>
                {t("common.download") || "Download"}
              </Text>
            </Pressable>
          )}
        </View>
        </View>{/* end left column */}

        {/* Right column (landscape) / continues below (portrait) */}
        <View style={isLandscape ? { flex: 7 } : {}}>
        {/* Tagline */}
        {media.tagline ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, fontStyle: "italic", marginTop: theme.spacing.md }}>
            {media.tagline}
          </Text>
        ) : null}

        {/* Overview */}
        {media.overview ? (
          <Text style={{ color: theme.colors.text, fontSize: 14, lineHeight: 20, marginTop: theme.spacing.sm }}>
            {media.overview}
          </Text>
        ) : null}

        {/* Directors */}
        {directors.length > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: theme.spacing.sm }}>
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("media.detail.director")}</Text>
            {directors.map((d) => d.name).join(", ")}
          </Text>
        ) : null}

        {/* Writers */}
        {writers.length > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: 4 }}>
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("media.detail.writer")}</Text>
            {writers.map((w) => w.name).join(", ")}
          </Text>
        ) : null}

        {/* Studios */}
        {media.studios && media.studios.length > 0 ? (
          <Text style={{ color: theme.colors.textMuted, fontSize: 13, marginTop: 4 }}>
            <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("media.detail.studio")}</Text>
            {media.studios.join(", ")}
          </Text>
        ) : null}

        {/* Trailers */}
        {sortedTrailers.length > 0 ? (
          <View style={{ flexDirection: "row", flexWrap: "wrap", gap: 8, marginTop: theme.spacing.md }}>
            {sortedTrailers.map((tr, i) => (
              <TrailerButton key={`${tr.key}-${i}`} trailer={tr} />
            ))}
          </View>
        ) : null}

        {/* Season picker for series */}
        {isSeries && seasons.length > 0 ? (
          <View style={{ marginTop: theme.spacing.lg }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
              {t("media.detail.seasons")}
            </Text>
            <ScrollView horizontal showsHorizontalScrollIndicator={false}>
              {seasons.map((season) => {
                const active = season.id === selectedSeason;
                const label = season.indexNumber != null
                  ? t("media.season", { number: season.indexNumber })
                  : (season.title ?? "");
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
                    accessible={true}
                    accessibilityLabel={label}
                    accessibilityRole="tab"
                  >
                    <Text style={{ color: active ? "#fff" : theme.colors.text, fontSize: 13, fontWeight: "600" }}>
                      {label}
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
              {t("media.detail.episodes")}
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
              {t("media.detail.cast")}
            </Text>
            <FlatList
              horizontal
              data={actors}
              keyExtractor={(p) => p.id ?? p.name ?? ""}
              renderItem={({ item }) => <CastCard person={item} />}
              showsHorizontalScrollIndicator={false}
            />
          </View>
        ) : null}

        {/* Crew */}
        {crew.length > 0 ? (
          <View style={{ marginTop: theme.spacing.lg }}>
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "600", marginBottom: theme.spacing.sm }}>
              {t("media.detail.crew")}
            </Text>
            <FlatList
              horizontal
              data={crew}
              keyExtractor={(p) => p.id ?? p.name ?? ""}
              renderItem={({ item }) => <CastCard person={item} />}
              showsHorizontalScrollIndicator={false}
            />
          </View>
        ) : null}

        {/* Similar content */}
        {id && !isSeason ? <SimilarSection item={media} /> : null}

        {/* File info */}
        <FileInfoSection item={media} />

        {/* Bottom spacing */}
        <View style={{ height: theme.spacing.xl * 2 }} />
        </View>{/* end right column */}
      </View>

      {id ? (
        <AddToPlaylistSheet
          visible={playlistSheetOpen}
          mediaItemId={id}
          onClose={() => setPlaylistSheetOpen(false)}
        />
      ) : null}

      {gateProfileId ? (
        <ParentalPINModal
          visible={showPinModal}
          onClose={() => { setShowPinModal(false); setPendingPlayTarget(null); }}
          onVerified={handlePinVerified}
          profileId={gateProfileId}
        />
      ) : null}
    </ScrollView>
  );
}
