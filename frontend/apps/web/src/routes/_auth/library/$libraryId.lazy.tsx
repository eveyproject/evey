import { useState, useEffect, useMemo, useCallback } from "react";
import { ArrowUpNarrowWide, ArrowDownWideNarrow, Folder, List, LayoutGrid, Star, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { createLazyFileRoute, Link, useNavigate } from "@tanstack/react-router";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
  usePlaybackServiceMarkAsWatched,
  usePlaybackServiceMarkAsUnwatched,
} from "@evey/api";
import { usePlaylistList, useAddToPlaylist } from "@/features/playlist/api";
import { Modal } from "@/components/ui/Modal";
import type {
  LibraryServiceListMediaItemsType,
  LibraryServiceListMediaItemsSortField,
  LibraryServiceListMediaItemsSortOrder,
  V1MediaItem,
} from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Select";
import { Input } from "@/components/ui/Input";
import { EmptyState } from "@/components/ui/EmptyState";
import { MediaGridSkeleton } from "@/components/ui/Skeleton";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard, typeLabel } from "@/features/library/MediaCard";
import {
  LibraryFilterToggle,
  LibraryFiltersPanel,
  collectFilterOptions,
  applyLocalFilters,
} from "@/features/library/LibraryFilters";
import type { LibrarySearch } from "@/routes/_auth/library/$libraryId";
import { slugify } from "@/utils/slugify";
import { toast, useAuthStore } from "@evey/store";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { BulkActionBar } from "@/components/BulkActionBar";
import { AddToCollectionModal } from "@/features/collection/AddToCollectionModal";

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

type FilterApiItem = {
  id: string;
  title: string;
  type: string;
  year?: number;
  communityRating?: number;
  libraryId: string;
};

type FilterApiResponse = {
  items: FilterApiItem[];
  total: number;
};

function toFilterSortField(f: LibraryServiceListMediaItemsSortField): string {
  switch (f) {
    case "MEDIA_SORT_FIELD_YEAR": return "year";
    case "MEDIA_SORT_FIELD_RATING": return "community_rating";
    case "MEDIA_SORT_FIELD_DATE_ADDED": return "added_at";
    default: return "sort_title";
  }
}

function toFilterSortOrder(o: LibraryServiceListMediaItemsSortOrder): string {
  return o === "SORT_ORDER_DESC" ? "desc" : "asc";
}

function filterItemToMediaItem(item: FilterApiItem): V1MediaItem {
  return {
    id: item.id,
    title: item.title,
    type: item.type as V1MediaItem["type"],
    year: item.year,
    communityRating: item.communityRating,
    libraryId: item.libraryId,
  };
}

function formatDuration(ms: string): string {
  const totalMin = Math.round(parseInt(ms, 10) / 60000);
  if (totalMin < 60) return `${totalMin}m`;
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return m > 0 ? `${h}h ${m}m` : `${h}h`;
}

function SortOrderButton({
  order,
  onChange,
}: {
  order: LibraryServiceListMediaItemsSortOrder;
  onChange: (o: LibraryServiceListMediaItemsSortOrder) => void;
}) {
  const isAsc = order === "SORT_ORDER_ASC";
  return (
    <button
      type="button"
      onClick={() => onChange(isAsc ? "SORT_ORDER_DESC" : "SORT_ORDER_ASC")}
      className="p-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--accent)] transition-colors-fast"
      title={isAsc ? "Ascending" : "Descending"}
    >
      {isAsc ? <ArrowUpNarrowWide size={16} /> : <ArrowDownWideNarrow size={16} />}
    </button>
  );
}

function ViewToggleButton({
  mode,
  onChange,
}: {
  mode: "grid" | "list";
  onChange: (m: "grid" | "list") => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onChange(mode === "grid" ? "list" : "grid")}
      className="p-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--accent)] transition-colors-fast"
      title={mode === "grid" ? "Switch to list view" : "Switch to grid view"}
      data-testid="view-toggle"
    >
      {mode === "grid" ? <List size={16} /> : <LayoutGrid size={16} />}
    </button>
  );
}

