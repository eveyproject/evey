import { useState, useEffect, useMemo, useCallback } from "react";
import { Folder, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { createLazyFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  libraryServiceListMediaItems,
  libraryServiceFilterMediaItems,
  libraryServiceSearchMedia,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
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
import { favoriteServiceAddFavorite } from "@evey/api";
import { BulkActionBar } from "@/components/BulkActionBar";
import { AddToCollectionModal } from "@/features/collection/AddToCollectionModal";
import { SortOrderButton, ViewToggleButton } from "./LibraryToolbar";
import { LibraryListView } from "./LibraryListView";
import { BulkPlaylistModal } from "./BulkPlaylistModal";

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

const DEDICATED_ROUTES: Partial<Record<string, string>> = {
  LIBRARY_TYPE_PHOTOS: "/photos",
  LIBRARY_TYPE_MUSIC: "/music",
};

const DEFAULT_TYPE_FILTER: Partial<Record<string, LibraryServiceListMediaItemsType>> = {
  LIBRARY_TYPE_SHOWS: "MEDIA_ITEM_TYPE_SERIES",
  LIBRARY_TYPE_MOVIES: "MEDIA_ITEM_TYPE_MOVIE",
};

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
    await Promise.all(ids.map((mediaItemId) => fn({ mediaItemId, params: { profileId } })));
    ids.forEach((id) => queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${id}`] }));
    toast.success(watched ? t("media.favorites.markedWatched", { count: ids.length }) : t("media.favorites.markedUnwatched", { count: ids.length }));
    setSelectedIds(new Set());
    setBulkMode(false);
  }

  async function bulkAddFavorites() {
    if (selectedIds.size === 0) return;
    setBulkFavPending(true);
    const ids = Array.from(selectedIds);
    try {
      await Promise.all(ids.map((id) => favoriteServiceAddFavorite(id, {})));
      queryClient.invalidateQueries({ queryKey: ["favorites"] });
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
    const dedicated = DEDICATED_ROUTES[library.type ?? ""];
    if (dedicated) { void navigate({ to: dedicated, replace: true }); return; }
    if (typeFilter !== undefined) return;
    const type = DEFAULT_TYPE_FILTER[library.type ?? ""];
    if (type) void navigate({ to: "/library/$libraryId", params: { libraryId: slug }, search: { ...search, type }, replace: true });
  // eslint-disable-next-line react-hooks/exhaustive-deps -- search/slug excluded: search is a new object each render, slug is stable but derived from params already covered
  }, [library?.type, typeFilter, navigate]);

  const typeOptions = isShowLibrary ? SHOW_TYPE_KEYS : [];

  const hasServerFilters = Boolean(
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
      // Server falls back to offset when cursor isn't available — stop when
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
        search.yearFrom ?? null,
        search.yearTo ?? null,
        search.minRating ?? null,
        sortField,
        sortOrder,
      ] as const,
    [libraryId, typeFilter, genreFilter, search.yearFrom, search.yearTo, search.minRating, sortField, sortOrder],
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
  // paginated into memory — typing "Matrix" in a 5k-item library returned
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
          limit: 120,
        },
        signal,
      ),
  });

  const activeQuery = hasServerFilters ? filterQuery : regularQuery;
  const isLoading = searchActive ? searchQuery.isLoading : activeQuery.isLoading;

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

  const genres = useMemo(() => {
    const allGenres = new Set<string>();
    for (const item of metaData?.items ?? []) {
      for (const g of item.genres ?? []) allGenres.add(g);
    }
    return Array.from(allGenres).sort();
  }, [metaData?.items]);

  const availableOptions = useMemo(
    () => collectFilterOptions(metaData?.items ?? []),
    [metaData?.items],
  );

  const localFiltered = useMemo(() => {
    // Title-based search runs server-side via searchQuery now — no client
    // re-filter needed here, and it would actually narrow results since
    // searchQuery's top-N may include items whose title contains the term
    // via the search index's looser matching (synonyms, accent folding).
    return applyLocalFilters(items, {
      genres: search.genres,
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
    search.genres?.length ||
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

  return (
    <div data-testid="library-page">
      <nav className="flex items-center gap-1.5 text-sm mb-3" aria-label="Breadcrumb">
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

      <div className={`flex gap-4 ${filtersOpen ? "items-start" : ""}`}>
        {filtersOpen && (
          <div className="flex-shrink-0 w-52">
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
                if (hasMore && !hasLocalFilters && !activeQuery.isFetchingNextPage) {
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
            <LibraryListView items={displayItems} />
          )}

          {!isLoading && displayItems.length === 0 && (
            <div
              className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
            >
              <EmptyState
                icon={<Folder size={48} strokeWidth={1.5} />}
                title={t("media.library.noItems")}
                description={
                  hasActiveFilters
                    ? t("media.library.noItemsFiltered")
                    : typeFilter
                      ? t("media.library.noItemsType")
                      : t("media.library.noItemsEmpty")
                }
                action={
                  !hasActiveFilters
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
