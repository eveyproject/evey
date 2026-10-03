import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { View, Text, FlatList, ScrollView, TextInput, ActivityIndicator, StyleSheet } from "react-native";
import { Ionicons } from "@expo/vector-icons";
import { useLocalSearchParams, useRouter, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetLibrary,
  useLibraryServiceListMediaItems,
  hasMoreFromCursor,
  dedicatedRouteFor,
  IN_LIBRARY_SEARCH_LIMIT,
  useLibraryServiceListGenres,
  useLibraryServiceSearchMedia,
  usePlaybackServiceGetWatchHistory,
  libraryServiceListMediaItems,
  itemLabel,
  WATCH_HISTORY_OVERLAY_LIMIT,
} from "@evey/api";
import type {
  V1MediaItem,
  LibraryServiceListMediaItemsType,
  LibraryServiceListMediaItemsSortField,
  LibraryServiceListMediaItemsSortOrder,
  LibraryServiceSearchMediaType,
  LibraryServiceSearchMediaSortField,
  LibraryServiceSearchMediaSortOrder,
} from "@evey/api";
import { toast } from "@evey/store";
import { buildWatchProgressMaps, videoStream, isHdrStream, resolutionLabel } from "@evey/client";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { FocusableCard } from "../../../src/components/FocusableCard";
import { artworkUrl } from "../../../src/lib/artwork";
import { PosterImage } from "../../../src/components/PosterImage";
import { useServerUrl } from "../../../src/hooks/useServerUrl";
import { useAuthHeaders } from "../../../src/hooks/useAuthenticatedImage";
import { useDebounce } from "../../../src/hooks/useDebounce";
import { CARD_WIDTH_GRID, POSTER_RATIO, LABEL_BLOCK_HEIGHT, GRID_GAP } from "../../../src/lib/tvLayout";
import { canPlayAllForSelection, buildPlayAllQueue, pickRandomItem } from "../../../src/lib/librarySelection";

const NUM_COLUMNS = 5;
const CARD_WIDTH = CARD_WIDTH_GRID;
const POSTER_HEIGHT = CARD_WIDTH * POSTER_RATIO;
const ROW_HEIGHT = POSTER_HEIGHT + LABEL_BLOCK_HEIGHT + GRID_GAP;
const PAGE_SIZE = 30;
// The dedicated screens this app implements. A photo library now has its own
// grid and remote-driven slideshow (app/(auth)/photos), so it goes there
// instead of being browsed as a wall of poster-shaped cards whose detail page
// is built for video.
const TV_DEDICATED_ROUTES = ["/music", "/photos"];

const SORT_KEYS: Array<{ labelKey: string; value: LibraryServiceListMediaItemsSortField }> = [
  { labelKey: "media.library.titleSort", value: "MEDIA_SORT_FIELD_TITLE" },
  { labelKey: "media.library.year", value: "MEDIA_SORT_FIELD_YEAR" },
  { labelKey: "media.library.ratingSort", value: "MEDIA_SORT_FIELD_RATING" },
  { labelKey: "media.library.dateAdded", value: "MEDIA_SORT_FIELD_DATE_ADDED" },
];

const TYPE_KEYS: Array<{ labelKey: string; value: LibraryServiceListMediaItemsType }> = [
  { labelKey: "media.library.series", value: "MEDIA_ITEM_TYPE_SERIES" },
  { labelKey: "media.library.seasons", value: "MEDIA_ITEM_TYPE_SEASON" },
  { labelKey: "media.library.episodes", value: "MEDIA_ITEM_TYPE_EPISODE" },
];

function tvLibKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

const tvLibGetItemLayout = (_: unknown, index: number) => ({
  length: ROW_HEIGHT,
  offset: ROW_HEIGHT * Math.floor(index / NUM_COLUMNS),
  index,
});

// --- Pure helpers (unit-tested in [libraryId].pure.test.ts) -----------------

export interface BuildMediaListParamsInput {
  libraryId: string;
  type?: LibraryServiceListMediaItemsType | "";
  genre?: string;
  sortField: LibraryServiceListMediaItemsSortField;
  sortOrder: LibraryServiceListMediaItemsSortOrder;
  cursor: string;
  pageSize: number;
}