function ListRow({ item }: { item: Record<string, unknown> }) {
  const title = (item.title as string) ?? "";
  const year = item.year as number | undefined;
  const rating = item.communityRating as number | undefined;
  const type = item.type as string | undefined;
  const id = item.id as string | undefined;
  const durationMs = item.durationMs as string | undefined;
  const poster = id ? `/api/v1/media/${id}/artwork/poster` : null;

  return (
    <Link
      to="/media/$mediaId"
      params={{ mediaId: id ?? "" }}
      className="flex items-center gap-4 px-3 py-2 rounded-[var(--radius-md)] transition-colors-fast hover:bg-[var(--surface-raised)] border-b border-[var(--border)]"
      data-testid="list-row"
    >
      <div
        className="flex-shrink-0 w-7 h-10 rounded overflow-hidden bg-[var(--surface-raised)]"
      >
        {poster && (
          <img src={poster} alt={title} className="w-full h-full object-cover" loading="lazy" />
        )}
      </div>
      <span className="flex-1 min-w-0 text-sm font-medium truncate text-[var(--text)]">
        {title}
      </span>
      <span className="hidden sm:block w-16 text-xs text-right flex-shrink-0 text-[var(--text-muted)]">
        {year ?? "—"}
      </span>
      <span className="hidden md:flex items-center gap-1 w-16 text-xs flex-shrink-0 justify-end text-[var(--text-muted)]">
        {rating != null && rating > 0 ? (
          <>
            <Star size={12} fill="currentColor" className="text-[var(--accent)]" />
            {rating.toFixed(1)}
          </>
        ) : "—"}
      </span>
      {typeLabel(type) && (
        <span
          className="hidden lg:block text-xs px-1.5 py-0.5 flex-shrink-0 bg-[var(--surface-raised)] text-[var(--text-muted)] rounded-[var(--radius-sm)]"
        >
          {typeLabel(type)}
        </span>
      )}
      <span className="hidden lg:block w-16 text-xs text-right flex-shrink-0 text-[var(--text-muted)]">
        {durationMs ? formatDuration(durationMs) : "—"}
      </span>
    </Link>
  );
}

