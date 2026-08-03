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
import Ionicons from "@expo/vector-icons/Ionicons";
import { Skeleton } from "../../../src/components/Skeleton";
import { useLocalSearchParams, useRouter, Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import {
  useLibraryServiceGetLibrary,
  useLibraryServiceListMediaItems,
  hasMoreFromCursor,
  defaultTypeFilterFor,
  shouldAutoFetchMore,
  dedicatedRouteFor,
  useLibraryServiceFilterMediaItems,
  useLibraryServiceListGenres,
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
import { MediaCard } from "../../../src/components/MediaCard";
import { useCollections } from "../../../src/api/collections";
import { usePlaylistList, useAddToPlaylist } from "../../../src/api/playlists";
import { artworkUrl } from "../../../src/lib/artwork";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { LibraryFiltersPanel } from "../../../src/components/library/LibraryFiltersPanel";
import {
  collectFilterOptions,
  applyLocalFacetFilters,
  hasServerFilters,
  countActiveFilters,
  buildFilterMediaItemsParams,
  computeNextFilterOffset,
  buildBulkMediaIds,
  selectableMediaIds,
} from "../../../src/components/library/libraryFilterUtils";

const NUM_COLUMNS = 3;
const SPACING = 8;
const screenWidth = Dimensions.get("window").width;
const ITEM_WIDTH = (screenWidth - SPACING * (NUM_COLUMNS + 1)) / NUM_COLUMNS;
const POSTER_HEIGHT = ITEM_WIDTH * 1.5;
const ROW_HEIGHT = POSTER_HEIGHT + 30 + SPACING;
const PAGE_SIZE = 30;

// The dedicated screens this app implements, handed to dedicatedRouteFor so a
// library type is never redirected to a route that does not exist here.
const MOBILE_DEDICATED_ROUTES = ["/photos", "/music"];

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
  // Regular path: KEYSET pagination, driven by the opaque cursor the server
  // returns, exactly like web ($libraryId.lazy.tsx). It used to page by
  // `pagination.page` and stop on `pagination.totalPages`, but the server NEVER
  // populates total_pages (library_handler_mediaitems.go builds the response
  // with TotalItems/Page/PageSize/NextCursor only), so the field is always 0:
  // `0 ?? 1` keeps the 0, `page < 0` is false, and every library froze at its
  // first 30 items. Keyset also costs the DB a constant per page instead of
  // skipping `offset` rows, which matters at 8k items on a weak NAS, and it
  // stays stable while a scan inserts rows underneath us.
  const [cursor, setCursor] = useState("");
  const [allItems, setAllItems] = useState<V1MediaItem[]>([]);
  const [hasMore, setHasMore] = useState(true);
  const prevDataRef = useRef<unknown>(null);

  // Server-filtered path (genres/year/rating - see hasServerFilters below):
  // offset-paginated, mirrors the allItems/page accumulation above but for
  // LibraryService.FilterMediaItems, which has no cursor/page support.
  const [filterOffset, setFilterOffset] = useState(0);
  const [filterItems, setFilterItems] = useState<V1MediaItem[]>([]);
  const [filterNextOffset, setFilterNextOffset] = useState<number | undefined>(undefined);
  const prevFilterDataRef = useRef<unknown>(null);

  const [sortField, setSortField] =
    useState<LibraryServiceListMediaItemsSortField>(
      "MEDIA_SORT_FIELD_TITLE",
    );
  const [sortOrder, setSortOrder] =
    useState<LibraryServiceListMediaItemsSortOrder>("SORT_ORDER_ASC");
  const [genre, setGenre] = useState("");
  const [genres, setGenres] = useState<string[]>([]);
  const [typeFilter, setTypeFilter] = useState<LibraryServiceListMediaItemsType | "">("");
  // Once the user (or an explicit "reset filters") has touched the type
  // filter, the library-type default below must never override it again.
  const typeFilterTouchedRef = useRef(false);
  const handleTypeFilterChange = useCallback((v: LibraryServiceListMediaItemsType | "") => {
    typeFilterTouchedRef.current = true;
    setTypeFilter(v);
  }, []);
  const [searchText, setSearchText] = useState("");
  const debouncedSearch = useDebounce(searchText, 300);
  const [showFilters, setShowFilters] = useState(false);
  const [yearMin, setYearMin] = useState("");
  const [yearMax, setYearMax] = useState("");
  const [minRating, setMinRating] = useState<number | undefined>(undefined);
  const [resolutions, setResolutions] = useState<string[]>([]);
  const [codecs, setCodecs] = useState<string[]>([]);
  const [audioLangs, setAudioLangs] = useState<string[]>([]);
  const [subLangs, setSubLangs] = useState<string[]>([]);
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

  // Music and photo libraries are not browsable through this screen at all:
  // their content is not media_items, so ListMediaItems correctly returns zero
  // rows and the grid renders empty over a full library (production: 3 albums
  // behind a "0 items" Musique). Web has redirected them to their own screens
  // since forever; mobile never did. Handled HERE rather than at the tap site
  // so a deep link or a restored back-stack entry lands right too.
  //
  // replace(), not push(): the empty grid must not stay behind as a back
  // target.
  const dedicated = dedicatedRouteFor(library?.type, MOBILE_DEDICATED_ROUTES);
  useEffect(() => {
    if (dedicated) router.replace(dedicated);
  }, [dedicated, router]);

  // A shows library starts filtered to series-only, a movies library to
  // movies-only - otherwise a shows library's grid mixes series/seasons/
  // episodes on first visit. Skipped once the user has touched the filter.
  useEffect(() => {
    if (!library || typeFilterTouchedRef.current) return;
    const def = defaultTypeFilterFor(library.type);
    if (def) setTypeFilter(def);
  }, [library]);

  const yearFromNum = yearMin ? parseInt(yearMin, 10) : undefined;
  const yearToNum = yearMax ? parseInt(yearMax, 10) : undefined;
  // Only genres (multi)/year/rating have a real LibraryService.FilterMediaItems
  // param (see buildFilterMediaItemsParams) - resolution/codec/audio+subtitle
  // language have none, so they never switch the active endpoint, only the
  // client-side narrowing further down.
  const serverFiltersActive = hasServerFilters({ genres, yearFrom: yearFromNum, yearTo: yearToNum, minRating });

  const {
    data: regularData,
    isLoading: isRegularLoading,
    isError: isRegularError,
    isFetching: isRegularFetching,
    refetch: refetchRegular,
  } = useLibraryServiceListMediaItems(
    {
      libraryId,
      ...(typeFilter ? { type: typeFilter } : {}),
      ...(genre ? { genre } : {}),
      sortField,
      sortOrder,
      "pagination.pageSize": PAGE_SIZE,
      ...(cursor ? { "pagination.cursor": cursor } : {}),
    },
    {
      query: {
        enabled: !!libraryId && !serverFiltersActive,
        // Every cursor is its own cache key, so without this a deep scroll
        // through an 8k-item library would hold ~270 superseded pages in the
        // React Query cache (default gcTime: 5 min) ON TOP of the accumulated
        // allItems - the same rows twice, on a phone. Nothing ever reads a
        // spent cursor again (we only move forward, and any filter/sort change
        // resets to the top), so drop them promptly.
        gcTime: 30_000,
      },
    },
  );

  const {
    data: filterData,
    isLoading: isFilterLoading,
    isError: isFilterError,
    isFetching: isFilterFetching,
    refetch: refetchFilter,
  } = useLibraryServiceFilterMediaItems(
    buildFilterMediaItemsParams({
      libraryId: libraryId ?? "",
      type: typeFilter || undefined,
      genre: genre || undefined,
      genres,
      yearFrom: yearFromNum,
      yearTo: yearToNum,
      minRating,
      sortField,
      sortOrder,
      limit: PAGE_SIZE,
      offset: filterOffset,
    }),
    { query: { enabled: !!libraryId && serverFiltersActive } },
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
      // Matches web's in-library search limit ($libraryId.lazy.tsx:321).
      limit: 120,
    },
    { query: { enabled: searchActive && !!libraryId } },
  );

  // Full distinct genre set for this library (not just what's loaded so far -
  // same gap web's ListGenres endpoint fixed for the desktop chips). Gated on
  // showFilters like web gates it on filtersOpen, to avoid an extra request
  // on every library visit.
  const { data: genresData } = useLibraryServiceListGenres(libraryId ?? "", {
    query: { enabled: !!libraryId && showFilters },
  });
  const availableGenres = useMemo(() => {
    const fetched = genresData?.genres ?? [];
    return Array.from(new Set([...COMMON_GENRES, ...fetched])).sort();
  }, [genresData?.genres]);

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
    setCursor("");
    setAllItems([]);
    setHasMore(true);
    prevDataRef.current = null;
    setFilterOffset(0);
    setFilterItems([]);
    setFilterNextOffset(undefined);
    prevFilterDataRef.current = null;
  }, []);

  useEffect(() => {
    resetAndRefetch();
    // genres/yearMin/yearMax/minRating flip serverFiltersActive, which
    // switches the active endpoint - both accumulations reset together since
    // only one of them is ever fetched at a time (see the enabled: flags
    // above), so resetting the inactive one is a no-op, not a bug.
  }, [sortField, sortOrder, genre, typeFilter, genres, yearMin, yearMax, minRating, resetAndRefetch]);

  useEffect(() => {
    if (!regularData || regularData === prevDataRef.current) return;
    prevDataRef.current = regularData;
    const newItems = regularData.items ?? [];
    if (!cursor) {
      setAllItems(newItems);
    } else {
      setAllItems((prev) => [...prev, ...newItems]);
    }
    setHasMore(hasMoreFromCursor(regularData.pagination));
  }, [regularData, cursor]);

  useEffect(() => {
    if (!filterData || filterData === prevFilterDataRef.current) return;
    prevFilterDataRef.current = filterData;
    const newItems = filterData.items ?? [];
    if (filterOffset === 0) {
      setFilterItems(newItems);
    } else {
      setFilterItems((prev) => [...prev, ...newItems]);
    }
    const total = filterData.total ?? 0;
    const fetchedSoFar = filterOffset + newItems.length;
    setFilterNextOffset(computeNextFilterOffset(fetchedSoFar, newItems.length, total, PAGE_SIZE));
  }, [filterData, filterOffset]);

  const totalItems = searchActive
    ? (searchData?.totalHits ?? 0)
    : serverFiltersActive
      ? (filterData?.total ?? 0)
      : (regularData?.pagination?.totalItems ?? 0);

  // Whatever the active (non-search-narrowed) endpoint returned so far -
  // used both as the display source and as the facet sample below.
  const rawItems = searchActive ? (searchData?.items ?? []) : serverFiltersActive ? filterItems : allItems;

  // Resolution/codec/audio+subtitle language have no server param at all
  // (see LibraryServiceFilterMediaItemsParams) - always narrowed client-side,
  // exactly like web's applyLocalFilters. Genres/minRating already went
  // server-side via FilterMediaItems when !searchActive; SearchMediaRequest
  // has neither, so they get a client top-up only while searching.
  const displayItems = useMemo(
    () =>
      applyLocalFacetFilters(rawItems, {
        resolutions,
        codecs,
        audioLangs,
        subLangs,
        genres: searchActive ? genres : undefined,
        minRating: searchActive ? minRating : undefined,
      }),
    [rawItems, resolutions, codecs, audioLangs, subLangs, searchActive, genres, minRating],
  );

  // Facet options are sampled from whatever's currently loaded (same source
  // the genre chips used before) rather than a dedicated sampling query -
  // kept independent of the facet selections themselves (built from rawItems,
  // not displayItems) so picking one facet doesn't shrink the others' options.
  const availableFacets = useMemo(() => collectFilterOptions(rawItems), [rawItems]);

  const filterCount = useMemo(
    () =>
      countActiveFilters({
        type: typeFilter,
        genre,
        genres,
        resolutions,
        codecs,
        audioLangs,
        subLangs,
        yearFrom: yearFromNum,
        yearTo: yearToNum,
        minRating,
      }),
    [typeFilter, genre, genres, resolutions, codecs, audioLangs, subLangs, yearFromNum, yearToNum, minRating],
  );
  const anyFilterOrSearchActive = filterCount > 0 || searchActive;

  // Unified view over whichever of the three endpoints is currently driving
  // the screen (search / server-filtered / regular list), so the render
  // below doesn't need a 3-way branch at every call site.
  const activeIsLoading = searchActive ? isSearchLoading : serverFiltersActive ? isFilterLoading : isRegularLoading;
  const activeIsFetching = searchActive ? isSearchLoading : serverFiltersActive ? isFilterFetching : isRegularFetching;
  const activeIsError = searchActive ? isSearchError : serverFiltersActive ? isFilterError : isRegularError;
  const activeRefetch = searchActive ? refetchSearch : serverFiltersActive ? refetchFilter : refetchRegular;
  const isFirstPage = searchActive ? true : serverFiltersActive ? filterOffset === 0 : cursor === "";

  const [shuffling, setShuffling] = useState(false);
  // Search results and client-only facets (resolution/codec/language) can't
  // be honored by the random-page probe below (search has no page offset;
  // those facets have no server param at all), so whenever any of them - or
  // a server filter (genres/year/rating) - is active, shuffle draws from
  // what's actually on screen (displayItems) instead. With no such filter,
  // shuffle the WHOLE library via a random page offset (pageSize 1) instead
  // of just the scrolled-in slice - the genre/type quick-filters, if any,
  // still apply server-side since they're baked into the probe request.
  const hasLocalFilters = Boolean(
    debouncedSearch.trim() || resolutions.length || codecs.length || audioLangs.length || subLangs.length || serverFiltersActive,
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
    if (serverFiltersActive) {
      setFilterOffset(0);
      setFilterNextOffset(undefined);
      prevFilterDataRef.current = null;
      return;
    }
    setCursor("");
    setHasMore(true);
    prevDataRef.current = null;
  }, [searchActive, serverFiltersActive, refetchSearch]);

  const handleEndReached = useCallback(() => {
    // Search returns a single fixed-size page (top-N by relevance), no
    // cursor/offset to page through - so scrolling never advances it.
    if (searchActive) return;
    if (serverFiltersActive) {
      if (!isFilterFetching && filterNextOffset != null) setFilterOffset(filterNextOffset);
      return;
    }
    // Advance to the cursor the last page handed us. Guarding on
    // isRegularFetching keeps a fast scroll from firing the same cursor twice
    // (FlatList can call onEndReached again before the fetch resolves).
    const next = regularData?.pagination?.nextCursor;
    if (!isRegularFetching && hasMore && next && next !== cursor) {
      setCursor(next);
    }
  }, [searchActive, serverFiltersActive, isFilterFetching, filterNextOffset, isRegularFetching, hasMore, regularData, cursor]);

  // Whether the active (non-search) endpoint still has more pages - unified
  // over the regular/filter paths so the fetch-until-filled effect below
  // doesn't need its own 3-way branch.
  const canFetchMore = searchActive ? false : serverFiltersActive ? filterNextOffset != null : hasMore;

  // Fetch-until-filled: client-only facets (resolution/codec/language) can
  // narrow the currently loaded page down to a list too short to reach the
  // scroll bottom and trigger onEndReached, leaving a near-empty grid even
  // though more matching items exist further in the library. Mirrors web's
  // effect ($libraryId.lazy.tsx:406-415).
  useEffect(() => {
    if (
      !shouldAutoFetchMore({
        hasLocalFilters,
        hasMore: canFetchMore,
        isFetching: activeIsFetching,
        filteredCount: displayItems.length,
        loadedCount: rawItems.length,
      })
    )
      return;
    handleEndReached();
  }, [hasLocalFilters, canFetchMore, activeIsFetching, displayItems.length, rawItems.length, handleEndReached]);

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
        <View
          style={{ flex: 1, justifyContent: "center", alignItems: "center" }}
        >
          <Text
            style={{ color: theme.colors.textMuted, marginBottom: 12 }}
          >
            {t("common.loadError")}
          </Text>
          <Pressable onPress={() => void activeRefetch()}>
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
                {filterCount ? ` (${filterCount})` : ""}
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
              {anyFilterOrSearchActive
                ? `${displayItems.length} / ${totalItems}`
                : `${totalItems}`}{" "}
              {totalItems === 1 ? "item" : "items"}
            </Text>
          </View>

          {/* Filters bottom sheet */}
          <LibraryFiltersPanel
            visible={showFilters}
            onClose={() => setShowFilters(false)}
            sortField={sortField}
            onSortFieldChange={setSortField}
            sortOrder={sortOrder}
            onSortOrderChange={setSortOrder}
            isShowLibrary={isShowLibrary}
            typeFilter={typeFilter}
            onTypeFilterChange={handleTypeFilterChange}
            genreText={genre}
            onGenreTextChange={setGenre}
            genres={genres}
            onGenresChange={setGenres}
            availableGenres={availableGenres}
            yearMin={yearMin}
            onYearMinChange={setYearMin}
            yearMax={yearMax}
            onYearMaxChange={setYearMax}
            minRating={minRating}
            onMinRatingChange={setMinRating}
            resolutions={resolutions}
            onResolutionsChange={setResolutions}
            codecs={codecs}
            onCodecsChange={setCodecs}
            audioLangs={audioLangs}
            onAudioLangsChange={setAudioLangs}
            subLangs={subLangs}
            onSubLangsChange={setSubLangs}
            availableFacets={availableFacets}
            activeCount={filterCount}
            onReset={() => {
              typeFilterTouchedRef.current = true;
              setTypeFilter("");
              setGenre("");
              setGenres([]);
              setResolutions([]);
              setCodecs([]);
              setAudioLangs([]);
              setSubLangs([]);
              setYearMin("");
              setYearMax("");
              setMinRating(undefined);
            }}
          />

          {/* Grid / List */}
          {(activeIsLoading && isFirstPage) ? (
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
              refreshing={activeIsFetching && isFirstPage}
              onRefresh={handleRefresh}
              onEndReached={handleEndReached}
              onEndReachedThreshold={0.5}
              getItemLayout={libraryGetItemLayout}
              initialNumToRender={9}
              maxToRenderPerBatch={12}
              windowSize={5}
              removeClippedSubviews={true}
              ListFooterComponent={
                activeIsFetching && !isFirstPage ? (
                  <ActivityIndicator
                    color={theme.colors.accent}
                    style={{ marginVertical: 16 }}
                  />
                ) : null
              }
              ListEmptyComponent={
                <View style={styles.center}>
                  <Text style={{ color: theme.colors.textMuted }}>
                    {anyFilterOrSearchActive
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
              refreshing={activeIsFetching && isFirstPage}
              onRefresh={handleRefresh}
              onEndReached={handleEndReached}
              onEndReachedThreshold={0.5}
              initialNumToRender={10}
              maxToRenderPerBatch={12}
              windowSize={5}
              removeClippedSubviews={true}
              ListFooterComponent={
                activeIsFetching && !isFirstPage ? (
                  <ActivityIndicator
                    color={theme.colors.accent}
                    style={{ marginVertical: 16 }}
                  />
                ) : null
              }
              ListEmptyComponent={
                <View style={styles.center}>
                  <Text style={{ color: theme.colors.textMuted }}>
                    {anyFilterOrSearchActive
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
          <View style={styles.bulkBarTopRow}>
            <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }}>{t("common.selectedCount", "{{count}} selected", { count: selected.size })}</Text>
            <View style={{ flexDirection: "row", gap: 14 }}>
              <Pressable onPress={() => setSelected(new Set(selectableMediaIds(displayItems)))} accessibilityRole="button">
                <Text style={{ color: theme.colors.accent, fontSize: 12 }}>{t("common.selectAll")}</Text>
              </Pressable>
              <Pressable onPress={() => setSelected(new Set())} accessibilityRole="button">
                <Text style={{ color: theme.colors.textMuted, fontSize: 12 }}>{t("common.deselectAll")}</Text>
              </Pressable>
            </View>
          </View>
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
        <BulkAddToPlaylistSheet
          visible={showBulkPlaylist}
          mediaItemIds={buildBulkMediaIds(selected)}
          onClose={() => { setShowBulkPlaylist(false); exitBulk(); }}
        />
      )}
      {showBulkCollection && selected.size > 0 && (
        <BulkAddToCollectionSheet
          visible={showBulkCollection}
          mediaItemIds={buildBulkMediaIds(selected)}
          onClose={() => { setShowBulkCollection(false); exitBulk(); }}
        />
      )}
    </View>
  );
}