// Single source of truth for the ListMediaItems query params, so the sort/
// type/genre wiring can be unit-tested without rendering the screen.
// Keyset-paginated (opaque server cursor), like web and mobile: the previous
// `pagination.page` + `totalPages` pairing could never advance, because the
// server never populates total_pages (always 0), so the TV froze on the first
// page of every library.
export function buildMediaListParams(input: BuildMediaListParamsInput) {
  return {
    libraryId: input.libraryId,
    ...(input.type ? { type: input.type } : {}),
    ...(input.genre ? { genre: input.genre } : {}),
    sortField: input.sortField,
    sortOrder: input.sortOrder,
    "pagination.pageSize": input.pageSize,
    ...(input.cursor ? { "pagination.cursor": input.cursor } : {}),
  };
}

export interface BuildLibrarySearchParamsInput {
  libraryId: string;
  query: string;
  type?: LibraryServiceListMediaItemsType | "";
  genre?: string;
  sortField: LibraryServiceListMediaItemsSortField;
  sortOrder: LibraryServiceListMediaItemsSortOrder;
  limit: number;
}

// SearchMedia's type/sortField/sortOrder enums share the same string values
// as ListMediaItems's - the cast avoids importing/maintaining a parallel set
// of literals purely to satisfy two structurally-identical generated types.
export function buildLibrarySearchParams(input: BuildLibrarySearchParamsInput) {
  return {
    libraryId: input.libraryId,
    query: input.query,
    ...(input.type ? { type: input.type as unknown as LibraryServiceSearchMediaType } : {}),
    ...(input.genre ? { genre: input.genre } : {}),
    sortField: input.sortField as unknown as LibraryServiceSearchMediaSortField,
    sortOrder: input.sortOrder as unknown as LibraryServiceSearchMediaSortOrder,
    limit: input.limit,
  };
}

export { videoStream, isHdrStream, resolutionLabel };

// D-pad genre/sort/type filter: a row of focusable chips (same pattern as the
// search screen's ScopeTab) rather than a dropdown, which has no natural TV/
// remote affordance.
function Chip({ label, active, onPress }: { label: string; active: boolean; onPress: () => void }) {
  const theme = useTheme();
  return (
    <FocusableCard
      onPress={onPress}
      style={[
        styles.chip,
        { backgroundColor: active ? theme.colors.accent : theme.colors.surface },
      ]}
      accessibilityLabel={label}
      accessibilityState={{ selected: active }}
    >
      <Text style={{ color: active ? theme.colors.onAccent : theme.colors.text, fontSize: 17, fontWeight: "600" }}>{label}</Text>
    </FocusableCard>
  );
}

