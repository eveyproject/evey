import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  View,
  Text,
  FlatList,
  TouchableOpacity,
  Pressable,
  TextInput,
  ScrollView,
  ActivityIndicator,
  Dimensions,
  StyleSheet,
  Modal,
} from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { Skeleton } from "../../../src/components/Skeleton";
import { useLocalSearchParams, useRouter, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetLibrary,
  useLibraryServiceListMediaItems,
  useLibraryServiceSearchMedia,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
  usePlaybackServiceGetWatchHistory,
  collectionServiceAddCollectionItem,
  libraryServiceListMediaItems,
} from "@evey/api";
import type {
  V1MediaItem,
  V1MediaStream,
  LibraryServiceListMediaItemsType,
  LibraryServiceListMediaItemsSortField,
  LibraryServiceListMediaItemsSortOrder,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { useBulkFavorite } from "../../../src/hooks/useFavorite";
import { AddToPlaylistSheet } from "../../../src/components/AddToPlaylistSheet";
import { MediaCard } from "../../../src/components/MediaCard";
import { useCollections } from "../../../src/api/collections";
import { artworkUrl } from "../../../src/lib/artwork";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";

const NUM_COLUMNS = 3;
const SPACING = 8;
const screenWidth = Dimensions.get("window").width;
const ITEM_WIDTH = (screenWidth - SPACING * (NUM_COLUMNS + 1)) / NUM_COLUMNS;
const POSTER_HEIGHT = ITEM_WIDTH * 1.5;
const ROW_HEIGHT = POSTER_HEIGHT + 30 + SPACING;
const PAGE_SIZE = 30;

const SORT_OPTIONS: Array<{
  labelKey: string;
  value: LibraryServiceListMediaItemsSortField;
}> = [
  { labelKey: "media.library.titleSort", value: "MEDIA_SORT_FIELD_TITLE" },
  { labelKey: "media.library.year", value: "MEDIA_SORT_FIELD_YEAR" },
  { labelKey: "media.library.ratingSort", value: "MEDIA_SORT_FIELD_RATING" },
  { labelKey: "media.library.dateAdded", value: "MEDIA_SORT_FIELD_DATE_ADDED" },
];

const SHOW_TYPE_OPTIONS: Array<{
  labelKey: string;
  value: LibraryServiceListMediaItemsType;
}> = [
  { labelKey: "media.library.series", value: "MEDIA_ITEM_TYPE_SERIES" },
  { labelKey: "media.library.seasons", value: "MEDIA_ITEM_TYPE_SEASON" },
  { labelKey: "media.library.episodes", value: "MEDIA_ITEM_TYPE_EPISODE" },
];

const COMMON_GENRES = [
  "Action",
  "Adventure",
  "Animation",
  "Comedy",
  "Crime",
  "Documentary",
  "Drama",
  "Fantasy",
  "Horror",
  "Mystery",
  "Romance",
  "Sci-Fi",
  "Thriller",
  "Western",
];

function libraryKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

const libraryGetItemLayout = (_: unknown, index: number) => ({
  length: ROW_HEIGHT,
  offset: ROW_HEIGHT * Math.floor(index / NUM_COLUMNS),
  index,
});

function getPoster(item: V1MediaItem, serverUrl: string | undefined): string | undefined {
  return serverUrl && item.id ? artworkUrl(serverUrl, item.id, "poster", 300) : undefined;
}

function formatType(type?: string): string {
  if (type === "MEDIA_ITEM_TYPE_MOVIE") return "Movie";
  if (type === "MEDIA_ITEM_TYPE_SERIES") return "Series";
  return "";
}

// Mirrors web's streamLabels.ts (frontend/apps/web/src/features/library/streamLabels.ts)
// - kept local since the mobile app has no package shared with web for this.
function videoStream(streams?: V1MediaStream[]): V1MediaStream | undefined {
  return streams?.find((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
}

function isHdrStream(stream?: V1MediaStream): boolean {
  if (!stream) return false;
  const transfer = stream.colorTransfer?.trim().toLowerCase();
  if (transfer === "smpte2084" || transfer === "arib-std-b67") return true;
  const cs = stream.colorSpace?.trim().toLowerCase() ?? "";
  return cs.startsWith("bt2020") && (stream.bitDepth ?? 0) >= 10;
}

function resolutionLabel(stream?: V1MediaStream): string | null {
  const h = stream?.height ?? 0;
  if (h <= 0) return null;
  if (h >= 2160) return "4K";
  if (h >= 1440) return "1440p";
  if (h >= 1080) return "1080p";
  if (h >= 720) return "720p";
  return `${h}p`;
}

export default function LibraryDetailScreen() {
  const { libraryId } = useLocalSearchParams<{ libraryId: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const [page, setPage] = useState(1);
  const [allItems, setAllItems] = useState<V1MediaItem[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const prevDataRef = useRef<unknown>(null);

  const [sortField, setSortField] =
    useState<LibraryServiceListMediaItemsSortField>(
      "MEDIA_SORT_FIELD_TITLE",
    );
  const [sortOrder, setSortOrder] =
    useState<LibraryServiceListMediaItemsSortOrder>("SORT_ORDER_ASC");
  const [genre, setGenre] = useState("");
  const [typeFilter, setTypeFilter] = useState<LibraryServiceListMediaItemsType | "">("");
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);
  const [showFilters, setShowFilters] = useState(false);
  const [yearMin, setYearMin] = useState("");
  const [yearMax, setYearMax] = useState("");
  const [ratingMin, setRatingMin] = useState("");
  const [ratingMax, setRatingMax] = useState("");
  const [viewMode, setViewMode] = useState<"grid" | "list">("grid");
  const [bulkMode, setBulkMode] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const profileId = useAuthStore((s) => s.profileId) ?? "";

  const markWatched = usePlaybackServiceMarkAsWatched({ mutation: { onSuccess: () => toast.success(t("media.library.markedWatched", "Marked watched")) } });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({ mutation: { onSuccess: () => toast.success(t("media.library.markedUnwatched", "Marked unwatched")) } });
  const bulkFavorite = useBulkFavorite();
  const [showBulkPlaylist, setShowBulkPlaylist] = useState(false);
  const [showBulkCollection, setShowBulkCollection] = useState(false);

  const toggleSelect = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      if (next.size === 0) setBulkMode(false);
      return next;
    });
  }, []);

  const handleLongPress = useCallback((id: string) => {
    setBulkMode(true);
    setSelected(new Set([id]));
  }, []);

  const exitBulk = useCallback(() => {
    setBulkMode(false);
    setSelected(new Set());
  }, []);

  const bulkMarkWatched = useCallback(async () => {
    let failed = 0;
    for (const id of selected) {
      try { await markWatched.mutateAsync({ mediaItemId: id, params: { profileId } }); }
      catch { failed++; }
    }
    if (failed > 0) toast.error(t("media.library.bulkPartialFailure", { count: failed, defaultValue: "{{count}} items failed" }));
    exitBulk();
  }, [selected, markWatched, profileId, exitBulk, t]);

  const bulkMarkUnwatched = useCallback(async () => {
    let failed = 0;
    for (const id of selected) {
      try { await markUnwatched.mutateAsync({ mediaItemId: id, params: { profileId } }); }
      catch { failed++; }
    }
    if (failed > 0) toast.error(t("media.library.bulkPartialFailure", { count: failed, defaultValue: "{{count}} items failed" }));
    exitBulk();
  }, [selected, markUnwatched, profileId, exitBulk, t]);

  const bulkToggleFavorite = useCallback(async () => {
    try {
      await bulkFavorite.mutateAsync(Array.from(selected));
      toast.success(t("media.favorites.toggled", "Favorites toggled"));
    } catch {
      toast.error(t("media.favorites.bulkFailed", "Failed to toggle favorites"));
    }
    exitBulk();
  }, [selected, bulkFavorite, exitBulk, t]);

  const authHeaders = useAuthHeaders();
  const serverUrl = useServerUrl();
  const { data: library } = useLibraryServiceGetLibrary(libraryId ?? "");
  const isShowLibrary = library?.type === "LIBRARY_TYPE_SHOWS";

  const { data, isLoading, isError, isFetching, refetch } =
    useLibraryServiceListMediaItems(
      {
        libraryId,
        ...(typeFilter ? { type: typeFilter } : {}),
        ...(genre ? { genre } : {}),
        sortField,
        sortOrder,
        "pagination.page": page,
        "pagination.pageSize": PAGE_SIZE,
      },
      { query: { enabled: !!libraryId } },
    );

  // Server-side title search, scoped to this library, replacing the previous
  // client-side filter over already-loaded pages (which missed anything not
  // in the currently fetched slice). Mirrors web's $libraryId.lazy.tsx.
  const trimmedSearch = debouncedSearch.trim();
  const searchActive = trimmedSearch.length > 0;
  const {
    data: searchData,
    isLoading: isSearchLoading,
    isError: isSearchError,
    refetch: refetchSearch,
  } = useLibraryServiceSearchMedia(
    {
      libraryId,
      query: trimmedSearch,
      ...(typeFilter ? { type: typeFilter } : {}),
      ...(genre ? { genre } : {}),
      ...(yearMin ? { yearMin: parseInt(yearMin) } : {}),
      ...(yearMax ? { yearMax: parseInt(yearMax) } : {}),
      sortField,
      sortOrder,
      limit: 60,
    },
    { query: { enabled: searchActive && !!libraryId } },
  );

  // Watched status + resume progress for the badge/progress-bar overlay
  // MediaCard already supports. Same source and shape as useWatchedMap
  // (src/hooks/useWatchedMap.ts), read directly here so the percent-complete
  // map can be derived from the same single fetch instead of duplicating it.
  const { data: watchHistoryData } = usePlaybackServiceGetWatchHistory(
    { limit: 500 },
    { query: { staleTime: 30_000 } },
  );
  const watchedMap = useMemo(() => {
    const map = new Map<string, boolean>();
    for (const w of watchHistoryData?.items ?? []) {
      if (w.mediaItemId && w.played) map.set(w.mediaItemId, true);
    }
    return map;
  }, [watchHistoryData]);
  const percentMap = useMemo(() => {
    const map = new Map<string, number>();
    for (const w of watchHistoryData?.items ?? []) {
      if (w.mediaItemId && w.percentComplete != null) map.set(w.mediaItemId, w.percentComplete);
    }
    return map;
  }, [watchHistoryData]);

  // Reset pagination when filters/sort change. Declared BEFORE the accumulate
  // effect ON PURPOSE: React runs effects in declaration order, so when a
  // filter flips back to a value whose page-1 is already cached (query resolves
  // synchronously), this clear runs FIRST and the accumulate repopulates
  // SECOND. The reverse order let the accumulate fill allItems and this clear
  // wipe it with no re-run ([data,page] deps unchanged) - a persistent empty
  // grid on the common peek-a-genre-then-back flow over a static library.
  const resetAndRefetch = useCallback(() => {
    setPage(1);
    setAllItems([]);
    setHasMore(true);
    prevDataRef.current = null;
  }, []);

  useEffect(() => {
    resetAndRefetch();
  }, [sortField, sortOrder, genre, typeFilter, resetAndRefetch]);

  useEffect(() => {
    if (!data || data === prevDataRef.current) return;
    prevDataRef.current = data;
    const newItems = data.items ?? [];
    if (page === 1) {
      setAllItems(newItems);
    } else {
      setAllItems((prev) => [...prev, ...newItems]);
    }
    const totalPages = data.pagination?.totalPages ?? 1;
    setHasMore(page < totalPages);
  }, [data, page]);

  const totalItems = searchActive
    ? (searchData?.totalHits ?? 0)
    : (data?.pagination?.totalItems ?? 0);

  // Title search now hits the server (searchData) - only rating stays a pure
  // client-side facet in both paths, since SearchMediaRequest has no rating
  // param. Year is skipped here while searching because it's already sent as
  // a server param above; the regular (non-search) path still filters it
  // client-side since ListMediaItems has no year param.
  const displayItems = useMemo(() => {
    let items = searchActive ? (searchData?.items ?? []) : allItems;
    if (!searchActive) {
      if (yearMin) {
        const min = parseInt(yearMin);
        items = items.filter((item) => (item.year ?? 0) >= min);
      }
      if (yearMax) {
        const max = parseInt(yearMax);
        items = items.filter((item) => (item.year ?? 9999) <= max);
      }
    }
    if (ratingMin) {
      const min = parseFloat(ratingMin);
      items = items.filter((item) => (item.communityRating ?? 0) >= min);
    }
    if (ratingMax) {
      const max = parseFloat(ratingMax);
      items = items.filter((item) => (item.communityRating ?? 10) <= max);
    }
    return items;
  }, [searchActive, searchData, allItems, yearMin, yearMax, ratingMin, ratingMax]);

  // Collect genres from loaded items
  const availableGenres = useMemo(() => {
    const set = new Set<string>();
    for (const item of allItems) {
      for (const g of item.genres ?? []) set.add(g);
    }
    const merged = Array.from(
      new Set([...COMMON_GENRES, ...Array.from(set)]),
    ).sort();
    return merged;
  }, [allItems]);

  const [shuffling, setShuffling] = useState(false);
  // Search results and year/rating facets can't be honored by the random-page
  // probe below (search has no page offset; year/rating have no server
  // param), so whenever any of them is active, shuffle draws from what's
  // actually on screen (displayItems) instead. With no such filter, shuffle
  // the WHOLE library via a random page offset (pageSize 1) instead of just
  // the scrolled-in slice - the genre/type quick-filters, if any, still apply
  // server-side since they're baked into the probe request.
  const hasLocalFilters = Boolean(
    debouncedSearch.trim() || yearMin || yearMax || ratingMin || ratingMax,
  );
  const handleShuffle = useCallback(async () => {
    if (shuffling || !libraryId) return;
    if (hasLocalFilters) {
      if (displayItems.length === 0) { toast.info(t("common.shuffleEmpty")); return; }
      const pick = displayItems[Math.floor(Math.random() * displayItems.length)];
      if (pick?.id) router.push(`/(auth)/player/${pick.id}`);
      else toast.error(t("common.shuffleFailed"));
      return;
    }
    setShuffling(true);
    try {
      const base = { libraryId, ...(typeFilter ? { type: typeFilter } : {}), ...(genre ? { genre } : {}) };
      const probe = await libraryServiceListMediaItems({ ...base, "pagination.page": 1, "pagination.pageSize": 1 });
      const total = probe.pagination?.totalItems ?? 0;
      if (total === 0) { toast.info(t("common.shuffleEmpty")); return; }
      const randomPage = Math.floor(Math.random() * total) + 1;
      const res = randomPage === 1
        ? probe
        : await libraryServiceListMediaItems({ ...base, "pagination.page": randomPage, "pagination.pageSize": 1 });
      const pick = res.items?.[0];
      if (pick?.id) router.push(`/(auth)/player/${pick.id}`);
      else toast.error(t("common.shuffleFailed"));
    } catch {
      toast.error(t("common.shuffleFailed"));
    } finally {
      setShuffling(false);
    }
  }, [shuffling, libraryId, hasLocalFilters, displayItems, typeFilter, genre, router, t]);

  const handleRefresh = useCallback(() => {
    if (searchActive) { void refetchSearch(); return; }
    setPage(1);
    setHasMore(true);
    prevDataRef.current = null;
  }, [searchActive, refetchSearch]);

  const handleEndReached = useCallback(() => {
    // Search returns a single fixed-size page (top-N by relevance), no
    // cursor/offset to page through - so scrolling never advances it.
    if (searchActive) return;
    if (!isFetching && hasMore) {
      setPage((p) => p + 1);
    }
  }, [searchActive, isFetching, hasMore]);

  const renderItem = useCallback(
    ({ item }: { item: V1MediaItem }) => {
      const isSelected = bulkMode && selected.has(item.id ?? "");
      const watched = item.id ? watchedMap.get(item.id) === true : false;
      const watchPercent = item.id ? percentMap.get(item.id) : undefined;
      const hasProgress = watchPercent != null && watchPercent > 0;
      const vStream = videoStream(item.streams);
      const resLabel = resolutionLabel(vStream);
      const hdr = isHdrStream(vStream);
      const rating = item.communityRating ?? 0;
      // MediaCard renders its own Pressable + a marginRight: theme.spacing.sm
      // (== SPACING here) on its outer box, so its `width` prop must be
      // narrowed by that amount for the card's total footprint to still
      // match this cell's declared ITEM_WIDTH - otherwise the 3-column grid
      // overflows to the right by SPACING per column.
      return (
        <View style={{ width: ITEM_WIDTH, marginLeft: SPACING, marginBottom: SPACING }}>
          <View style={{ opacity: bulkMode && !isSelected ? 0.5 : 1 }}>
            <MediaCard
              item={item}
              width={ITEM_WIDTH - SPACING}
              watchPercent={watchPercent}
              watched={watched}
            />
            {rating > 0 && (
              <View
                style={[
                  styles.techBadge,
                  { bottom: hasProgress ? 14 : 6, right: 6, backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm },
                ]}
                pointerEvents="none"
              >
                <Ionicons name="star" size={10} color={theme.colors.warning} />
                <Text style={[styles.techBadgeText, { color: theme.colors.text }]}>
                  {rating.toFixed(1)}
                </Text>
              </View>
            )}
            {(resLabel || hdr) && (
              <View style={[styles.techBadgeRow, { bottom: hasProgress ? 14 : 6, left: 6 }]} pointerEvents="none">
                {resLabel && (
                  <View style={[styles.techBadge, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}>
                    <Text style={[styles.techBadgeText, { color: theme.colors.text }]}>{resLabel}</Text>
                  </View>
                )}
                {hdr && (
                  <View style={[styles.techBadge, { backgroundColor: theme.colors.accent, borderRadius: theme.radii.sm }]}>
                    <Text style={[styles.techBadgeText, { color: theme.colors.onAccent }]}>HDR</Text>
                  </View>
                )}
              </View>
            )}
            {isSelected && (
              <View style={styles.selectCheckmark} pointerEvents="none">
                <Ionicons name="checkmark-circle" size={22} color={theme.colors.accent} />
              </View>
            )}
          </View>
          {/* Transparent, full-cover overlay: MediaCard has its own internal
              Pressable, which would otherwise win the touch responder
              negotiation over this cell's tap-to-open/long-press-to-select
              behavior since it's the deeper view. Placed last (topmost in
              paint order) so it alone captures touches for this cell. */}
          <Pressable
            style={StyleSheet.absoluteFillObject}
            onPress={() => bulkMode ? toggleSelect(item.id ?? "") : router.push(`/media/${item.id}`)}
            onLongPress={() => !bulkMode && handleLongPress(item.id ?? "")}
            accessibilityRole="button"
            accessibilityLabel={item.title}
          />
        </View>
      );
    },
    [theme, router, bulkMode, selected, watchedMap, percentMap, toggleSelect, handleLongPress],
  );

  const renderListItem = useCallback(
    ({ item }: { item: V1MediaItem }) => {
      const poster = getPoster(item, serverUrl);
      const isSelected = bulkMode && selected.has(item.id ?? "");
      return (
        <TouchableOpacity
          style={[
            styles.listRow,
            {
              backgroundColor: theme.colors.surface,
              borderRadius: theme.radii.md,
              opacity: bulkMode && !isSelected ? 0.5 : 1,
            },
          ]}
          activeOpacity={0.7}
          onPress={() => bulkMode ? toggleSelect(item.id ?? "") : router.push(`/media/${item.id}`)}
          onLongPress={() => !bulkMode && handleLongPress(item.id ?? "")}
        >
          {bulkMode && (
            <Ionicons name={isSelected ? "checkmark-circle" : "ellipse-outline"} size={20} color={isSelected ? theme.colors.accent : theme.colors.textMuted} style={{ marginRight: 8 }} />
          )}
          {poster ? (
            <Image
              source={{ uri: poster, headers: authHeaders }}
              style={[styles.listThumb, { borderRadius: theme.radii.sm }]}
              contentFit="cover"
              transition={200}
            />
          ) : (
            <View
              style={[
                styles.listThumb,
                {
                  borderRadius: theme.radii.sm,
                  backgroundColor: theme.colors.surfaceRaised,
                  justifyContent: "center",
                  alignItems: "center",
                },
              ]}
            >
              <Text style={{ color: theme.colors.textMuted, fontSize: 10 }}>
                ?
              </Text>
            </View>
          )}
          <View style={styles.listInfo}>
            <Text
              style={{ fontSize: 15, fontWeight: "600", color: theme.colors.text }}
              numberOfLines={1}
            >
              {item.title}
            </Text>
            <Text style={{ fontSize: 12, marginTop: 2, color: theme.colors.textMuted }}>
              {[item.year, formatType(item.type)].filter(Boolean).join(" · ")}
            </Text>
          </View>
        </TouchableOpacity>
      );
    },
    [theme, router, authHeaders, serverUrl, bulkMode, selected, toggleSelect, handleLongPress],
  );

  const isActiveSort = (f: LibraryServiceListMediaItemsSortField) =>
    sortField === f;

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: library?.name ?? t("media.library.title"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />

      {(searchActive ? isSearchError : isError) ? (
        <View
          style={{ flex: 1, justifyContent: "center", alignItems: "center" }}
        >
          <Text
            style={{ color: theme.colors.textMuted, marginBottom: 12 }}
          >
            {t("common.loadError")}
          </Text>
          <Pressable onPress={() => (searchActive ? refetchSearch() : refetch())}>
            <Text style={{ color: theme.colors.accent }}>
              {t("common.retry")}
            </Text>
          </Pressable>
        </View>
      ) : (
        <>
          {/* Toolbar: search + filter toggle + item count */}
          <View
            style={[
              styles.toolbar,
              { paddingHorizontal: SPACING, paddingVertical: 8 },
            ]}
          >
            <TextInput
              style={[
                styles.searchInput,
                {
                  backgroundColor: theme.colors.surface,
                  color: theme.colors.text,
                  borderRadius: theme.radii.sm,
                },
              ]}
              placeholder={t("media.library.filterByName")}
              placeholderTextColor={theme.colors.textMuted}
              value={searchText}
              onChangeText={setSearchText}
              autoCapitalize="none"
              autoCorrect={false}
            />
            <TouchableOpacity
              style={[
                styles.filterBtn,
                {
                  backgroundColor: showFilters
                    ? theme.colors.accent
                    : theme.colors.surface,
                  borderRadius: theme.radii.sm,
                },
              ]}
              activeOpacity={0.7}
              onPress={() => setShowFilters((v) => !v)}
            >
              <Text
                style={{
                  color: showFilters ? theme.colors.onAccent : theme.colors.textMuted,
                  fontSize: 13,
                  fontWeight: "600",
                }}
              >
                {t("media.library.filters")}
                {(() => {
                  const c = (typeFilter ? 1 : 0) + (genre ? 1 : 0) + (yearMin || yearMax ? 1 : 0) + (ratingMin || ratingMax ? 1 : 0);
                  return c ? ` (${c})` : "";
                })()}
              </Text>
            </TouchableOpacity>
            <TouchableOpacity
              style={[
                styles.filterBtn,
                {
                  backgroundColor: theme.colors.surface,
                  borderRadius: theme.radii.sm,
                },
              ]}
              activeOpacity={0.7}
              onPress={() => setViewMode((v) => (v === "grid" ? "list" : "grid"))}
              accessibilityRole="button"
              accessibilityLabel={t(viewMode === "grid" ? "common.switchToListView" : "common.switchToGridView")}
            >
              <Ionicons
                name={viewMode === "grid" ? "list" : "grid"}
                size={18}
                color={theme.colors.textMuted}
              />
            </TouchableOpacity>
            <TouchableOpacity
              style={[
                styles.filterBtn,
                {
                  backgroundColor: theme.colors.surface,
                  borderRadius: theme.radii.sm,
                  opacity: shuffling ? 0.5 : 1,
                },
              ]}
              activeOpacity={0.7}
              onPress={handleShuffle}
              disabled={shuffling}
              testID="library-shuffle-play"
              accessibilityLabel={t("common.shufflePlay")}
              accessibilityRole="button"
            >
              {shuffling ? (
                <ActivityIndicator size="small" color={theme.colors.textMuted} />
              ) : (
                <Ionicons name="shuffle" size={18} color={theme.colors.textMuted} />
              )}
            </TouchableOpacity>
          </View>

          {/* Item count */}
          <View style={[styles.countRow, { paddingHorizontal: SPACING }]}>
            <Text style={[styles.countText, { color: theme.colors.textMuted }]}>
              {debouncedSearch.trim() || yearMin || yearMax || ratingMin || ratingMax
                ? `${displayItems.length} / ${totalItems}`
                : `${totalItems}`}{" "}
              {totalItems === 1 ? "item" : "items"}
            </Text>
          </View>

          {/* Filters panel */}
          {showFilters && (
            <View
              style={[
                styles.filterPanel,
                {
                  backgroundColor: theme.colors.surface,
                  marginHorizontal: SPACING,
                  borderRadius: theme.radii.md,
                  padding: 12,
                  marginBottom: 8,
                },
              ]}
            >
              {/* Sort */}
              <Text
                style={[
                  styles.filterLabel,
                  { color: theme.colors.textMuted },
                ]}
              >
                {t("media.search.sortBy")}
              </Text>
              <ScrollView
                horizontal
                showsHorizontalScrollIndicator={false}
                style={{ marginBottom: 10 }}
              >
                {SORT_OPTIONS.map((opt) => (
                  <TouchableOpacity
                    key={opt.value}
                    style={[
                      styles.chip,
                      {
                        backgroundColor: isActiveSort(opt.value)
                          ? theme.colors.accent
                          : theme.colors.surfaceRaised,
                        borderRadius: theme.radii.sm,
                      },
                    ]}
                    activeOpacity={0.7}
                    onPress={() => setSortField(opt.value)}
                  >
                    <Text
                      style={{
                        color: isActiveSort(opt.value)
                          ? theme.colors.onAccent
                          : theme.colors.text,
                        fontSize: 13,
                      }}
                    >
                      {t(opt.labelKey)}
                    </Text>
                  </TouchableOpacity>
                ))}
              </ScrollView>

              {/* Sort order */}
              <View style={{ flexDirection: "row", gap: 8, marginBottom: 10 }}>
                <TouchableOpacity
                  style={[
                    styles.chip,
                    {
                      backgroundColor:
                        sortOrder === "SORT_ORDER_ASC"
                          ? theme.colors.accent
                          : theme.colors.surfaceRaised,
                      borderRadius: theme.radii.sm,
                    },
                  ]}
                  activeOpacity={0.7}
                  onPress={() => setSortOrder("SORT_ORDER_ASC")}
                >
                  <Text
                    style={{
                      color:
                        sortOrder === "SORT_ORDER_ASC"
                          ? theme.colors.onAccent
                          : theme.colors.text,
                      fontSize: 13,
                    }}
                  >
                    ↑ {t("media.library.ascending") || "A-Z"}
                  </Text>
                </TouchableOpacity>
                <TouchableOpacity
                  style={[
                    styles.chip,
                    {
                      backgroundColor:
                        sortOrder === "SORT_ORDER_DESC"
                          ? theme.colors.accent
                          : theme.colors.surfaceRaised,
                      borderRadius: theme.radii.sm,
                    },
                  ]}
                  activeOpacity={0.7}
                  onPress={() => setSortOrder("SORT_ORDER_DESC")}
                >
                  <Text
                    style={{
                      color:
                        sortOrder === "SORT_ORDER_DESC"
                          ? theme.colors.onAccent
                          : theme.colors.text,
                      fontSize: 13,
                    }}
                  >
                    ↓ {t("media.library.descending") || "Z-A"}
                  </Text>
                </TouchableOpacity>
              </View>

              {/* Type (series/seasons/episodes) - shown only for SHOWS libraries */}
              {isShowLibrary && (
                <>
                  <Text
                    style={[
                      styles.filterLabel,
                      { color: theme.colors.textMuted, marginTop: 10 },
                    ]}
                  >
                    {t("media.library.type", "Type")}
                  </Text>
                  <ScrollView
                    horizontal
                    showsHorizontalScrollIndicator={false}
                    style={{ marginBottom: 10 }}
                  >
                    <TouchableOpacity
                      style={[
                        styles.chip,
                        {
                          backgroundColor: !typeFilter
                            ? theme.colors.accent
                            : theme.colors.surfaceRaised,
                          borderRadius: theme.radii.sm,
                        },
                      ]}
                      activeOpacity={0.7}
                      onPress={() => setTypeFilter("")}
                    >
                      <Text
                        style={{
                          color: !typeFilter ? theme.colors.onAccent : theme.colors.text,
                          fontSize: 13,
                        }}
                      >
                        {t("media.library.typeAll", "All")}
                      </Text>
                    </TouchableOpacity>
                    {SHOW_TYPE_OPTIONS.map((opt) => (
                      <TouchableOpacity
                        key={opt.value}
                        style={[
                          styles.chip,
                          {
                            backgroundColor:
                              typeFilter === opt.value
                                ? theme.colors.accent
                                : theme.colors.surfaceRaised,
                            borderRadius: theme.radii.sm,
                          },
                        ]}
                        activeOpacity={0.7}
                        onPress={() => setTypeFilter(opt.value)}
                      >
                        <Text
                          style={{
                            color: typeFilter === opt.value ? theme.colors.onAccent : theme.colors.text,
                            fontSize: 13,
                          }}
                        >
                          {t(opt.labelKey)}
                        </Text>
                      </TouchableOpacity>
                    ))}
                  </ScrollView>
                </>
              )}

              {/* Genre */}
              <Text
                style={[
                  styles.filterLabel,
                  { color: theme.colors.textMuted },
                ]}
              >
                {t("media.library.genreLabel")}
              </Text>
              <ScrollView
                horizontal
                showsHorizontalScrollIndicator={false}
              >
                <TouchableOpacity
                  style={[
                    styles.chip,
                    {
                      backgroundColor: !genre
                        ? theme.colors.accent
                        : theme.colors.surfaceRaised,
                      borderRadius: theme.radii.sm,
                    },
                  ]}
                  activeOpacity={0.7}
                  onPress={() => setGenre("")}
                >
                  <Text
                    style={{
                      color: !genre ? theme.colors.onAccent : theme.colors.text,
                      fontSize: 13,
                    }}
                  >
                    {t("media.search.allGenres") || "All"}
                  </Text>
                </TouchableOpacity>
                {availableGenres.map((g) => (
                  <TouchableOpacity
                    key={g}
                    style={[
                      styles.chip,
                      {
                        backgroundColor:
                          genre === g
                            ? theme.colors.accent
                            : theme.colors.surfaceRaised,
                        borderRadius: theme.radii.sm,
                      },
                    ]}
                    activeOpacity={0.7}
                    onPress={() => setGenre(genre === g ? "" : g)}
                  >
                    <Text
                      style={{
                        color:
                          genre === g ? theme.colors.onAccent : theme.colors.text,
                        fontSize: 13,
                      }}
                    >
                      {g}
                    </Text>
                  </TouchableOpacity>
                ))}
              </ScrollView>

              {/* Year range */}
              <Text
                style={[
                  styles.filterLabel,
                  { color: theme.colors.textMuted, marginTop: 10 },
                ]}
              >
                {t("media.search.year")}
              </Text>
              <View style={{ flexDirection: "row", gap: 8 }}>
                <TextInput
                  placeholder={t("media.search.fromYear")}
                  placeholderTextColor={theme.colors.textMuted}
                  value={yearMin}
                  onChangeText={(v) => setYearMin(v.replace(/\D/g, ""))}
                  keyboardType="number-pad"
                  maxLength={4}
                  style={{
                    flex: 1,
                    backgroundColor: theme.colors.bg,
                    color: theme.colors.text,
                    borderRadius: theme.radii.sm,
                    paddingVertical: 8,
                    paddingHorizontal: 10,
                    fontSize: 13,
                    borderWidth: 1,
                    borderColor: theme.colors.border,
                  }}
                />
                <TextInput
                  placeholder={t("media.search.toYear")}
                  placeholderTextColor={theme.colors.textMuted}
                  value={yearMax}
                  onChangeText={(v) => setYearMax(v.replace(/\D/g, ""))}
                  keyboardType="number-pad"
                  maxLength={4}
                  style={{
                    flex: 1,
                    backgroundColor: theme.colors.bg,
                    color: theme.colors.text,
                    borderRadius: theme.radii.sm,
                    paddingVertical: 8,
                    paddingHorizontal: 10,
                    fontSize: 13,
                    borderWidth: 1,
                    borderColor: theme.colors.border,
                  }}
                />
              </View>

              {/* Rating range */}
              <Text
                style={[
                  styles.filterLabel,
                  { color: theme.colors.textMuted, marginTop: 10 },
                ]}
              >
                {t("media.library.ratingSort")}
              </Text>
              <View style={{ flexDirection: "row", gap: 8 }}>
                <TextInput
                  placeholder="0"
                  placeholderTextColor={theme.colors.textMuted}
                  value={ratingMin}
                  onChangeText={(v) => setRatingMin(v.replace(/[^0-9.]/g, ""))}
                  keyboardType="decimal-pad"
                  maxLength={4}
                  style={{
                    flex: 1,
                    backgroundColor: theme.colors.bg,
                    color: theme.colors.text,
                    borderRadius: theme.radii.sm,
                    paddingVertical: 8,
                    paddingHorizontal: 10,
                    fontSize: 13,
                    borderWidth: 1,
                    borderColor: theme.colors.border,
                  }}
                />
                <TextInput
                  placeholder="10"
                  placeholderTextColor={theme.colors.textMuted}
                  value={ratingMax}
                  onChangeText={(v) => setRatingMax(v.replace(/[^0-9.]/g, ""))}
                  keyboardType="decimal-pad"
                  maxLength={4}
                  style={{
                    flex: 1,
                    backgroundColor: theme.colors.bg,
                    color: theme.colors.text,
                    borderRadius: theme.radii.sm,
                    paddingVertical: 8,
                    paddingHorizontal: 10,
                    fontSize: 13,
                    borderWidth: 1,
                    borderColor: theme.colors.border,
                  }}
                />
              </View>
            </View>
          )}

          {/* Grid / List */}
          {(searchActive ? isSearchLoading : (isLoading && page === 1)) ? (
            viewMode === "grid" ? (
              <View
                style={{
                  flexDirection: "row",
                  flexWrap: "wrap",
                  paddingTop: SPACING,
                }}
              >
                {Array.from({ length: 9 }).map((_, i) => (
                  <View
                    key={i}
                    style={{
                      marginLeft: SPACING,
                      marginBottom: SPACING,
                      alignItems: "center",
                      width: ITEM_WIDTH,
                    }}
                  >
                    <Skeleton
                      width={ITEM_WIDTH}
                      height={POSTER_HEIGHT}
                      borderRadius={theme.radii.sm}
                    />
                    <Skeleton
                      width={ITEM_WIDTH * 0.7}
                      height={12}
                      borderRadius={4}
                      style={{ marginTop: 6 }}
                    />
                  </View>
                ))}
              </View>
            ) : (
              <View style={{ padding: SPACING }}>
                {Array.from({ length: 6 }).map((_, i) => (
                  <View
                    key={i}
                    style={{
                      flexDirection: "row",
                      alignItems: "center",
                      padding: 10,
                      marginBottom: 8,
                    }}
                  >
                    <Skeleton
                      width={48}
                      height={72}
                      borderRadius={theme.radii.sm}
                    />
                    <View style={{ flex: 1, marginLeft: 12 }}>
                      <Skeleton width="70%" height={15} borderRadius={4} />
                      <Skeleton
                        width="40%"
                        height={12}
                        borderRadius={4}
                        style={{ marginTop: 6 }}
                      />
                    </View>
                  </View>
                ))}
              </View>
            )
          ) : viewMode === "grid" ? (
            <FlatList
              key="grid"
              data={displayItems}
              keyExtractor={libraryKeyExtractor}
              renderItem={renderItem}
              numColumns={NUM_COLUMNS}
              contentContainerStyle={{ paddingTop: SPACING }}
              refreshing={searchActive ? isSearchLoading : (isFetching && page === 1)}
              onRefresh={handleRefresh}
              onEndReached={handleEndReached}
              onEndReachedThreshold={0.5}
              getItemLayout={libraryGetItemLayout}
              initialNumToRender={9}
              maxToRenderPerBatch={12}
              windowSize={5}
              removeClippedSubviews={true}
              ListFooterComponent={
                isFetching && page > 1 ? (
                  <ActivityIndicator
                    color={theme.colors.accent}
                    style={{ marginVertical: 16 }}
                  />
                ) : null
              }
              ListEmptyComponent={
                <View style={styles.center}>
                  <Text style={{ color: theme.colors.textMuted }}>
                    {typeFilter || genre || debouncedSearch.trim() || yearMin || yearMax || ratingMin || ratingMax
                      ? t("media.library.noItemsFiltered") || t("common.noItems")
                      : t("common.noItems")}
                  </Text>
                </View>
              }
            />
          ) : (
            <FlatList
              key="list"
              data={displayItems}
              keyExtractor={libraryKeyExtractor}
              renderItem={renderListItem}
              contentContainerStyle={{ padding: SPACING }}
              ItemSeparatorComponent={() => <View style={{ height: 8 }} />}
              refreshing={searchActive ? isSearchLoading : (isFetching && page === 1)}
              onRefresh={handleRefresh}
              onEndReached={handleEndReached}
              onEndReachedThreshold={0.5}
              initialNumToRender={10}
              maxToRenderPerBatch={12}
              windowSize={5}
              removeClippedSubviews={true}
              ListFooterComponent={
                isFetching && page > 1 ? (
                  <ActivityIndicator
                    color={theme.colors.accent}
                    style={{ marginVertical: 16 }}
                  />
                ) : null
              }
              ListEmptyComponent={
                <View style={styles.center}>
                  <Text style={{ color: theme.colors.textMuted }}>
                    {typeFilter || genre || debouncedSearch.trim() || yearMin || yearMax || ratingMin || ratingMax
                      ? t("media.library.noItemsFiltered") || t("common.noItems")
                      : t("common.noItems")}
                  </Text>
                </View>
              }
            />
          )}
        </>
      )}

      {/* Bulk action bar */}
      {bulkMode && (
        <View style={[styles.bulkBar, { backgroundColor: theme.colors.surfaceRaised, borderTopColor: theme.colors.border }]}>
          <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{t("common.selectedCount", "{{count}} selected", { count: selected.size })}</Text>
          <ScrollView horizontal showsHorizontalScrollIndicator={false} contentContainerStyle={{ gap: 12 }}>
            <Pressable onPress={bulkMarkWatched} style={styles.bulkBtn} accessible accessibilityLabel={t("media.library.markWatched", "Mark watched")} accessibilityRole="button">
              <Ionicons name="checkmark-circle-outline" size={20} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.library.markWatched", "Watched")}</Text>
            </Pressable>
            <Pressable onPress={bulkMarkUnwatched} style={styles.bulkBtn} accessible accessibilityLabel={t("media.library.markUnwatched", "Mark unwatched")} accessibilityRole="button">
              <Ionicons name="eye-off-outline" size={20} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.library.markUnwatched", "Unwatched")}</Text>
            </Pressable>
            <Pressable onPress={bulkToggleFavorite} style={styles.bulkBtn} accessible accessibilityLabel={t("media.favorites.toggleFavorite", "Favorite")} accessibilityRole="button">
              <Ionicons name="heart-outline" size={20} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.favorites.toggleFavorite", "Favorite")}</Text>
            </Pressable>
            <Pressable onPress={() => setShowBulkPlaylist(true)} style={styles.bulkBtn} accessible accessibilityLabel={t("media.detail.addToPlaylist", "Playlist")} accessibilityRole="button">
              <Ionicons name="add-circle-outline" size={20} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.detail.addToPlaylist", "Playlist")}</Text>
            </Pressable>
            <Pressable onPress={() => setShowBulkCollection(true)} style={styles.bulkBtn} accessible accessibilityLabel={t("media.library.addToCollection", "Add to collection")} accessibilityRole="button">
              <Ionicons name="albums-outline" size={20} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.library.addToCollection", "Add to collection")}</Text>
            </Pressable>
            <Pressable onPress={exitBulk} style={styles.bulkBtn} accessible accessibilityLabel={t("common.cancel")} accessibilityRole="button">
              <Ionicons name="close" size={20} color={theme.colors.error} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("common.cancel")}</Text>
            </Pressable>
          </ScrollView>
        </View>
      )}
      {showBulkPlaylist && selected.size > 0 && (
        <AddToPlaylistSheet
          visible={showBulkPlaylist}
          mediaItemId={Array.from(selected)[0]}
          onClose={() => { setShowBulkPlaylist(false); exitBulk(); }}
        />
      )}
      {showBulkCollection && selected.size > 0 && (
        <BulkAddToCollectionSheet
          visible={showBulkCollection}
          mediaItemIds={Array.from(selected)}
          onClose={() => { setShowBulkCollection(false); exitBulk(); }}
        />
      )}
    </View>
  );
}

