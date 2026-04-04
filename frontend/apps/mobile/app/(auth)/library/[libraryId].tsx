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
} from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { Skeleton } from "../../../src/components/Skeleton";
import { useLocalSearchParams, useRouter, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetLibrary,
  useLibraryServiceListMediaItems,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
} from "@evey/api";
import type {
  V1MediaItem,
  LibraryServiceListMediaItemsSortField,
  LibraryServiceListMediaItemsSortOrder,
} from "@evey/api";
import { useAuthStore } from "@evey/store";
import { toast } from "@evey/store";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useDebounce } from "../../../src/hooks/useDebounce";

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

function getPoster(item: V1MediaItem): string | undefined {
  return item.artwork?.find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.url;
}

function formatType(type?: string): string {
  if (type === "MEDIA_ITEM_TYPE_MOVIE") return "Movie";
  if (type === "MEDIA_ITEM_TYPE_SERIES") return "Series";
  return "";
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

  const markWatched = usePlaybackServiceMarkAsWatched({ mutation: { onSuccess: () => toast.success("Marked watched") } });
  const markUnwatched = usePlaybackServiceMarkAsUnwatched({ mutation: { onSuccess: () => toast.success("Marked unwatched") } });

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
    for (const id of selected) {
      try { await markWatched.mutateAsync({ mediaItemId: id, params: { profileId } }); } catch {}
    }
    exitBulk();
  }, [selected, markWatched, profileId, exitBulk]);

  const bulkMarkUnwatched = useCallback(async () => {
    for (const id of selected) {
      try { await markUnwatched.mutateAsync({ mediaItemId: id, params: { profileId } }); } catch {}
    }
    exitBulk();
  }, [selected, markUnwatched, profileId, exitBulk]);

  const authHeaders = useAuthHeaders();
  const { data: library } = useLibraryServiceGetLibrary(libraryId ?? "");

  const { data, isLoading, isError, isFetching, refetch } =
    useLibraryServiceListMediaItems(
      {
        libraryId,
        ...(genre ? { genre } : {}),
        sortField,
        sortOrder,
        "pagination.page": page,
        "pagination.pageSize": PAGE_SIZE,
      },
      { query: { enabled: !!libraryId } },
    );

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

  // Reset pagination when filters/sort change
  const resetAndRefetch = useCallback(() => {
    setPage(1);
    setAllItems([]);
    setHasMore(true);
    prevDataRef.current = null;
  }, []);

  useEffect(() => {
    resetAndRefetch();
  }, [sortField, sortOrder, genre, resetAndRefetch]);

  const totalItems = data?.pagination?.totalItems ?? 0;

  // Client-side text + year/rating filter
  const displayItems = useMemo(() => {
    let items = allItems;
    if (debouncedSearch.trim()) {
      const lower = debouncedSearch.toLowerCase();
      items = items.filter((item) => item.title?.toLowerCase().includes(lower));
    }
    if (yearMin) {
      const min = parseInt(yearMin);
      items = items.filter((item) => (item.year ?? 0) >= min);
    }
    if (yearMax) {
      const max = parseInt(yearMax);
      items = items.filter((item) => (item.year ?? 9999) <= max);
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
  }, [allItems, debouncedSearch, yearMin, yearMax, ratingMin, ratingMax]);

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

  const handleRefresh = useCallback(() => {
    setPage(1);
    setHasMore(true);
    prevDataRef.current = null;
  }, []);

  const handleEndReached = useCallback(() => {
    if (!isFetching && hasMore) {
      setPage((p) => p + 1);
    }
  }, [isFetching, hasMore]);

  const renderItem = useCallback(
    ({ item }: { item: V1MediaItem }) => {
      const poster = getPoster(item);
      const isSelected = bulkMode && selected.has(item.id ?? "");
      return (
        <TouchableOpacity
          style={[
            styles.gridItem,
            {
              width: ITEM_WIDTH,
              marginLeft: SPACING,
              marginBottom: SPACING,
              opacity: bulkMode && !isSelected ? 0.5 : 1,
            },
          ]}
          activeOpacity={0.7}
          onPress={() => bulkMode ? toggleSelect(item.id ?? "") : router.push(`/media/${item.id}`)}
          onLongPress={() => !bulkMode && handleLongPress(item.id ?? "")}
        >
          {isSelected && (
            <View style={styles.checkmark}>
              <Ionicons name="checkmark-circle" size={22} color={theme.colors.accent} />
            </View>
          )}
          {poster ? (
            <Image
              source={{ uri: poster, headers: authHeaders }}
              style={[
                styles.poster,
                {
                  width: ITEM_WIDTH,
                  height: POSTER_HEIGHT,
                  borderRadius: theme.radii.sm,
                },
              ]}
              contentFit="cover"
              transition={200}
            />
          ) : (
            <View
              style={[
                styles.posterPlaceholder,
                {
                  width: ITEM_WIDTH,
                  height: POSTER_HEIGHT,
                  borderRadius: theme.radii.sm,
                  backgroundColor: theme.colors.surfaceRaised,
                },
              ]}
            >
              <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>
                {t("common.noPoster")}
              </Text>
            </View>
          )}
          <Text
            style={[styles.title, { color: theme.colors.text }]}
            numberOfLines={2}
          >
            {item.title}
          </Text>
          {item.year ? (
            <Text style={[styles.year, { color: theme.colors.textMuted }]}>
              {item.year}
            </Text>
          ) : null}
        </TouchableOpacity>
      );
    },
    [theme, router, authHeaders, t, bulkMode, selected, toggleSelect, handleLongPress],
  );

  const renderListItem = useCallback(
    ({ item }: { item: V1MediaItem }) => {
      const poster = getPoster(item);
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
    [theme, router, authHeaders, bulkMode, selected, toggleSelect, handleLongPress],
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

      {isError ? (
        <View
          style={{ flex: 1, justifyContent: "center", alignItems: "center" }}
        >
          <Text
            style={{ color: theme.colors.textMuted, marginBottom: 12 }}
          >
            {t("common.loadError")}
          </Text>
          <Pressable onPress={() => refetch()}>
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
                  color: showFilters ? "#fff" : theme.colors.textMuted,
                  fontSize: 13,
                  fontWeight: "600",
                }}
              >
                {t("media.library.filters")}
                {(() => {
                  const c = (genre ? 1 : 0) + (yearMin || yearMax ? 1 : 0) + (ratingMin || ratingMax ? 1 : 0);
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
            >
              <Ionicons
                name={viewMode === "grid" ? "list" : "grid"}
                size={18}
                color={theme.colors.textMuted}
              />
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
                          ? "#fff"
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
                          ? "#fff"
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
                          ? "#fff"
                          : theme.colors.text,
                      fontSize: 13,
                    }}
                  >
                    ↓ {t("media.library.descending") || "Z-A"}
                  </Text>
                </TouchableOpacity>
              </View>

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
                      color: !genre ? "#fff" : theme.colors.text,
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
                          genre === g ? "#fff" : theme.colors.text,
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
          {isLoading && page === 1 ? (
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
              refreshing={isFetching && page === 1}
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
                    {genre || debouncedSearch.trim() || yearMin || yearMax || ratingMin || ratingMax
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
              refreshing={isFetching && page === 1}
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
                    {genre || debouncedSearch.trim() || yearMin || yearMax || ratingMin || ratingMax
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
          <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{selected.size} selected</Text>
          <View style={{ flexDirection: "row", gap: 12 }}>
            <Pressable onPress={bulkMarkWatched} style={styles.bulkBtn}>
              <Ionicons name="checkmark-circle-outline" size={20} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>Watched</Text>
            </Pressable>
            <Pressable onPress={bulkMarkUnwatched} style={styles.bulkBtn}>
              <Ionicons name="eye-off-outline" size={20} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>Unwatched</Text>
            </Pressable>
            <Pressable onPress={exitBulk} style={styles.bulkBtn}>
              <Ionicons name="close" size={20} color="#ef4444" />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>Cancel</Text>
            </Pressable>
          </View>
        </View>
      )}
    </View>
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
  gridItem: { alignItems: "center" },
  poster: { width: "100%" },
  posterPlaceholder: {
    justifyContent: "center",
    alignItems: "center",
  },
  title: {
    fontSize: 12,
    fontWeight: "500",
    marginTop: 4,
    textAlign: "center",
  },
  year: {
    fontSize: 10,
    marginTop: 1,
    textAlign: "center",
  },
  listRow: {
    flexDirection: "row",
    alignItems: "center",
    padding: 10,
  },
  listThumb: { width: 48, height: 72, marginRight: 12 },
  listInfo: { flex: 1 },
  checkmark: { position: "absolute", top: 4, right: 4, zIndex: 1, backgroundColor: "rgba(0,0,0,0.5)", borderRadius: 12 },
  bulkBar: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: 16, paddingVertical: 10, borderTopWidth: StyleSheet.hairlineWidth },
  bulkBtn: { alignItems: "center", gap: 2 },
});
