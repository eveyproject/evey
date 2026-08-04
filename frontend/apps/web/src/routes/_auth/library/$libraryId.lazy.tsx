import { useState, useEffect, useMemo, useCallback } from "react";
import { Folder, ChevronRight, Shuffle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { createLazyFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  useLibraryServiceListGenres,
  libraryServiceListMediaItems,
  libraryServiceFilterMediaItems,
  libraryServiceSearchMedia,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
  dedicatedRouteFor,
  defaultTypeFilterFor,
  shouldAutoFetchMore,
  IN_LIBRARY_SEARCH_LIMIT,
} from "@evey/api";
import type {
  LibraryServiceListMediaItemsType,
  LibraryServiceListMediaItemsSortField,
  LibraryServiceSearchMediaType,
  LibraryServiceSearchMediaSortField,
  LibraryServiceSearchMediaSortOrder,
  V1MediaItem,
} from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Select";
import { Input } from "@/components/ui/Input";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { MediaGridSkeleton } from "@/components/ui/Skeleton";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCardWithModals as MediaCard } from "@/features/library/MediaCardWithModals";
import {
  LibraryFilterToggle,
  LibraryFiltersPanel,
  collectFilterOptions,
  applyLocalFilters,
} from "@/features/library/LibraryFilters";
import type { LibrarySearch } from "@/routes/_auth/library/$libraryId";
import { slugify } from "@/utils/slugify";
import { toast, useAuthStore } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";
import { favoriteServiceAddFavorite, getFavoriteServiceListFavoritesQueryKey } from "@evey/api";
import { BulkActionBar } from "@/components/BulkActionBar";
import { AddToCollectionModal } from "@/features/collection/AddToCollectionModal";
import { SortOrderButton, ViewToggleButton } from "./LibraryToolbar";
import { LibraryListView } from "./LibraryListView";
import { BulkPlaylistModal } from "./BulkPlaylistModal";
import { PluginSlot } from "@/features/plugins/PluginSlot";

export const Route = createLazyFileRoute("/_auth/library/$libraryId")({
  component: LibraryPage,
});

const SHOW_TYPE_KEYS: Array<{ labelKey: string; value: LibraryServiceListMediaItemsType | "" }> = [
  { labelKey: "media.library.series", value: "MEDIA_ITEM_TYPE_SERIES" },
  { labelKey: "media.library.seasons", value: "MEDIA_ITEM_TYPE_SEASON" },
  { labelKey: "media.library.episodes", value: "MEDIA_ITEM_TYPE_EPISODE" },
];

const SORT_KEYS: Array<{ labelKey: string; value: LibraryServiceListMediaItemsSortField }> = [
  { labelKey: "media.library.titleSort", value: "MEDIA_SORT_FIELD_TITLE" },
  { labelKey: "media.library.year", value: "MEDIA_SORT_FIELD_YEAR" },
  { labelKey: "media.library.ratingSort", value: "MEDIA_SORT_FIELD_RATING" },
  { labelKey: "media.library.dateAdded", value: "MEDIA_SORT_FIELD_DATE_ADDED" },
];

const PAGE_SIZE = 48;

// The dedicated screens this app implements, handed to dedicatedRouteFor so a
// library type is never redirected to a route that does not exist here.
const WEB_DEDICATED_ROUTES = ["/photos", "/music"];

// AUTO_FETCH_*, DEDICATED_ROUTES and DEFAULT_TYPE_FILTER used to be declared
// here and hand-copied into mobile/TV. They live in @evey/api (libraryRules.ts)
// now, so the three clients cannot drift; rulesGuard.test.ts fails the build if
// a local copy reappears.