// The shared AddToPlaylistSheet only accepts a single mediaItemId (built for
// the per-item long-press menu), so bulk add needs its own sheet - mirrors
// BulkAddToCollectionSheet just below and web's BulkPlaylistModal
// (library/BulkPlaylistModal.tsx), both of which add the FULL selection via
// Promise.all instead of a single id (the bug this fixes).
function BulkAddToPlaylistSheet({
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
  const { data: playlists, isLoading } = usePlaylistList();
  const addTo = useAddToPlaylist();
  const manualPlaylists = (playlists ?? []).filter((p) => p.type === "manual");
  const [pendingId, setPendingId] = useState<string | null>(null);

  async function handleAdd(playlistId: string, name: string) {
    setPendingId(playlistId);
    try {
      await Promise.all(mediaItemIds.map((id) => addTo.mutateAsync({ playlistId, mediaItemId: id })));
      toast.success(t("media.library.itemsAddedToPlaylist", { count: mediaItemIds.length, name }));
      onClose();
    } catch {
      toast.error(t("media.library.failedAddPlaylist"));
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
            {t("media.library.addToPlaylist")}
          </Text>

          {isLoading ? (
            <ActivityIndicator color={theme.colors.accent} style={{ marginTop: 24 }} />
          ) : manualPlaylists.length === 0 ? (
            <Text style={[collectionSheetStyles.empty, { color: theme.colors.textMuted }]}>
              {t("media.library.noPlaylists")}
            </Text>
          ) : (
            <FlatList
              data={manualPlaylists}
              keyExtractor={(item) => item.id}
              style={{ maxHeight: 320 }}
              renderItem={({ item }) => (
                <TouchableOpacity
                  style={[collectionSheetStyles.row, { borderBottomColor: theme.colors.border }]}
                  activeOpacity={0.7}
                  onPress={() => void handleAdd(item.id, item.name)}
                  disabled={pendingId !== null}
                >
                  <Ionicons name="list" size={ICON_SIZE.lg} color={theme.colors.accent} />
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
  bulkBar: { paddingHorizontal: 16, paddingVertical: 10, borderTopWidth: StyleSheet.hairlineWidth },
  bulkBarTopRow: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", marginBottom: 8 },
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