export default function LibraryDetailScreen() {
  const { libraryId } = useLocalSearchParams<{ libraryId: string }>();
  const theme = useTheme();
  const { t } = useTranslation();
  const router = useRouter();
  const serverUrl = useServerUrl();
  const authHeaders = useAuthHeaders();
  const [cursor, setCursor] = useState("");
  const [allItems, setAllItems] = useState<V1MediaItem[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const [genre, setGenre] = useState("");
  const [typeFilter, setTypeFilter] = useState<LibraryServiceListMediaItemsType | "">("");
  const [sortField, setSortField] = useState<LibraryServiceListMediaItemsSortField>("MEDIA_SORT_FIELD_TITLE");
  const [sortOrder, setSortOrder] = useState<LibraryServiceListMediaItemsSortOrder>("SORT_ORDER_ASC");
  const [searchText, setSearchText] = useState("");
  const [searchFocused, setSearchFocused] = useState(false);
  const debouncedSearch = useDebounce(searchText, 300);
  const [shuffling, setShuffling] = useState(false);
  // Ref latch, not just the state flag: a second remote OK press before the
  // setShuffling(true) re-render would read the old state and slip a second
  // probe + navigation through. The ref updates synchronously, closing that
  // window (FocusableCard has no disabled prop to fall back on).
  const shufflingRef = useRef(false);
  const prevDataRef = useRef<unknown>(null);

  const { data: library } = useLibraryServiceGetLibrary(libraryId ?? "");
  const isShowLibrary = library?.type === "LIBRARY_TYPE_SHOWS";

  // A music library holds no media_items, so browsing it here renders an empty
  // grid over a full library. Send it to the music screen instead, as web has
  // always done. replace(), so the empty grid is not left as a back target.
  const dedicated = dedicatedRouteFor(library?.type, TV_DEDICATED_ROUTES);
  useEffect(() => {
    if (dedicated) router.replace(dedicated);
  }, [dedicated, router]);
  const { data: genresData } = useLibraryServiceListGenres(libraryId ?? "", {
    query: { enabled: !!libraryId },
  });
  const genres = genresData?.genres ?? [];

  const trimmedSearch = debouncedSearch.trim();
  const searchActive = trimmedSearch.length > 0;
  const hasActiveFilters = Boolean(genre || typeFilter || searchActive);

  const { data, isLoading, isError, isFetching, refetch } = useLibraryServiceListMediaItems(
    buildMediaListParams({ libraryId: libraryId ?? "", type: typeFilter, genre, sortField, sortOrder, cursor, pageSize: PAGE_SIZE }),
    {
      query: {
        enabled: !!libraryId && !searchActive,
        // See mobile: one cache key per cursor, and a spent cursor is never
        // read again, so don't let superseded pages pile up next to allItems.
        gcTime: 30_000,
      },
    },
  );

  const {
    data: searchData,
    isLoading: isSearchLoading,
    isError: isSearchError,
    refetch: refetchSearch,
  } = useLibraryServiceSearchMedia(
    buildLibrarySearchParams({ libraryId: libraryId ?? "", query: trimmedSearch, type: typeFilter, genre, sortField, sortOrder, limit: IN_LIBRARY_SEARCH_LIMIT }),
    { query: { enabled: searchActive && !!libraryId } },
  );

  // Watched/progress overlay data, same source used by the history screen.
  // Fetched at a generous limit so the map covers everything likely to be
  // visible in one library, mirroring mobile's equivalent (limit: 500).
  const { data: watchHistoryData } = usePlaybackServiceGetWatchHistory(
    { limit: WATCH_HISTORY_OVERLAY_LIMIT },
    { query: { enabled: !!libraryId, staleTime: 30_000 } },
  );
  const { watchedMap, percentMap } = buildWatchProgressMaps(watchHistoryData?.items ?? []);

  // Reset pagination when a filter/sort param changes. Declared BEFORE the
  // accumulate effect ON PURPOSE: React runs effects in declaration order, so
  // on the render where a filter flips to a value whose page-1 is already cached
  // (query resolves synchronously), this clear must run FIRST and the
  // accumulate effect repopulates SECOND. The reverse order let the accumulate
  // fill allItems and this clear wipe it to [] with no re-run (its [data,page]
  // deps were unchanged) - a persistent empty grid on the common peek-a-facet-
  // then-back-to-default flow over a static library.
  useEffect(() => {
    setCursor("");
    setAllItems([]);
    setHasMore(true);
    prevDataRef.current = null;
  }, [genre, typeFilter, sortField, sortOrder]);

  useEffect(() => {
    if (!data || data === prevDataRef.current) return;
    prevDataRef.current = data;
    const newItems = data.items ?? [];
    if (!cursor) {
      setAllItems(newItems);
    } else {
      setAllItems((prev) => [...prev, ...newItems]);
    }
    setHasMore(hasMoreFromCursor(data.pagination));
  }, [data, cursor]);

  const displayItems = useMemo(
    () => (searchActive ? (searchData?.items ?? []) : allItems),
    [searchActive, searchData?.items, allItems],
  );

  // Shuffle play: with a search or type/genre facet active, shuffle among
  // what's actually on screen so the pick honors it. With no facet, shuffle
  // the WHOLE library via a random page offset (pageSize 1) rather than the
  // loaded slice - the genre/type filter, if any, still applies server-side
  // since it's baked into the probe request.
  const handleShuffle = useCallback(async () => {
    if (shufflingRef.current || !libraryId) return;
    shufflingRef.current = true;
    setShuffling(true);
    try {
      if (searchActive) {
        // The search page has no cursor to page through - it IS the full
        // result set for this query, so picking within it is not the
        // pagination trap the whole-library path below avoids.
        const pick = pickRandomItem(displayItems);
        if (!pick) { toast.info(t("common.shuffleEmpty")); return; }
        if (pick.id) router.push(`/player/${pick.id}`);
        else toast.error(t("common.shuffleFailed"));
        return;
      }
      const base = { libraryId, ...(typeFilter ? { type: typeFilter } : {}), ...(genre ? { genre } : {}) };
      const probe = await libraryServiceListMediaItems({ ...base, "pagination.page": 1, "pagination.pageSize": 1 });
      const total = probe.pagination?.totalItems ?? 0;
      if (total === 0) { toast.info(t("common.shuffleEmpty")); return; }
      const randomPage = Math.floor(Math.random() * total) + 1;
      const res = randomPage === 1
        ? probe
        : await libraryServiceListMediaItems({ ...base, "pagination.page": randomPage, "pagination.pageSize": 1 });
      const pick = res.items?.[0];
      if (pick?.id) router.push(`/player/${pick.id}`);
      else toast.error(t("common.shuffleFailed"));
    } catch {
      toast.error(t("common.shuffleFailed"));
    } finally {
      shufflingRef.current = false;
      setShuffling(false);
    }
  }, [libraryId, genre, typeFilter, searchActive, displayItems, router, t]);

  // A series or a season has no video stream of its own, so "play all" only
  // makes sense once the on-screen selection is directly playable: always on
  // a movies library, and on a shows library only once the type filter has
  // narrowed the view down to episodes. Elsewhere the control is not shown at
  // all, rather than surfaced as a button that always no-ops.
  const canPlayAll = canPlayAllForSelection(isShowLibrary, typeFilter);

  // Only the first entry of the built queue is actually navigated to today:
  // the player has no way to consume a queue beyond that (its own "up next"
  // is series continuity, unrelated to this screen's sort/filter). The full
  // ordered queue is still built here, in one place, ready to be handed to
  // the player wholesale if it grows that capability later.
  const handlePlayAll = useCallback(() => {
    const queue = buildPlayAllQueue(displayItems);
    const first = queue[0];
    if (!first) { toast.info(t("common.playAllEmpty")); return; }
    router.push(`/player/${first}`);
  }, [displayItems, router, t]);

  const handleEndReached = useCallback(() => {
    if (searchActive) return; // search returns a single fixed-size page, no cursor to page through
    // Guarding on isFetching and on the cursor actually changing keeps a held-
    // down remote from firing the same cursor twice before the fetch resolves.
    const next = data?.pagination?.nextCursor;
    if (!isFetching && hasMore && next && next !== cursor) setCursor(next);
  }, [searchActive, isFetching, hasMore, data, cursor]);

  const renderItem = useCallback(
    ({ item, index }: { item: V1MediaItem; index: number }) => {
      const posterUri = item.id ? artworkUrl(serverUrl, item.id, "poster", CARD_WIDTH * 2, item) : undefined;
      const blurhash = (item.artwork ?? []).find((a) => a.type === "ARTWORK_TYPE_PRIMARY")?.blurhash;
      const vStream = videoStream(item.streams);
      const resLabel = resolutionLabel(vStream);
      const hdr = isHdrStream(vStream);
      const rating = item.communityRating ?? 0;
      const watched = item.id ? watchedMap.get(item.id) === true : false;
      const percent = item.id ? percentMap.get(item.id) : undefined;
      const hasProgress = percent != null && percent > 0;
      return (
        <FocusableCard
          onPress={() => router.push(`/media/${item.id}`)}
          hasTVPreferredFocus={index === 0}
          style={styles.gridItem}
          accessibilityLabel={itemLabel(item, t)}
        >
          <View style={[styles.poster, { borderRadius: theme.radii.sm, backgroundColor: theme.colors.surface, overflow: "hidden" }]}>
            {posterUri ? (
              <PosterImage uri={posterUri} headers={authHeaders} style={StyleSheet.absoluteFill} blurhash={blurhash} recyclingKey={item.id} />
            ) : (
              <Text style={{ color: theme.colors.textMuted, fontSize: 17 }}>?</Text>
            )}

            {rating > 0 && (
              <View
                style={[
                  styles.techBadge,
                  { bottom: hasProgress ? 14 : 6, right: 6, backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm },
                ]}
                pointerEvents="none"
              >
                <Ionicons name="star" size={11} color={theme.colors.warning} />
                <Text style={[styles.techBadgeText, { color: theme.colors.text }]}>{rating.toFixed(1)}</Text>
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

            {item.id && (
              <View
                style={[
                  styles.watchedBadge,
                  { backgroundColor: watched ? theme.colors.success : theme.colors.overlay },
                ]}
                pointerEvents="none"
              >
                <Ionicons name="checkmark" size={12} color={watched ? theme.colors.onSuccess : theme.colors.textMuted} />
              </View>
            )}

            {hasProgress && (
              <View style={[styles.progressTrack, { backgroundColor: theme.colors.border }]} pointerEvents="none">
                <View style={[styles.progressFill, { width: `${Math.min(100, Math.max(0, percent))}%`, backgroundColor: theme.colors.accent }]} />
              </View>
            )}
          </View>
          <Text style={[styles.title, { color: theme.colors.text }]} numberOfLines={2}>
            {itemLabel(item, t)}
          </Text>
        </FocusableCard>
      );
    },
    [theme, router, serverUrl, authHeaders, watchedMap, percentMap, t],
  );

  const activeIsLoading = searchActive ? isSearchLoading : isLoading;
  const activeIsError = searchActive ? isSearchError : isError;
  const activeRefetch = searchActive ? refetchSearch : refetch;
  const isFirstPage = searchActive ? true : cursor === "";

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
      {activeIsError ? (
        <View style={{ flex: 1, justifyContent: "center", alignItems: "center" }}>
          <Text style={{ color: theme.colors.textMuted, fontSize: 20, marginBottom: 12 }}>{t("common.loadError")}</Text>
          {/* A plain Pressable has no hasTVPreferredFocus, so nothing on this
              error state claims initial remote focus - stranding the D-pad.
              FocusableCard matches every other retry action in this app. */}
          <FocusableCard onPress={() => activeRefetch()} hasTVPreferredFocus accessibilityLabel={t("common.retry")}>
            <Text style={{ color: theme.colors.accent, fontSize: 18 }}>{t("common.retry")}</Text>
          </FocusableCard>
        </View>
      ) : (
        <>
          <TextInput
            style={[
              styles.searchInput,
              {
                backgroundColor: theme.colors.surface,
                color: theme.colors.text,
                borderRadius: theme.radii.md,
                borderColor: searchFocused ? theme.colors.accent : theme.colors.border,
              },
            ]}
            placeholder={t("media.library.filterByName")}
            placeholderTextColor={theme.colors.textMuted}
            value={searchText}
            onChangeText={setSearchText}
            autoCapitalize="none"
            autoCorrect={false}
            returnKeyType="search"
            onFocus={() => setSearchFocused(true)}
            onBlur={() => setSearchFocused(false)}
            accessibilityLabel={t("media.library.filterByName")}
          />

          <View style={styles.toolbar}>
            <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ flex: 1 }} contentContainerStyle={{ gap: 10 }}>
              <Chip label={t("media.search.allGenres") || "All"} active={!genre} onPress={() => setGenre("")} />
              {genres.map((g) => (
                <Chip key={g} label={g} active={genre === g} onPress={() => setGenre(genre === g ? "" : g)} />
              ))}
            </ScrollView>
            {canPlayAll && (
              <FocusableCard
                onPress={handlePlayAll}
                style={[styles.shuffleBtn, { backgroundColor: theme.colors.surface }]}
                accessibilityLabel={t("common.playAll")}
              >
                <Ionicons name="play" size={24} color={theme.colors.text} />
                <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "600" }}>{t("common.playAll")}</Text>
              </FocusableCard>
            )}
            <FocusableCard
              onPress={handleShuffle}
              style={[styles.shuffleBtn, { backgroundColor: theme.colors.surface }]}
              accessibilityLabel={t("common.shufflePlay")}
            >
              {shuffling ? (
                <ActivityIndicator color={theme.colors.accent} size="small" />
              ) : (
                <Ionicons name="shuffle" size={24} color={theme.colors.text} />
              )}
              <Text style={{ color: theme.colors.text, fontSize: 17, fontWeight: "600" }}>{t("common.shufflePlay")}</Text>
            </FocusableCard>
          </View>

          <View style={styles.toolbar}>
            <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ flex: 1 }} contentContainerStyle={{ gap: 10 }}>
              {SORT_KEYS.map((s) => (
                <Chip key={s.value} label={t(s.labelKey)} active={sortField === s.value} onPress={() => setSortField(s.value)} />
              ))}
            </ScrollView>
            <FocusableCard
              onPress={() => setSortOrder(sortOrder === "SORT_ORDER_ASC" ? "SORT_ORDER_DESC" : "SORT_ORDER_ASC")}
              style={[styles.shuffleBtn, { backgroundColor: theme.colors.surface }]}
              accessibilityLabel={sortOrder === "SORT_ORDER_ASC" ? t("common.sortAscending") : t("common.sortDescending")}
            >
              <Ionicons name={sortOrder === "SORT_ORDER_ASC" ? "arrow-up" : "arrow-down"} size={22} color={theme.colors.text} />
            </FocusableCard>
          </View>

          {isShowLibrary && (
            <View style={styles.toolbar}>
              <ScrollView horizontal showsHorizontalScrollIndicator={false} style={{ flex: 1 }} contentContainerStyle={{ gap: 10 }}>
                <Chip label={t("media.search.allGenres") || "All"} active={!typeFilter} onPress={() => setTypeFilter("")} />
                {TYPE_KEYS.map((tk) => (
                  <Chip key={tk.value} label={t(tk.labelKey)} active={typeFilter === tk.value} onPress={() => setTypeFilter(typeFilter === tk.value ? "" : tk.value)} />
                ))}
              </ScrollView>
            </View>
          )}

          {activeIsLoading && isFirstPage ? (
            <View style={styles.center}>
              <ActivityIndicator color={theme.colors.accent} size="large" />
            </View>
          ) : (
            <FlatList
              data={displayItems}
              keyExtractor={tvLibKeyExtractor}
              renderItem={renderItem}
              numColumns={NUM_COLUMNS}
              contentContainerStyle={{ padding: 24 }}
              columnWrapperStyle={{ gap: 12, marginBottom: 16 }}
              onEndReached={handleEndReached}
              onEndReachedThreshold={0.5}
              getItemLayout={searchActive ? undefined : tvLibGetItemLayout}
              initialNumToRender={10}
              maxToRenderPerBatch={15}
              windowSize={5}
              removeClippedSubviews={true}
              ListFooterComponent={
                isFetching && !isFirstPage && !searchActive ? (
                  <ActivityIndicator color={theme.colors.accent} style={{ marginVertical: 24 }} />
                ) : null
              }
              ListEmptyComponent={
                <View style={styles.center}>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 20 }}>
                    {hasActiveFilters ? t("media.library.noItemsFiltered") : t("common.noItems")}
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
  center: { flex: 1, justifyContent: "center", alignItems: "center", paddingTop: 80 },
  searchInput: {
    marginHorizontal: 24,
    marginTop: 16,
    paddingHorizontal: 20,
    paddingVertical: 9,
    fontSize: 17,
    borderWidth: 3,
  },
  toolbar: {
    flexDirection: "row",
    alignItems: "center",
    paddingHorizontal: 24,
    paddingTop: 16,
    gap: 12,
  },
  chip: {
    paddingHorizontal: 20,
    paddingVertical: 10,
  },
  shuffleBtn: {
    flexDirection: "row",
    alignItems: "center",
    gap: 8,
    paddingHorizontal: 20,
    paddingVertical: 10,
  },
  gridItem: { width: CARD_WIDTH, alignItems: "center" },
  poster: {
    width: CARD_WIDTH,
    height: POSTER_HEIGHT,
    justifyContent: "center",
    alignItems: "center",
  },
  title: { fontSize: 15, fontWeight: "500", marginTop: 8, textAlign: "center" },
  techBadge: {
    position: "absolute",
    flexDirection: "row",
    alignItems: "center",
    gap: 3,
    paddingHorizontal: 5,
    paddingVertical: 2,
  },
  techBadgeText: { fontSize: 11, fontWeight: "700" },
  techBadgeRow: { position: "absolute", flexDirection: "row", gap: 4 },
  watchedBadge: {
    position: "absolute",
    top: 6,
    right: 6,
    width: 20,
    height: 20,
    borderRadius: 10,
    justifyContent: "center",
    alignItems: "center",
  },
  progressTrack: {
    position: "absolute",
    bottom: 0,
    left: 0,
    right: 0,
    height: 4,
  },
  progressFill: { height: "100%" },
});
