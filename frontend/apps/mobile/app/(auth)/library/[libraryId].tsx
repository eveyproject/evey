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
import { Skeleton } from "../../../src/components/Skeleton";
import { useLocalSearchParams, useRouter, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetLibrary,
  useLibraryServiceListMediaItems,
} from "@evey/api";
import type {
  V1MediaItem,
  LibraryServiceListMediaItemsSortField,
  LibraryServiceListMediaItemsSortOrder,
} from "@evey/api";
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

  // Client-side text filter
  const displayItems = useMemo(() => {
    if (!debouncedSearch.trim()) return allItems;
    const lower = debouncedSearch.toLowerCase();
    return allItems.filter((item) =>
      item.title?.toLowerCase().includes(lower),
    );
  }, [allItems, debouncedSearch]);

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
      return (
        <TouchableOpacity
          style={[
            styles.gridItem,
            {
              width: ITEM_WIDTH,
              marginLeft: SPACING,
              marginBottom: SPACING,
            },
          ]}
          activeOpacity={0.7}
          onPress={() => router.push(`/media/${item.id}`)}
        >
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
    [theme, router, authHeaders, t],
  );

  const isActiveSort = (f: LibraryServiceListMediaItemsSortField) =>
    sortField === f;

  return (
    <View style={[styles.container, { backgroundColor: theme.colors.bg }]}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: library?.name ?? "Library",
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
                {genre ? " (1)" : ""}
              </Text>
            </TouchableOpacity>
          </View>

          {/* Item count */}
          <View style={[styles.countRow, { paddingHorizontal: SPACING }]}>
            <Text style={[styles.countText, { color: theme.colors.textMuted }]}>
              {debouncedSearch.trim()
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
                    ↑ {t("common.ascending") || "A-Z"}
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
                    ↓ {t("common.descending") || "Z-A"}
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
            </View>
          )}

          {/* Grid */}
          {isLoading && page === 1 ? (
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
            <FlatList
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
                    {genre || debouncedSearch.trim()
                      ? t("media.library.noItemsFiltered") || t("common.noItems")
                      : t("common.noItems")}
                  </Text>
                </View>
              }
            />
          )}
        </>
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
});