function BulkPlaylistModal({
  open,
  mediaIds,
  onClose,
}: {
  open: boolean;
  mediaIds: string[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { data: playlists, isLoading } = usePlaylistList();
  const addTo = useAddToPlaylist();
  const manualPlaylists = (playlists ?? []).filter((p) => p.type === "manual");

  async function handleAdd(playlistId: string, playlistName: string) {
    try {
      await Promise.all(mediaIds.map((id) => addTo.mutateAsync({ playlistId, mediaItemId: id })));
      toast.success(t("media.library.itemsAddedToPlaylist", { count: mediaIds.length, name: playlistName }));
      onClose();
    } catch {
      toast.error(t("media.library.failedAddPlaylist"));
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("media.library.addToPlaylist")}>
      {isLoading ? (
        <div className="flex justify-center py-8">
          <div className="w-6 h-6 border-2 rounded-full animate-spin border-[var(--border)] border-t-[var(--accent)]" />
        </div>
      ) : manualPlaylists.length === 0 ? (
        <p className="text-sm text-center py-6 text-[var(--text-muted)]">{t("media.library.noPlaylists")}</p>
      ) : (
        <div className="space-y-1">
          {manualPlaylists.map((pl) => (
            <button
              key={pl.id}
              onClick={() => void handleAdd(pl.id, pl.name)}
              disabled={addTo.isPending}
              className="w-full flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] text-left transition-colors-fast hover:bg-[var(--surface-raised)] text-[var(--text)]"
            >
              <span className="text-sm font-medium truncate">{pl.name}</span>
            </button>
          ))}
        </div>
      )}
    </Modal>
  );
}

function LibraryPage() {
  const { t } = useTranslation();
    const { libraryId: slug } = Route.useParams();
  const search = Route.useSearch();
  const navigate = useNavigate();

  const [page, setPage] = useState(1);
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
      setPage(1);
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
      await Promise.all(ids.map((id) => eveyFetch<unknown>({ url: `/api/v1/favorites/${id}`, method: "POST" })));
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

  // Set default type filter based on library type (only on first load when no URL param)
  useEffect(() => {
    if (!library || typeFilter !== undefined) return;
    if (library.type === "LIBRARY_TYPE_SHOWS") {
      void navigate({ to: "/library/$libraryId", params: { libraryId: slug }, search: { ...search, type: "MEDIA_ITEM_TYPE_SERIES" as LibraryServiceListMediaItemsType }, replace: true });
    } else if (library.type === "LIBRARY_TYPE_MOVIES") {
      void navigate({ to: "/library/$libraryId", params: { libraryId: slug }, search: { ...search, type: "MEDIA_ITEM_TYPE_MOVIE" as LibraryServiceListMediaItemsType }, replace: true });
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps -- search/slug excluded: search is a new object each render, slug is stable but derived from params already covered
  }, [library?.type, typeFilter, navigate]);

  const typeOptions = isShowLibrary ? SHOW_TYPE_KEYS : [];

  const hasServerFilters = Boolean(
    search.yearFrom != null ||
    search.yearTo != null ||
    (search.minRating != null && search.minRating > 0),
  );

  const params = {
    libraryId,
    ...(typeFilter ? { type: typeFilter } : {}),
    ...(genreFilter ? { genre: genreFilter } : {}),
    sortField,
    sortOrder,
    "pagination.page": 1,
    "pagination.pageSize": page * PAGE_SIZE,
  };

  const { data: regularData, isLoading: regularLoading } = useLibraryServiceListMediaItems(params, {
    query: { enabled: !hasServerFilters && !!libraryId },
  });

  const filterParams = useMemo(() => {
    if (!hasServerFilters || !libraryId) return null;
    const p = new URLSearchParams({ library_id: libraryId });
    if (search.yearFrom != null) p.set("year_from", String(search.yearFrom));
    if (search.yearTo != null) p.set("year_to", String(search.yearTo));
    if (search.minRating != null && search.minRating > 0) p.set("min_rating", String(search.minRating));
    if (typeFilter) p.set("type", typeFilter);
    if (genreFilter) p.set("genre", genreFilter);
    p.set("sort_field", toFilterSortField(sortField));
    p.set("sort_order", toFilterSortOrder(sortOrder));
    p.set("limit", String(page * PAGE_SIZE));
    p.set("offset", "0");
    return p;
  }, [hasServerFilters, libraryId, search.yearFrom, search.yearTo, search.minRating, typeFilter, genreFilter, sortField, sortOrder, page]);

  const { data: filterData, isLoading: filterLoading } = useQuery<FilterApiResponse>({
    queryKey: ["media-filter", libraryId, search.yearFrom, search.yearTo, search.minRating, typeFilter, genreFilter, sortField, sortOrder, page],
    queryFn: () => eveyFetch<FilterApiResponse>({ url: `/api/v1/media/filter?${filterParams!.toString()}`, method: "GET" }),
    enabled: hasServerFilters && !!libraryId && filterParams !== null,
  });

  const isLoading = hasServerFilters ? filterLoading : regularLoading;

  const items: V1MediaItem[] = hasServerFilters
    ? (filterData?.items ?? []).map(filterItemToMediaItem)
    : (regularData?.items ?? []);
  const totalItems = hasServerFilters
    ? (filterData?.total ?? 0)
    : (regularData?.pagination?.totalItems ?? 0);
  const hasMore = items.length < totalItems && !hasServerFilters;

  // Fetch a broader set for deriving genres + filter options (without pagination limit)
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

  // Client-side: text search + stream-level filters (year/rating now handled server-side when active)
  const localFiltered = useMemo(() => {
    let result = items;
    if (searchTerm.trim()) {
      const lower = searchTerm.toLowerCase();
      result = result.filter((item) => item.title?.toLowerCase().includes(lower));
    }
    result = applyLocalFilters(result, {
      genres: search.genres,
      resolutions: search.resolutions,
      codecs: search.codecs,
      audioLangs: search.audioLangs,
      subLangs: search.subLangs,
      yearFrom: hasServerFilters ? undefined : search.yearFrom,
      yearTo: hasServerFilters ? undefined : search.yearTo,
      minRating: hasServerFilters ? undefined : search.minRating,
    });
    return result;
  }, [items, searchTerm, search.genres, search.resolutions, search.codecs, search.audioLangs, search.subLangs, search.yearFrom, search.yearTo, search.minRating, hasServerFilters]);

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
          {displayCount} {displayCount === 1 ? "item" : "items"}
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

      {/* Bulk action bar (fixed bottom) */}
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
            <MediaGrid>
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
            <div
              className="rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface)] border border-[var(--border)]"
            >
              <div
                className="flex items-center gap-4 px-3 py-2 text-xs font-semibold text-[var(--text-muted)] border-b border-[var(--border)]"
              >
                <span className="w-7 flex-shrink-0" />
                <span className="flex-1 min-w-0">{t("media.library.titleSort")}</span>
                <span className="hidden sm:block w-16 text-right flex-shrink-0">{t("media.library.year")}</span>
                <span className="hidden md:block w-16 text-right flex-shrink-0">{t("media.library.ratingSort")}</span>
                <span className="hidden lg:block w-12 flex-shrink-0">{t("media.library.type")}</span>
                <span className="hidden lg:block w-16 text-right flex-shrink-0">{t("media.library.duration")}</span>
              </div>
              {displayItems.map((item) => (
                <ListRow key={item.id} item={item as unknown as Record<string, unknown>} />
              ))}
            </div>
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
                    ? { label: t("media.library.goToAdmin"), onClick: () => { window.location.href = "/admin/libraries"; } }
                    : undefined
                }
              />
            </div>
          )}

          {hasMore && !isLoading && !hasActiveFilters && (
            <div className="flex justify-center mt-8">
              <Button variant="secondary" onClick={() => setPage((p) => p + 1)}>
                {t("media.library.loadMore", { current: items.length, total: totalItems })}
              </Button>
            </div>
          )}

          {isLoading && items.length > 0 && (
            <div className="mt-8">
              <MediaGridSkeleton count={6} />
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