// Mobile has no shared "add to collection" sheet (unlike AddToPlaylistSheet),
// so this mirrors that component's structure locally rather than growing a
// new shared component outside this screen's whitelist. Reuses the existing
// useCollections list hook and the generated collectionServiceAddCollectionItem
// mutation - same pieces web's AddToCollectionModal is built from.
function BulkAddToCollectionSheet({
  visible,
  mediaItemIds,
  onClose,
}: {
  visible: boolean;
  mediaItemIds: string[];
  onClose: () => void;
}) {
  const theme = useTheme();
  const { t } = useTranslation();
  const { data, isLoading } = useCollections();
  const collections = data?.collections ?? [];
  const [pendingId, setPendingId] = useState<string | null>(null);

  async function handleAdd(collectionId: string, name: string) {
    setPendingId(collectionId);
    try {
      await Promise.all(
        mediaItemIds.map((id) => collectionServiceAddCollectionItem(collectionId, { mediaItemId: id, sortOrder: 0 })),
      );
      toast.success(t("media.collections.itemsAdded", { count: mediaItemIds.length, name }));
      onClose();
    } catch {
      toast.error(t("media.collections.addFailed"));
    } finally {
      setPendingId(null);
    }
  }

  return (
    <Modal visible={visible} animationType="slide" transparent onRequestClose={onClose}>
      <Pressable style={[collectionSheetStyles.backdrop, { backgroundColor: theme.colors.overlay }]} onPress={onClose}>
        <Pressable
          style={[collectionSheetStyles.sheet, { backgroundColor: theme.colors.surface, borderTopLeftRadius: theme.radii.lg, borderTopRightRadius: theme.radii.lg }]}
          onPress={(e) => e.stopPropagation()}
        >
          <View style={collectionSheetStyles.handle}>
            <View style={[collectionSheetStyles.handleBar, { backgroundColor: theme.colors.textMuted }]} />
          </View>
          <Text style={[collectionSheetStyles.title, { color: theme.colors.text }]}>
            {t("media.contextMenu.addToCollection", "Add to Collection")}
          </Text>

          {isLoading ? (
            <ActivityIndicator color={theme.colors.accent} style={{ marginTop: 24 }} />
          ) : collections.length === 0 ? (
            <Text style={[collectionSheetStyles.empty, { color: theme.colors.textMuted }]}>
              {t("media.collections.noCollectionsAvailable")}
            </Text>
          ) : (
            <FlatList
              data={collections}
              keyExtractor={(item) => item.id}
              style={{ maxHeight: 320 }}
              renderItem={({ item }) => (
                <TouchableOpacity
                  style={[collectionSheetStyles.row, { borderBottomColor: theme.colors.border }]}
                  activeOpacity={0.7}
                  onPress={() => void handleAdd(item.id, item.name)}
                  disabled={pendingId !== null}
                >
                  <Ionicons name="albums" size={ICON_SIZE.lg} color={theme.colors.accent} />
                  <Text style={[collectionSheetStyles.collectionName, { color: theme.colors.text }]} numberOfLines={1}>
                    {item.name}
                  </Text>
                  {pendingId === item.id ? (
                    <ActivityIndicator size="small" color={theme.colors.accent} />
                  ) : (
                    <Ionicons name="add-circle-outline" size={ICON_SIZE.lg} color={theme.colors.textMuted} />
                  )}
                </TouchableOpacity>
              )}
            />
          )}

          <TouchableOpacity
            style={[collectionSheetStyles.closeBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
            onPress={onClose}
          >
            <Text style={[collectionSheetStyles.closeBtnText, { color: theme.colors.text }]}>{t("common.close", "Close")}</Text>
          </TouchableOpacity>
        </Pressable>
      </Pressable>
    </Modal>
  );
}

const styles = StyleSheet.create({
  container: { flex: 1 },
  center: {
    flex: 1,
    justifyContent: "center",
    alignItems: "center",
    paddingTop: 60,
  },
  toolbar: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
  },
  searchInput: {
    flex: 1,
    paddingHorizontal: 12,
    paddingVertical: 8,
    fontSize: 14,
  },
  filterBtn: {
    paddingHorizontal: 14,
    paddingVertical: 9,
  },
  countRow: { paddingVertical: 4 },
  countText: { fontSize: 12 },
  filterPanel: {},
  filterLabel: {
    fontSize: 11,
    fontWeight: "700",
    textTransform: "uppercase",
    letterSpacing: 0.5,
    marginBottom: 6,
  },
  chip: {
    paddingHorizontal: 12,
    paddingVertical: 6,
    marginRight: 6,
  },
  listRow: {
    flexDirection: "row",
    alignItems: "center",
    padding: 10,
  },
  listThumb: { width: 48, height: 72, marginRight: 12 },
  listInfo: { flex: 1 },
  selectCheckmark: { position: "absolute", top: 6, left: 6, zIndex: 1 },
  techBadge: {
    position: "absolute",
    flexDirection: "row",
    alignItems: "center",
    gap: 2,
    paddingHorizontal: 4,
    paddingVertical: 2,
  },
  techBadgeText: { fontSize: 10, fontWeight: "700" },
  techBadgeRow: { position: "absolute", flexDirection: "row", gap: 4 },
  bulkBar: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: 16, paddingVertical: 10, borderTopWidth: StyleSheet.hairlineWidth },
  bulkBtn: { alignItems: "center", gap: 2 },
});

const collectionSheetStyles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "flex-end" },
  sheet: { paddingBottom: 34, paddingHorizontal: 16 },
  handle: { alignItems: "center", paddingVertical: 10 },
  handleBar: { width: 40, height: 4, borderRadius: 2, opacity: 0.4 },
  title: { fontSize: FONT_SIZE.lg, fontWeight: "700", marginBottom: 12 },
  empty: { fontSize: FONT_SIZE.sm, textAlign: "center", marginTop: 24 },
  row: { flexDirection: "row", alignItems: "center", paddingVertical: 14, gap: 12, borderBottomWidth: StyleSheet.hairlineWidth },
  collectionName: { flex: 1, fontSize: FONT_SIZE.sm, fontWeight: "500" },
  closeBtn: { marginTop: 16, paddingVertical: 12, alignItems: "center" },
  closeBtnText: { fontSize: FONT_SIZE.sm, fontWeight: "600" },
});