function LibraryPage() {
  const { t } = useTranslation();
    const { libraryId: slug } = Route.useParams();
  const search = Route.useSearch();
  const navigate = useNavigate();

  const [viewMode, setViewMode] = useState<"grid" | "list">("grid");
  const [bulkMode, setBulkMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [bulkPlaylistOpen, setBulkPlaylistOpen] = useState(false);
  const [bulkCollectionOpen, setBulkCollectionOpen] = useState(false);
  const [bulkFavPending, setBulkFavPending] = useState(false);
  const profileId = useAuthStore((s) => s.profileId);
  const isAdmin = useAuthStore((s) => s.isAdmin);
  const queryClient = useQueryClient();

  const updateSearch = useCallback(
    (patch: Partial<LibrarySearch>) => {
      void navigate({
        to: "/library/$libraryId",
        params: { libraryId: slug },
        search: { ...search, ...patch },
        replace: true,
      });
      // No setPage(1) needed: infinite query keys include filter/sort/type,
      // so changing any of them naturally starts a fresh paged sequence.
    },
    [navigate, search, slug],
  );

  const typeFilter = search.type;
  const genreFilter = search.genre ?? "";
  const sortField = search.sortField ?? "MEDIA_SORT_FIELD_TITLE";
  const sortOrder = search.sortOrder ?? "SORT_ORDER_ASC";
  const searchTerm = search.q ?? "";

  const handleSelect = useCallback((id: string, sel: boolean) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (sel) next.add(id);
      else next.delete(id);
      return next;
    });
  }, []);

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape" && bulkMode) {
        setBulkMode(false);
        setSelectedIds(new Set());
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [bulkMode]);

  const markWatched = usePlaybackServiceMarkAsWatched();
  const markUnwatched = usePlaybackServiceMarkAsUnwatched();

  async function bulkMarkWatched(watched: boolean) {
    if (!profileId || selectedIds.size === 0) return;
    const ids = Array.from(selectedIds);
    const fn = watched ? markWatched.mutateAsync : markUnwatched.mutateAsync;
    try {
      await Promise.all(ids.map((mediaItemId) => fn({ mediaItemId, params: { profileId } })));
      ids.forEach((id) => queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${id}`] }));
      // The grid cards render watched status from the library LIST queries, not the
      // per-item detail query, so those must be invalidated too - otherwise the
      // watched badges stay stale after a bulk mark until reload.
      queryClient.invalidateQueries({ queryKey: ["lib-infinite"] });
      queryClient.invalidateQueries({ queryKey: ["lib-infinite-filter"] });
      queryClient.invalidateQueries({ queryKey: ["lib-search"] });
      toast.success(watched ? t("media.favorites.markedWatched", { count: ids.length }) : t("media.favorites.markedUnwatched", { count: ids.length }));
      setSelectedIds(new Set());
      setBulkMode(false);
    } catch {
      // Without this, a rejected Promise.all left the selection + bulk bar stuck
      // with no feedback and skipped the cache invalidations above.
      toast.error(t("common.error"));
    }
  }

  async function bulkAddFavorites() {
    if (selectedIds.size === 0) return;
    setBulkFavPending(true);
    const ids = Array.from(selectedIds);
    try {
      await Promise.all(ids.map((id) => favoriteServiceAddFavorite(id, {})));
      // Use the orval-generated key (["/api/v1/favorites", …]); the bare
      // ["favorites"] prefix never matched, so the Favorites list/feed stayed stale.
      queryClient.invalidateQueries({ queryKey: getFavoriteServiceListFavoritesQueryKey() });
      toast.success(t("media.library.addedToFavorites", { count: ids.length }));
      setSelectedIds(new Set());
      setBulkMode(false);
    } catch {
      toast.error(t("media.library.failedFavorites"));
    } finally {
      setBulkFavPending(false);
    }
  }

  const { data: librariesData } = useLibraryServiceListLibraries();
  const library = librariesData?.libraries?.find(
    (l) => slugify(l.name ?? "") === slug
  );
  const libraryId = library?.id ?? "";
  const isShowLibrary = library?.type === "LIBRARY_TYPE_SHOWS";

  useEffect(() => {
    if (!library) return;
    const dedicated = dedicatedRouteFor(library.type, WEB_DEDICATED_ROUTES);
    if (dedicated) { void navigate({ to: dedicated, replace: true }); return; }
    if (typeFilter !== undefined) return;
    const type = defaultTypeFilterFor(library.type);
    if (type) void navigate({ to: "/library/$libraryId", params: { libraryId: slug }, search: { ...search, type }, replace: true });
  // eslint-disable-next-line react-hooks/exhaustive-deps -- search/slug excluded: search is a new object each render, slug is stable but derived from params already covered
  }, [library?.type, typeFilter, navigate]);

  const typeOptions = isShowLibrary ? SHOW_TYPE_KEYS : [];

  const hasServerFilters = Boolean(
    search.genres?.length ||
    search.yearFrom != null ||
    search.yearTo != null ||
    (search.minRating != null && search.minRating > 0),
  );

  // Regular list path uses keyset pagination (opaque cursor returned by the
  // server). useInfiniteQuery appends pages instead of refetching everything
  // with a growing pageSize, so scroll position is preserved and bandwidth
  // is O(n) instead of O(n²).
  const regularKey = useMemo(
    () => ["lib-infinite", libraryId, typeFilter ?? "", genreFilter, sortField, sortOrder] as const,
    [libraryId, typeFilter, genreFilter, sortField, sortOrder],
  );
  const regularQuery = useInfiniteQuery({
    queryKey: regularKey,
    enabled: !hasServerFilters && !!libraryId,
    initialPageParam: "" as string,
    queryFn: ({ pageParam, signal }) =>
      libraryServiceListMediaItems(
        {
          libraryId,
          ...(typeFilter ? { type: typeFilter } : {}),
          ...(genreFilter ? { genre: genreFilter } : {}),
          sortField,
          sortOrder,
          "pagination.pageSize": PAGE_SIZE,
          ...(pageParam ? { "pagination.cursor": pageParam } : {}),
        },
        signal,
      ),
    getNextPageParam: (last, allPages) => {
      const cursor = last.pagination?.nextCursor;
      if (cursor) return cursor;
      // Server falls back to offset when cursor isn't available - stop when
      // the last page wasn't a full slice (no more rows beyond it).
      const got = last.items?.length ?? 0;
      if (got < PAGE_SIZE) return undefined;
      const total = last.pagination?.totalItems ?? 0;
      const fetched = allPages.reduce((n, p) => n + (p.items?.length ?? 0), 0);
      return fetched < total ? String(fetched) : undefined;
    },
  });

  // Filter endpoint uses offset/limit (no cursor support yet); we page by
  // offset = sum(items fetched so far). Same UX as the regular path.
  const filterKey = useMemo(
    () =>
      [
        "lib-infinite-filter",
        libraryId,
        typeFilter ?? "",
        genreFilter,
        (search.genres ?? []).join("|"),
        search.yearFrom ?? null,
        search.yearTo ?? null,
        search.minRating ?? null,
        sortField,
        sortOrder,
      ] as const,
    [libraryId, typeFilter, genreFilter, search.genres, search.yearFrom, search.yearTo, search.minRating, sortField, sortOrder],
  );
  const filterQuery = useInfiniteQuery({
    queryKey: filterKey,
    enabled: hasServerFilters && !!libraryId,
    initialPageParam: 0,
    queryFn: ({ pageParam, signal }) =>
      libraryServiceFilterMediaItems(
        {
          libraryId,
          ...(typeFilter ? { type: typeFilter } : {}),
          ...(genreFilter ? { genre: genreFilter } : {}),
          ...(search.genres?.length ? { genres: search.genres } : {}),
          ...(search.yearFrom != null ? { yearFrom: search.yearFrom } : {}),
          ...(search.yearTo != null ? { yearTo: search.yearTo } : {}),
          ...(search.minRating != null && search.minRating > 0 ? { minRating: search.minRating } : {}),
          sortField,
          sortOrder,
          limit: PAGE_SIZE,
          offset: pageParam as number,
        },
        signal,
      ),
    getNextPageParam: (last, allPages) => {
      const fetched = allPages.reduce((n, p) => n + (p.items?.length ?? 0), 0);
      const total = last.total ?? 0;
      const got = last.items?.length ?? 0;
      if (got < PAGE_SIZE || fetched >= total) return undefined;
      return fetched;
    },
  });

  // Server-side title search when the input is non-empty. Replaces the
  // previously cosmetic client-side filter that only matched items already
  // paginated into memory - typing "Matrix" in a 5k-item library returned
  // nothing because Matrix wasn't in the loaded slice. Server search hits
  // Meilisearch (or the SQL fallback) with library_id scoping.
  const trimmedSearch = searchTerm.trim();
  const searchActive = trimmedSearch.length > 0;
  const searchQuery = useQuery({
    queryKey: ["lib-search", libraryId, trimmedSearch, typeFilter ?? "", genreFilter, search.yearFrom ?? null, search.yearTo ?? null, sortField, sortOrder],
    enabled: searchActive && !!libraryId,
    queryFn: ({ signal }) =>
      libraryServiceSearchMedia(
        {
          libraryId,
          query: trimmedSearch,
          ...(typeFilter ? { type: typeFilter as unknown as LibraryServiceSearchMediaType } : {}),
          ...(genreFilter ? { genre: genreFilter } : {}),
          ...(search.yearFrom != null ? { yearMin: search.yearFrom } : {}),
          ...(search.yearTo != null ? { yearMax: search.yearTo } : {}),
          // Honor the toolbar sort while searching (the API supports it);
          // without this the query key also ignored sort, so changing sort
          // mid-search returned stale cached results.
          sortField: sortField as unknown as LibraryServiceSearchMediaSortField,
          sortOrder: sortOrder as unknown as LibraryServiceSearchMediaSortOrder,
          limit: IN_LIBRARY_SEARCH_LIMIT,
        },
        signal,
      ),
  });

  const activeQuery = hasServerFilters ? filterQuery : regularQuery;
  const isLoading = searchActive ? searchQuery.isLoading : activeQuery.isLoading;
  // Whichever query is actually driving the grid: a failure here used to render
  // "this library is empty", sending an admin hunting for a scan problem that
  // did not exist.
  const activeIsError = searchActive ? searchQuery.isError : activeQuery.isError;
  const activeError = searchActive ? searchQuery.error : activeQuery.error;
  const activeRefetch = () => { void (searchActive ? searchQuery.refetch() : activeQuery.refetch()); };

  const items: V1MediaItem[] = useMemo(() => {
    if (searchActive) return searchQuery.data?.items ?? [];
    return activeQuery.data?.pages.flatMap((p) => p.items ?? []) ?? [];
  }, [searchActive, searchQuery.data, activeQuery.data]);
  const totalItems = searchActive
    ? (searchQuery.data?.totalHits ?? 0)
    : hasServerFilters
      ? (filterQuery.data?.pages[0]?.total ?? 0)
      : (regularQuery.data?.pages[0]?.pagination?.totalItems ?? 0);
  // Search returns top-N in a single response, no infinite scroll.
  const hasMore = searchActive ? false : (activeQuery.hasNextPage ?? false);

  const { data: metaData } = useLibraryServiceListMediaItems({
    libraryId,
    ...(typeFilter ? { type: typeFilter } : {}),
    "pagination.page": 1,
    "pagination.pageSize": PAGE_SIZE * 4,
  }, { query: { enabled: !!libraryId && filtersOpen } });

  // Genres come from a dedicated endpoint that returns the library's full
  // distinct set, not just the genres present in the sampled first pages
  // (metaData) - otherwise the filter chips silently missed genres.
  const { data: genresData } = useLibraryServiceListGenres(libraryId, {
    query: { enabled: !!libraryId && filtersOpen },
  });
  const genres = useMemo(() => genresData?.genres ?? [], [genresData?.genres]);

  const availableOptions = useMemo(
    () => collectFilterOptions(metaData?.items ?? []),
    [metaData?.items],
  );

  const localFiltered = useMemo(() => {
    // Title-based search runs server-side via searchQuery now - no client
    // re-filter needed here, and it would actually narrow results since
    // searchQuery's top-N may include items whose title contains the term
    // via the search index's looser matching (synonyms, accent folding).
    return applyLocalFilters(items, {
      // Genres are filtered server-side via FilterMediaItems (genres=[...]).
      // The only path that doesn't is text search (SearchMedia has no
      // multi-genre param), so re-filter genres client-side only then.
      genres: searchActive ? search.genres : undefined,
      resolutions: search.resolutions,
      codecs: search.codecs,
      audioLangs: search.audioLangs,
      subLangs: search.subLangs,
      yearFrom: hasServerFilters || searchActive ? undefined : search.yearFrom,
      yearTo: hasServerFilters || searchActive ? undefined : search.yearTo,
      minRating: hasServerFilters ? undefined : search.minRating,
    });
  }, [items, search.genres, search.resolutions, search.codecs, search.audioLangs, search.subLangs, search.yearFrom, search.yearTo, search.minRating, hasServerFilters, searchActive]);

  const hasLocalFilters = Boolean(
    searchTerm.trim() ||
    (searchActive && search.genres?.length) ||
    search.resolutions?.length ||
    search.codecs?.length ||
    search.audioLangs?.length ||
    search.subLangs?.length ||
    (!hasServerFilters && search.yearFrom != null) ||
    (!hasServerFilters && search.yearTo != null) ||
    (!hasServerFilters && search.minRating != null && search.minRating > 0),
  );
  const hasActiveFilters = hasLocalFilters || hasServerFilters;
  const displayItems = hasLocalFilters ? localFiltered : items;
  const displayCount = hasLocalFilters ? localFiltered.length : totalItems;


  // Proactive fetch-until-filled: when client-side facets (resolution, codec,
  // audio/sub lang…) are active the filtered list can be short - not tall
  // enough to reach the scroll bottom and trigger onEndReached. We therefore
  // fetch the next server page proactively until enough matches are visible
  // or the loaded-item cap is hit (shouldAutoFetchMore, @evey/api).
  // isFetchingNextPage prevents overlapping fetches; hasMore short-circuits
  // when the server has no further pages (including when searchActive, because
  // hasMore is false for the one-shot search path).
  useEffect(() => {
    if (
      !shouldAutoFetchMore({
        hasLocalFilters,
        hasMore,
        isFetching: activeQuery.isFetchingNextPage,
        filteredCount: localFiltered.length,
        loadedCount: items.length,
      })
    ) return;
    void activeQuery.fetchNextPage();
  }, [hasLocalFilters, hasMore, activeQuery.isFetchingNextPage, activeQuery.fetchNextPage, localFiltered.length, items.length]);

  const [randomLoading, setRandomLoading] = useState(false);
  // Shuffle play: start one random item from the current view. A movie plays
  // straight away; a series goes to its detail so the user picks an episode.
  const playRandomPick = useCallback((pick?: V1MediaItem) => {
    const id = pick?.id;
    if (!id) { toast.error(t("common.shuffleFailed")); return; }
    if (pick?.type === "MEDIA_ITEM_TYPE_MOVIE") {
      navigate({ to: "/player/$mediaId", params: { mediaId: id } });
    } else {
      navigate({ to: "/media/$mediaId", params: { mediaId: id } });
    }
  }, [navigate, t]);
  const handleRandomPlay = useCallback(async () => {
    if (randomLoading || !libraryId) return;
    // With a search or panel filters active, shuffle within what's on screen so
    // the pick honors every facet (multi-genre, resolution, year…). With no
    // such filter, shuffle the WHOLE library via a random page offset (pageSize
    // 1, page = offset+1) so it isn't limited to the scrolled-in slice - the
    // genre deep-link, if any, is still applied server-side.
    if (hasActiveFilters || searchActive) {
      if (displayItems.length === 0) { toast.info(t("common.shuffleEmpty")); return; }
      playRandomPick(displayItems[Math.floor(Math.random() * displayItems.length)]);
      return;
    }
    setRandomLoading(true);
    try {
      const base = {
        libraryId,
        ...(typeFilter ? { type: typeFilter } : {}),
        ...(genreFilter ? { genre: genreFilter } : {}),
      };
      const probe = await libraryServiceListMediaItems({ ...base, "pagination.page": 1, "pagination.pageSize": 1 });
      const total = probe.pagination?.totalItems ?? 0;
      if (total === 0) { toast.info(t("common.shuffleEmpty")); return; }
      const page = Math.floor(Math.random() * total) + 1;
      const res = page === 1 ? probe : await libraryServiceListMediaItems({ ...base, "pagination.page": page, "pagination.pageSize": 1 });
      playRandomPick(res.items?.[0]);
    } catch {
      toast.error(t("common.shuffleFailed"));
    } finally {
      setRandomLoading(false);
    }
  }, [randomLoading, libraryId, hasActiveFilters, searchActive, displayItems, typeFilter, genreFilter, playRandomPick, t]);

  return (
    <div data-testid="library-page">
      <nav className="flex items-center gap-1.5 text-sm mb-3" aria-label={t("common.aria.breadcrumb")}>
        <Link to="/library" className="hover:underline text-[var(--text-muted)]">
          {t("media.library.libraries")}
        </Link>
        <ChevronRight size={12} className="flex-shrink-0 text-[var(--text-muted)]" />
        <span className="truncate font-medium text-[var(--text)]">
          {library?.name ?? t("media.library.title")}
        </span>
      </nav>
      <div className="flex items-center justify-between gap-4 mb-2 flex-wrap">
        <h1 className="text-2xl font-bold text-[var(--text)]" data-testid="library-name">
          {library?.name ?? t("media.library.title")}
        </h1>
        <span className="text-sm text-[var(--text-muted)]" data-testid="library-item-count">
          {t("common.itemCount", "{{count}} item", { count: displayCount })}
          {hasActiveFilters && ` ${t("common.filtered")}`}
        </span>
      </div>

      <div className="flex items-end gap-3 mb-6 flex-wrap">
        <div className="w-48">
          <Input
            type="search"
            placeholder={t("media.library.filterByName")}
            value={searchTerm}
            onChange={(e) => updateSearch({ q: e.target.value || undefined })}
            data-testid="library-search-input"
          />
        </div>
        {typeOptions.length > 0 && (
          <Select
            value={typeFilter ?? ""}
            onChange={(v) => updateSearch({ type: (v as LibraryServiceListMediaItemsType) || undefined })}
            options={typeOptions.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
            className="w-auto"
          />
        )}
        <Select
          value={sortField}
          onChange={(v) => updateSearch({ sortField: v as LibraryServiceListMediaItemsSortField })}
          options={SORT_KEYS.map((o) => ({ value: o.value, label: t(o.labelKey) }))}
          className="w-auto"
        />
        <SortOrderButton
          order={sortOrder}
          onChange={(o) => updateSearch({ sortOrder: o })}
        />
        <Button
          variant="secondary"
          onClick={handleRandomPlay}
          loading={randomLoading}
          title={t("common.shufflePlay")}
          data-testid="library-shuffle-play"
        >
          <Shuffle size={16} />
          {t("common.shufflePlay")}
        </Button>
        <LibraryFilterToggle search={search} resultCount={hasLocalFilters || hasServerFilters ? displayCount : undefined} onClick={() => setFiltersOpen((v) => !v)} />
        <ViewToggleButton mode={viewMode} onChange={setViewMode} />
        <button
          type="button"
          onClick={() => { setBulkMode((v) => !v); setSelectedIds(new Set()); }}
          className="p-2 rounded-[var(--radius-md)] border transition-colors-fast text-sm"
          style={{
            backgroundColor: bulkMode ? "var(--accent)" : "var(--surface-raised)",
            color: bulkMode ? "var(--filter-active-text)" : "var(--text-muted)",
            borderColor: bulkMode ? "var(--accent)" : "var(--border)",
          }}
          title={bulkMode ? t("common.exit") : t("common.select")}
        >
          {t("common.select")}
        </button>
      </div>

      {bulkPlaylistOpen && (
        <BulkPlaylistModal
          open={bulkPlaylistOpen}
          mediaIds={Array.from(selectedIds)}
          onClose={() => {
            setBulkPlaylistOpen(false);
            setSelectedIds(new Set());
            setBulkMode(false);
          }}
        />
      )}

      {bulkCollectionOpen && (
        <AddToCollectionModal
          open={bulkCollectionOpen}
          mediaItemIds={Array.from(selectedIds)}
          onClose={() => {
            setBulkCollectionOpen(false);
            setSelectedIds(new Set());
            setBulkMode(false);
          }}
        />
      )}

      {bulkMode && (
        <>
          <div
            className="sticky top-0 z-raised flex items-center gap-2 px-4 py-2 mb-4 rounded-[var(--radius-md)] flex-wrap bg-[var(--surface)] border border-[var(--accent)]"
          >
            <span
              className="text-sm font-semibold px-2 py-0.5 rounded-full mr-1 bg-[var(--accent)] text-[var(--filter-active-text)]"
            >
              {selectedIds.size}
            </span>
            <span className="text-sm mr-2 text-[var(--text-muted)]">{t("media.library.selected")}</span>
            {selectedIds.size > 0 && (
              <>
                <Button
                  variant="secondary"
                  loading={bulkFavPending}
                  onClick={() => void bulkAddFavorites()}
                >
                  {t("media.library.addToFavorites")}
                </Button>
              </>
            )}
            <div className="flex items-center gap-2 ml-auto">
              <button
                type="button"
                onClick={() => setSelectedIds(new Set(displayItems.map((i) => i.id ?? "").filter(Boolean)))}
                className="text-xs hover:underline text-[var(--accent)]"
              >
                {t("common.selectAll")}
              </button>
              <button
                type="button"
                onClick={() => setSelectedIds(new Set())}
                className="text-xs hover:underline text-[var(--text-muted)]"
              >
                {t("common.deselectAll")}
              </button>
              <button
                type="button"
                onClick={() => { setBulkMode(false); setSelectedIds(new Set()); }}
                className="text-xs hover:underline text-[var(--text-muted)]"
              >
                {t("common.exit")}
              </button>
            </div>
          </div>
          <BulkActionBar
            count={selectedIds.size}
            onClear={() => { setSelectedIds(new Set()); setBulkMode(false); }}
            onMarkWatched={() => void bulkMarkWatched(true)}
            onMarkUnwatched={() => void bulkMarkWatched(false)}
            onAddToPlaylist={() => setBulkPlaylistOpen(true)}
            onAddToCollection={() => setBulkCollectionOpen(true)}
            markWatchedPending={markWatched.isPending}
            markUnwatchedPending={markUnwatched.isPending}
          />
        </>
      )}

      {/* Mobile bottom-sheet overlay - only below md breakpoint */}
      {filtersOpen && (
        <>
          {/* Backdrop */}
          <button
            type="button"
            aria-label={t("common.close")}
            className="md:hidden fixed inset-0 z-[60] bg-black/50"
            onClick={() => setFiltersOpen(false)}
          />
          {/* Bottom-sheet panel */}
          <div className="md:hidden fixed inset-x-0 bottom-0 z-[61] rounded-t-[var(--radius-lg)] bg-[var(--surface)] max-h-[80dvh] overflow-y-auto pb-[env(safe-area-inset-bottom)]">
            <LibraryFiltersPanel
              search={search}
              availableGenres={genres}
              availableOptions={availableOptions}
              onUpdate={(patch) => updateSearch(patch)}
            />
          </div>
        </>
      )}

      {/* Plugins mounting on the library page (the slot name says sidebar;
          the mount is the always-visible strip above the grid, same
          pragmatism as media.detail.sidebar's end-of-page block). */}
      <PluginSlot name="library.sidebar" context={{ libraryId }} />

      <div className={`flex gap-4 ${filtersOpen ? "items-start" : ""}`}>
        {/* Desktop inline column - only md and above */}
        {filtersOpen && (
          <div className="hidden md:block flex-shrink-0 w-52">
            <LibraryFiltersPanel
              search={search}
              availableGenres={genres}
              availableOptions={availableOptions}
              onUpdate={(patch) => updateSearch(patch)}
            />
          </div>
        )}

        <div className="flex-1 min-w-0">
          {isLoading && items.length === 0 ? (
            <MediaGridSkeleton />
          ) : viewMode === "grid" ? (
            <MediaGrid
              onEndReached={() => {
                // Stable callback (always defined). Gating inside the body
                // avoids react-virtuoso losing the prop reference mid-fetch,
                // which used to leave the user stuck on page 1.
                // Local filters are intentionally NOT gated here: scrolling
                // continues to load more server pages even when client-side
                // facets are active; the fetch-until-filled effect above
                // handles the case where the filtered list is too short to
                // reach the scroll bottom.
                if (hasMore && !activeQuery.isFetchingNextPage) {
                  void activeQuery.fetchNextPage();
                }
              }}
            >
              {displayItems.map((item) => (
                <MediaCard
                  key={item.id}
                  item={item}
                  selected={selectedIds.has(item.id ?? "")}
                  selectionMode={bulkMode}
                  onSelect={bulkMode ? handleSelect : undefined}
                />
              ))}
            </MediaGrid>
          ) : (
            <LibraryListView
              items={displayItems}
              // Same pagination the grid drives through onEndReached. The list
              // view had none at all: reaching the bottom loaded nothing, so a
              // library larger than one page simply stopped at 48 items in
              // list mode while the grid kept going. The gating lives in the
              // props (not inside the child) so both views read the same
              // hasMore/isFetchingNextPage state.
              hasNextPage={hasMore}
              isFetchingNextPage={activeQuery.isFetchingNextPage}
              onLoadMore={() => { void activeQuery.fetchNextPage(); }}
            />
          )}

          {!isLoading && activeIsError && (
            <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
              <FetchErrorState error={activeError} onRetry={activeRefetch} />
            </div>
          )}

          {!isLoading && !activeIsError && displayItems.length === 0 && (
            <div
              className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
            >
              <EmptyState
                icon={<Folder size={48} strokeWidth={1.5} />}
                title={t("media.library.noItems")}
                description={
                  hasActiveFilters
                    ? t("media.library.noItemsFiltered")
                    : isAdmin
                      ? typeFilter
                        ? t("media.library.noItemsType")
                        : t("media.library.noItemsEmpty")
                      : t("media.library.noItemsEmptyUser")
                }
                action={
                  isAdmin && !hasActiveFilters
                    ? { label: t("media.library.goToAdmin"), onClick: () => void navigate({ to: "/admin/libraries" }) }
                    : undefined
                }
              />
            </div>
          )}

          {activeQuery.isFetchingNextPage && (
            <div className="mt-8">
              <MediaGridSkeleton count={6} />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
