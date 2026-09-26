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
  libraryServiceListMediaItems,
  formatDurationMs,
  itemLabel,
  WATCH_HISTORY_OVERLAY_LIMIT,
} from "@evey/api";
import type {
  V1MediaItem,
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
import { MediaCard, CARD_CAPTION_HEIGHT } from "../../../src/components/MediaCard";
import { artworkUrl } from "../../../src/lib/artwork";
import { LibraryFiltersPanel } from "../../../src/components/library/LibraryFiltersPanel";
import { BulkAddToPlaylistSheet, BulkAddToCollectionSheet } from "../../../src/components/library/BulkAddSheets";
import { useParentalGate } from "../../../src/hooks/useParentalGate";
import { ParentalPINModal } from "../../../src/components/ParentalPINModal";
import { PluginSlot } from "../../../src/features/plugins/PluginSlot";
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
import { videoStream, isHdrStream, resolutionLabel } from "@evey/client";

const NUM_COLUMNS = 3;
const SPACING = 8;
const screenWidth = Dimensions.get("window").width;
const ITEM_WIDTH = (screenWidth - SPACING * (NUM_COLUMNS + 1)) / NUM_COLUMNS;
// MediaCard puts a marginRight of SPACING on its own outer box, so the poster it
// draws is NARROWER than the cell by exactly that much. POSTER_HEIGHT used to be
// derived from ITEM_WIDTH and therefore described a poster nobody drew: ~12px
// taller than the real one. Everything anchored to it was off by that much, and
// getItemLayout - which FlatList TRUSTS instead of measuring - declared a row
// height no row had.
const CARD_WIDTH = ITEM_WIDTH - SPACING;
const POSTER_HEIGHT = CARD_WIDTH * 1.5;
// Title + year under the poster. Taken FROM MediaCard rather than guessed here:
// it is the component that draws those two lines, and it now pins their
// lineHeight so the total is arithmetic instead of whatever leading the platform
// chose. Guessed at 34, it was a couple of pixels short and the cell's clip cut
// the year in half (emulator, 2026-09-05).
const META_HEIGHT = CARD_CAPTION_HEIGHT;
const ROW_HEIGHT = POSTER_HEIGHT + META_HEIGHT + SPACING;
const PAGE_SIZE = 30;

// The dedicated screens this app implements, handed to dedicatedRouteFor so a
// library type is never redirected to a route that does not exist here.
const MOBILE_DEDICATED_ROUTES = ["/photos", "/music"];

function libraryKeyExtractor(item: V1MediaItem) {
  return item.id ?? "";
}

// With numColumns > 1, FlatList batches items into rows and calls getItemLayout
// with the ROW index - not the item index. Dividing by NUM_COLUMNS here divided
// it a second time, so rows 0, 1 and 2 all reported offset 0, rows 3-5 all
// reported ROW_HEIGHT, and so on: the list believed three rows shared one
// vertical position. Scrolling then jumped a screenful at a time and the grid
// looked like it swapped every poster at once instead of flowing ("j'ai pas un
// fil continu mais comme des pages", user report 2026-09-04).
//
// length is the row height for the same reason - a row, not a poster.
const libraryGetItemLayout = (_: unknown, index: number) => ({
  length: ROW_HEIGHT,
  offset: ROW_HEIGHT * index,
  index,
});

function getPoster(item: V1MediaItem, serverUrl: string | undefined): string | undefined {
  return serverUrl && item.id ? artworkUrl(serverUrl, item.id, "poster", 300, item) : undefined;
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
  // One bulk action at a time: the handlers are sequential per-item loops, and
  // a second press while one runs interleaves two loops over the same
  // selection (half watched / half unwatched - a state nobody asked for).
  const [bulkBusy, setBulkBusy] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const isAdmin = useAuthStore((s) => s.isAdmin);

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
    if (bulkBusy) return;
    setBulkBusy(true);
    let failed = 0;
    for (const id of selected) {
      try { await markWatched.mutateAsync({ mediaItemId: id, params: { profileId } }); }
      catch { failed++; }
    }
    if (failed > 0) toast.error(t("media.library.bulkPartialFailure", { count: failed, defaultValue: "{{count}} items failed" }));
    setBulkBusy(false);
    exitBulk();
  }, [selected, markWatched, profileId, exitBulk, t, bulkBusy]);

  const bulkMarkUnwatched = useCallback(async () => {
    if (bulkBusy) return;
    setBulkBusy(true);
    let failed = 0;
    for (const id of selected) {
      try { await markUnwatched.mutateAsync({ mediaItemId: id, params: { profileId } }); }
      catch { failed++; }
    }
    if (failed > 0) toast.error(t("media.library.bulkPartialFailure", { count: failed, defaultValue: "{{count}} items failed" }));
    setBulkBusy(false);
    exitBulk();
  }, [selected, markUnwatched, profileId, exitBulk, t, bulkBusy]);

  const bulkToggleFavorite = useCallback(async () => {
    if (bulkBusy) return;
    setBulkBusy(true);
    try {
      await bulkFavorite.mutateAsync(Array.from(selected));
      toast.success(t("media.favorites.toggled", "Favorites updated"));
    } catch {
      toast.error(t("media.favorites.bulkFailed", "Failed to toggle favorites"));
    }
    setBulkBusy(false);
    exitBulk();
  }, [selected, bulkFavorite, exitBulk, t, bulkBusy]);

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
  // ONLY the genres the library actually contains. A hardcoded English list
  // used to be merged in here - the same one removed from web on 2026-09-04 and
  // left behind on mobile. Providers return genres in the SERVER's language, so
  // a French library listed "Action, Adventure, Animation, Aventure, Comedy,
  // Comédie..." - every genre twice, in two languages. Worse, the English chips
  // that were not also a real genre matched nothing at all: half the filter
  // panel was guaranteed to return an empty library.
  const availableGenres = useMemo(
    () => Array.from(new Set(genresData?.genres ?? [])).sort((a, b) => a.localeCompare(b)),
    [genresData?.genres],
  );

  // Watched status + resume progress for the badge/progress-bar overlay
  // MediaCard already supports. Same source and shape as useWatchedMap
  // (src/hooks/useWatchedMap.ts), read directly here so the percent-complete
  // map can be derived from the same single fetch instead of duplicating it.
  const { data: watchHistoryData } = usePlaybackServiceGetWatchHistory(
    { limit: WATCH_HISTORY_OVERLAY_LIMIT },
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

  // Facet options (resolution, codec, audio and subtitle languages) come from a
  // DEDICATED sample, not from whatever the infinite list happens to have
  // scrolled to.
  //
  // They used to be read off rawItems alone, which on opening the panel is just
  // the first page - 30 items out of a thousand. A viewer therefore saw three
  // audio languages and three subtitle languages, always the same three, and
  // could not filter on anything the top of their library did not happen to
  // contain. Web has fired a dedicated sampling query since the beginning
  // ($libraryId.lazy.tsx, PAGE_SIZE * 4 gated on filtersOpen); this is the same
  // request, gated the same way.
  //
  // Still merged with rawItems: a language the viewer can SEE on screen must
  // never be missing from the panel, whatever the sample happened to catch.
  const { data: facetSample } = useLibraryServiceListMediaItems(
    {
      libraryId,
      ...(typeFilter ? { type: typeFilter } : {}),
      "pagination.pageSize": PAGE_SIZE * 4,
    },
    { query: { enabled: !!libraryId && showFilters } },
  );
  // Built from rawItems rather than displayItems on purpose: picking one facet
  // must not shrink the choices offered by the others.
  const availableFacets = useMemo(
    () => collectFilterOptions([...(facetSample?.items ?? []), ...rawItems]),
    [facetSample?.items, rawItems],
  );

  const filterCount = useMemo(
    () =>
      countActiveFilters({
        // The type a MOVIES library sets on itself is not something the viewer
        // chose, so it must not read as "Filters (1)" on a library nobody has
        // filtered - nor invite a "Clear all (1)" that would un-scope the grid.
        type: typeFilterTouchedRef.current ? typeFilter : "",
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
  // Shuffle jumps straight into the player, so it must pass the same PIN gate
  // as every other play path (the detail page's Play button gates; a shuffle
  // landing on a gated title must not slip past it). The rating varies per
  // draw, hence needsPinFor rather than the hook's fixed-rating needsPin.
  const { needsPinFor, profileId: gateProfileId } = useParentalGate(undefined);
  const [pinPlayTarget, setPinPlayTarget] = useState<string | null>(null);
  const playPick = useCallback((pick: { id?: string; officialRating?: string } | undefined) => {
    if (!pick?.id) { toast.error(t("common.shuffleFailed")); return; }
    if (needsPinFor(pick.officialRating)) { setPinPlayTarget(pick.id); return; }
    router.push(`/(auth)/player/${pick.id}`);
    // needsPinFor is recreated by the hook each render; this callback is only
    // used by tap handlers, never as a memoised child's prop, so that is fine.
  }, [needsPinFor, router, t]);
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
      playPick(pick);
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
      playPick(pick);
    } catch {
      toast.error(t("common.shuffleFailed"));
    } finally {
      setShuffling(false);
    }
  }, [shuffling, libraryId, hasLocalFilters, displayItems, typeFilter, genre, playPick, t]);

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
        <View style={{ width: ITEM_WIDTH, height: POSTER_HEIGHT + META_HEIGHT, overflow: "hidden", marginLeft: SPACING, marginBottom: SPACING }}>
          <View style={{ opacity: bulkMode && !isSelected ? 0.5 : 1 }}>
            <MediaCard
              item={item}
              width={CARD_WIDTH}
              watchPercent={watchPercent}
              watched={watched}
            />
            {/* Badge layer sized to the POSTER, not to the cell. Anchored to the
                cell they were `bottom: 6` of poster + title + year, so the
                quality chip rendered under the artwork, in the gap before the
                next row - "la qualité est entre 2 vignettes" (user, 2026-09-05).
                Clipping the cell alone would not have fixed it: it would have
                hidden them instead. */}
            <View
              style={{ position: "absolute", top: 0, left: 0, width: CARD_WIDTH, height: POSTER_HEIGHT }}
              pointerEvents="none"
            >
              {rating > 0 && (
                <View
                  style={[
                    styles.techBadge,
                    { bottom: hasProgress ? 14 : 6, right: 6, backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm },
                  ]}
                >
                  <Ionicons name="star" size={10} color={theme.colors.warning} />
                  <Text style={[styles.techBadgeText, { color: theme.colors.text }]}>
                    {rating.toFixed(1)}
                  </Text>
                </View>
              )}
              {(resLabel || hdr) && (
                <View style={[styles.techBadgeRow, { bottom: hasProgress ? 14 : 6, left: 6 }]}>
                  {resLabel && (
                    <View style={[styles.techBadge, { position: "relative", backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.sm }]}>
                      <Text style={[styles.techBadgeText, { color: theme.colors.text }]}>{resLabel}</Text>
                    </View>
                  )}
                  {hdr && (
                    <View style={[styles.techBadge, { position: "relative", backgroundColor: theme.colors.accent, borderRadius: theme.radii.sm }]}>
                      <Text style={[styles.techBadgeText, { color: theme.colors.onAccent }]}>HDR</Text>
                    </View>
                  )}
                </View>
              )}
            </View>
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
            accessibilityLabel={itemLabel(item, t)}
          />
        </View>
      );
    },
    [theme, router, bulkMode, selected, watchedMap, percentMap, toggleSelect, handleLongPress, t],
  );

  const renderListItem = useCallback(
    ({ item }: { item: V1MediaItem }) => {
      const poster = getPoster(item, serverUrl);
      const isSelected = bulkMode && selected.has(item.id ?? "");
      const label = itemLabel(item, t);
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
              {label}
            </Text>
            <Text style={{ fontSize: 12, marginTop: 2, color: theme.colors.textMuted }}>
              {/* Same facts as the web list view: year, type, community
                  rating and runtime (LibraryListView.tsx). */}
              {[
                item.year,
                formatType(item.type),
                item.communityRating != null && item.communityRating > 0
                  ? `★ ${item.communityRating.toFixed(1)}`
                  : null,
                formatDurationMs(item.durationMs) || null,
              ]
                .filter(Boolean)
                .join(" · ")}
            </Text>
          </View>
        </TouchableOpacity>
      );
    },
    [theme, router, authHeaders, serverUrl, bulkMode, selected, toggleSelect, handleLongPress, t],
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

          {/* Plugins mounting on the library page - same strip-above-the-grid
              placement as web. */}
          <PluginSlot name="library.sidebar" context={{ libraryId }} />

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
                    {/* Same footprint as a real cell, or the grid jumps the
                        moment the first page lands. */}
                    <Skeleton
                      width={CARD_WIDTH}
                      height={POSTER_HEIGHT}
                      borderRadius={theme.radii.sm}
                    />
                    <Skeleton
                      width={CARD_WIDTH * 0.7}
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
            {isAdmin && selected.size === 1 && (
              <Pressable
                onPress={() => { const [only] = Array.from(selected); exitBulk(); router.push(`/media/${only}?edit=1`); }}
                style={styles.bulkBtn}
                accessible
                accessibilityLabel={t("media.contextMenu.editMetadata")}
                accessibilityRole="button"
              >
                <Ionicons name="create-outline" size={20} color={theme.colors.accent} />
                <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.contextMenu.editMetadata")}</Text>
              </Pressable>
            )}
            <Pressable onPress={bulkMarkWatched} disabled={bulkBusy} style={[styles.bulkBtn, bulkBusy && { opacity: 0.4 }]} accessible accessibilityLabel={t("media.library.markWatched", "Mark watched")} accessibilityRole="button">
              <Ionicons name="checkmark-circle-outline" size={20} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.library.markWatched", "Watched")}</Text>
            </Pressable>
            <Pressable onPress={bulkMarkUnwatched} disabled={bulkBusy} style={[styles.bulkBtn, bulkBusy && { opacity: 0.4 }]} accessible accessibilityLabel={t("media.library.markUnwatched", "Mark unwatched")} accessibilityRole="button">
              <Ionicons name="eye-off-outline" size={20} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.library.markUnwatched", "Unwatched")}</Text>
            </Pressable>
            <Pressable onPress={bulkToggleFavorite} disabled={bulkBusy} style={[styles.bulkBtn, bulkBusy && { opacity: 0.4 }]} accessible accessibilityLabel={t("media.favorites.toggleFavorite", "Favorite")} accessibilityRole="button">
              <Ionicons name="heart-outline" size={20} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.favorites.toggleFavorite", "Favorite")}</Text>
            </Pressable>
            <Pressable onPress={() => setShowBulkPlaylist(true)} disabled={bulkBusy} style={[styles.bulkBtn, bulkBusy && { opacity: 0.4 }]} accessible accessibilityLabel={t("media.detail.addToPlaylist", "Playlist")} accessibilityRole="button">
              <Ionicons name="add-circle-outline" size={20} color={theme.colors.accent} />
              <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.detail.addToPlaylist", "Playlist")}</Text>
            </Pressable>
            {isAdmin && (
              <Pressable onPress={() => setShowBulkCollection(true)} disabled={bulkBusy} style={[styles.bulkBtn, bulkBusy && { opacity: 0.4 }]} accessible accessibilityLabel={t("media.library.addToCollection", "Add to collection")} accessibilityRole="button">
                <Ionicons name="albums-outline" size={20} color={theme.colors.accent} />
                <Text style={{ color: theme.colors.text, fontSize: 11 }}>{t("media.library.addToCollection", "Add to collection")}</Text>
              </Pressable>
            )}
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
      {gateProfileId && pinPlayTarget ? (
        <ParentalPINModal
          visible={!!pinPlayTarget}
          onClose={() => setPinPlayTarget(null)}
          onVerified={() => {
            const target = pinPlayTarget;
            setPinPlayTarget(null);
            if (target) router.push(`/(auth)/player/${target}`);
          }}
          profileId={gateProfileId}
        />
      ) : null}
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
