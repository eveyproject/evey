import { useState, useEffect, useMemo, useCallback } from "react";
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
} from "@evey/api/src/generated/library.schemas";
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
import { useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export const Route = createLazyFileRoute("/_auth/library/$libraryId")({
  component: LibraryPage,
});

const SHOW_TYPE_OPTIONS: Array<{ label: string; value: LibraryServiceListMediaItemsType | "" }> = [
  { label: "Series", value: "MEDIA_ITEM_TYPE_SERIES" },
  { label: "Seasons", value: "MEDIA_ITEM_TYPE_SEASON" },
  { label: "Episodes", value: "MEDIA_ITEM_TYPE_EPISODE" },
];

const SORT_OPTIONS: Array<{ label: string; value: LibraryServiceListMediaItemsSortField }> = [
  { label: "Title", value: "MEDIA_SORT_FIELD_TITLE" },
  { label: "Year", value: "MEDIA_SORT_FIELD_YEAR" },
  { label: "Rating", value: "MEDIA_SORT_FIELD_RATING" },
  { label: "Date Added", value: "MEDIA_SORT_FIELD_DATE_ADDED" },
];

const PAGE_SIZE = 48;

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
      className="p-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--accent)] transition-colors"
      title={isAsc ? "Ascending" : "Descending"}
    >
      <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
        {isAsc ? (
          <path strokeLinecap="round" strokeLinejoin="round" d="M3 4.5h14.25M3 9h9.75M3 13.5h5.25m5.25-.75L17.25 9m0 0L21 12.75M17.25 9v12" />
        ) : (
          <path strokeLinecap="round" strokeLinejoin="round" d="M3 4.5h14.25M3 9h9.75M3 13.5h9.75m4.5-4.5v12m0 0l-3.75-3.75M17.25 21L21 17.25" />
        )}
      </svg>
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
      className="p-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--accent)] transition-colors"
      title={mode === "grid" ? "Switch to list view" : "Switch to grid view"}
      data-testid="view-toggle"
    >
      {mode === "grid" ? (
        <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 5.25h16.5M3.75 9.75h16.5M3.75 14.25h16.5M3.75 18.75h16.5" />
        </svg>
      ) : (
        <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 6A2.25 2.25 0 016 3.75h2.25A2.25 2.25 0 0110.5 6v2.25a2.25 2.25 0 01-2.25 2.25H6a2.25 2.25 0 01-2.25-2.25V6zM3.75 15.75A2.25 2.25 0 016 13.5h2.25a2.25 2.25 0 012.25 2.25V18a2.25 2.25 0 01-2.25 2.25H6A2.25 2.25 0 013.75 18v-2.25zM13.5 6a2.25 2.25 0 012.25-2.25H18A2.25 2.25 0 0120.25 6v2.25A2.25 2.25 0 0118 10.5h-2.25a2.25 2.25 0 01-2.25-2.25V6zM13.5 15.75a2.25 2.25 0 012.25-2.25H18a2.25 2.25 0 012.25 2.25V18A2.25 2.25 0 0118 20.25h-2.25A2.25 2.25 0 0113.5 18v-2.25z" />
        </svg>
      )}
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
      className="flex items-center gap-4 px-3 py-2 rounded-[var(--radius-md)] transition-colors hover:bg-[var(--surface-raised)]"
      style={{ borderBottom: "1px solid var(--border)" }}
      data-testid="list-row"
    >
      <div
        className="flex-shrink-0 w-7 h-10 rounded overflow-hidden"
        style={{ backgroundColor: "var(--surface-raised)" }}
      >
        {poster && (
          <img src={poster} alt={title} className="w-full h-full object-cover" loading="lazy" />
        )}
      </div>
      <span className="flex-1 min-w-0 text-sm font-medium truncate" style={{ color: "var(--text)" }}>
        {title}
      </span>
      <span className="hidden sm:block w-16 text-xs text-right flex-shrink-0" style={{ color: "var(--text-muted)" }}>
        {year ?? "—"}
      </span>
      <span className="hidden md:flex items-center gap-1 w-16 text-xs flex-shrink-0 justify-end" style={{ color: "var(--text-muted)" }}>
        {rating != null && rating > 0 ? (
          <>
            <svg className="w-3 h-3" viewBox="0 0 20 20" fill="currentColor" style={{ color: "var(--accent)" }}>
              <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
            </svg>
            {rating.toFixed(1)}
          </>
        ) : "—"}
      </span>
      {typeLabel(type) && (
        <span
          className="hidden lg:block text-xs px-1.5 py-0.5 flex-shrink-0"
          style={{ backgroundColor: "var(--surface-raised)", color: "var(--text-muted)", borderRadius: "var(--radius-sm)" }}
        >
          {typeLabel(type)}
        </span>
      )}
      <span className="hidden lg:block w-16 text-xs text-right flex-shrink-0" style={{ color: "var(--text-muted)" }}>
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
  const { data: playlists, isLoading } = usePlaylistList();
  const addTo = useAddToPlaylist();
  const manualPlaylists = (playlists ?? []).filter((p) => p.type === "manual");

  async function handleAdd(playlistId: string, playlistName: string) {
    try {
      await Promise.all(mediaIds.map((id) => addTo.mutateAsync({ playlistId, mediaItemId: id })));
      toast.success(`${mediaIds.length} item${mediaIds.length > 1 ? "s" : ""} added to "${playlistName}"`);
      onClose();
    } catch {
      toast.error("Failed to add to playlist");
    }
  }

  return (
    <Modal open={open} onClose={onClose} title="Add to Playlist">
      {isLoading ? (
        <div className="flex justify-center py-8">
          <div className="w-6 h-6 border-2 rounded-full animate-spin" style={{ borderColor: "var(--border)", borderTopColor: "var(--accent)" }} />
        </div>
      ) : manualPlaylists.length === 0 ? (
        <p className="text-sm text-center py-6" style={{ color: "var(--text-muted)" }}>No playlists yet.</p>
      ) : (
        <div className="space-y-1">
          {manualPlaylists.map((pl) => (
            <button
              key={pl.id}
              onClick={() => void handleAdd(pl.id, pl.name)}
              disabled={addTo.isPending}
              className="w-full flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] text-left transition-colors hover:bg-[var(--surface-raised)]"
              style={{ color: "var(--text)" }}
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
  const { libraryId: slug } = Route.useParams();
  const search = Route.useSearch();
  const navigate = useNavigate();

  const [page, setPage] = useState(1);
  const [viewMode, setViewMode] = useState<"grid" | "list">("grid");
  const [bulkMode, setBulkMode] = useState(false);
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const [filtersOpen, setFiltersOpen] = useState(false);
  const [bulkPlaylistOpen, setBulkPlaylistOpen] = useState(false);
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
    toast.success(`${ids.length} item${ids.length > 1 ? "s" : ""} marked as ${watched ? "watched" : "unwatched"}`);
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
      toast.success(`${ids.length} item${ids.length > 1 ? "s" : ""} added to favorites`);
      setSelectedIds(new Set());
      setBulkMode(false);
    } catch {
      toast.error("Failed to add some items to favorites");
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
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [library?.type, typeFilter, navigate]);

  const typeOptions = isShowLibrary ? SHOW_TYPE_OPTIONS : [];

  const params = {
    libraryId,
    ...(typeFilter ? { type: typeFilter } : {}),
    ...(genreFilter ? { genre: genreFilter } : {}),
    sortField,
    sortOrder,
    "pagination.page": 1,
    "pagination.pageSize": page * PAGE_SIZE,
  };

  const { data, isLoading } = useLibraryServiceListMediaItems(params);

  // Fetch a broader set for deriving genres + filter options (without pagination limit)
  const { data: metaData } = useLibraryServiceListMediaItems({
    libraryId,
    ...(typeFilter ? { type: typeFilter } : {}),
    "pagination.page": 1,
    "pagination.pageSize": PAGE_SIZE * 4,
  });

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

  const items = data?.items ?? [];
  const totalItems = data?.pagination?.totalItems ?? 0;
  const hasMore = items.length < totalItems;

  // Client-side: text search + stream-level filters
  const localFiltered = useMemo(() => {
    let result = items;
    if (searchTerm.trim()) {
      const lower = searchTerm.toLowerCase();
      result = result.filter((item) => item.title?.toLowerCase().includes(lower));
    }
    result = applyLocalFilters(result, {
      resolutions: search.resolutions,
      codecs: search.codecs,
      audioLangs: search.audioLangs,
      subLangs: search.subLangs,
    });
    return result;
  }, [items, searchTerm, search.resolutions, search.codecs, search.audioLangs, search.subLangs]);

  const hasLocalFilters = Boolean(
    searchTerm.trim() ||
    search.resolutions?.length ||
    search.codecs?.length ||
    search.audioLangs?.length ||
    search.subLangs?.length,
  );
  const displayItems = hasLocalFilters ? localFiltered : items;
  const displayCount = hasLocalFilters ? localFiltered.length : totalItems;

  return (
    <div data-testid="library-page">
      <nav className="flex items-center gap-1.5 text-sm mb-3" aria-label="Breadcrumb">
        <Link to="/library" className="hover:underline" style={{ color: "var(--text-muted)" }}>
          Libraries
        </Link>
        <svg className="w-3 h-3 flex-shrink-0" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24" style={{ color: "var(--text-muted)" }}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
        </svg>
        <span className="truncate font-medium" style={{ color: "var(--text)" }}>
          {library?.name ?? "Library"}
        </span>
      </nav>
      <div className="flex items-center justify-between gap-4 mb-2 flex-wrap">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }} data-testid="library-name">
          {library?.name ?? "Library"}
        </h1>
        <span className="text-sm" style={{ color: "var(--text-muted)" }} data-testid="library-item-count">
          {displayCount} {displayCount === 1 ? "item" : "items"}
          {hasLocalFilters && " (filtered)"}
        </span>
      </div>

      <div className="flex items-end gap-3 mb-6 flex-wrap">
        <div className="w-48">
          <Input
            type="search"
            placeholder="Filter by name..."
            value={searchTerm}
            onChange={(e) => updateSearch({ q: e.target.value || undefined })}
            data-testid="library-search-input"
          />
        </div>
        {typeOptions.length > 0 && (
          <Select
            value={typeFilter ?? ""}
            onChange={(v) => updateSearch({ type: (v as LibraryServiceListMediaItemsType) || undefined })}
            options={typeOptions.map((o) => ({ value: o.value, label: o.label }))}
            className="w-auto"
          />
        )}
        <Select
          value={sortField}
          onChange={(v) => updateSearch({ sortField: v as LibraryServiceListMediaItemsSortField })}
          options={SORT_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
          className="w-auto"
        />
        <SortOrderButton
          order={sortOrder}
          onChange={(o) => updateSearch({ sortOrder: o })}
        />
        <LibraryFilterToggle search={search} onClick={() => setFiltersOpen((v) => !v)} />
        <ViewToggleButton mode={viewMode} onChange={setViewMode} />
        <button
          type="button"
          onClick={() => { setBulkMode((v) => !v); setSelectedIds(new Set()); }}
          className="p-2 rounded-[var(--radius-md)] border transition-colors text-sm"
          style={{
            backgroundColor: bulkMode ? "var(--accent)" : "var(--surface-raised)",
            color: bulkMode ? "#fff" : "var(--text-muted)",
            borderColor: bulkMode ? "var(--accent)" : "var(--border)",
          }}
          title={bulkMode ? "Exit selection" : "Select items"}
        >
          Select
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

      {/* Bulk action bar */}
      {bulkMode && (
        <div
          className="sticky top-0 z-10 flex items-center gap-2 px-4 py-2 mb-4 rounded-[var(--radius-md)] flex-wrap"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--accent)" }}
        >
          <span
            className="text-sm font-semibold px-2 py-0.5 rounded-full mr-1"
            style={{ backgroundColor: "var(--accent)", color: "#fff" }}
          >
            {selectedIds.size}
          </span>
          <span className="text-sm mr-2" style={{ color: "var(--text-muted)" }}>selected</span>
          {selectedIds.size > 0 && (
            <>
              <Button
                variant="secondary"
                onClick={() => bulkMarkWatched(true)}
                loading={markWatched.isPending}
              >
                Mark watched
              </Button>
              <Button
                variant="secondary"
                onClick={() => bulkMarkWatched(false)}
                loading={markUnwatched.isPending}
              >
                Mark unwatched
              </Button>
              <Button
                variant="secondary"
                loading={bulkFavPending}
                onClick={() => void bulkAddFavorites()}
              >
                Add to favorites
              </Button>
              <Button
                variant="secondary"
                onClick={() => setBulkPlaylistOpen(true)}
              >
                Add to playlist
              </Button>
            </>
          )}
          <div className="flex items-center gap-2 ml-auto">
            <button
              type="button"
              onClick={() => setSelectedIds(new Set(displayItems.map((i) => i.id ?? "").filter(Boolean)))}
              className="text-xs hover:underline"
              style={{ color: "var(--accent)" }}
            >
              Select all
            </button>
            <button
              type="button"
              onClick={() => setSelectedIds(new Set())}
              className="text-xs hover:underline"
              style={{ color: "var(--text-muted)" }}
            >
              Deselect all
            </button>
            <button
              type="button"
              onClick={() => { setBulkMode(false); setSelectedIds(new Set()); }}
              className="text-xs hover:underline"
              style={{ color: "var(--text-muted)" }}
            >
              Exit
            </button>
          </div>
        </div>
      )}

      <div className={`flex gap-4 ${filtersOpen ? "items-start" : ""}`}>
        {filtersOpen && (
          <div className="flex-shrink-0 w-52">
            <LibraryFiltersPanel
              search={search}
              genres={genres}
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
                  onSelect={bulkMode ? handleSelect : undefined}
                />
              ))}
            </MediaGrid>
          ) : (
            <div
              className="rounded-[var(--radius-md)] overflow-hidden"
              style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
            >
              <div
                className="flex items-center gap-4 px-3 py-2 text-xs font-semibold"
                style={{ color: "var(--text-muted)", borderBottom: "1px solid var(--border)" }}
              >
                <span className="w-7 flex-shrink-0" />
                <span className="flex-1 min-w-0">Title</span>
                <span className="hidden sm:block w-16 text-right flex-shrink-0">Year</span>
                <span className="hidden md:block w-16 text-right flex-shrink-0">Rating</span>
                <span className="hidden lg:block w-12 flex-shrink-0">Type</span>
                <span className="hidden lg:block w-16 text-right flex-shrink-0">Duration</span>
              </div>
              {displayItems.map((item) => (
                <ListRow key={item.id} item={item as unknown as Record<string, unknown>} />
              ))}
            </div>
          )}

          {!isLoading && displayItems.length === 0 && (
            <div
              className="rounded-[var(--radius-lg)]"
              style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
            >
              <EmptyState
                icon="library"
                title="No items found"
                description={
                  hasLocalFilters
                    ? "Try adjusting your filters or search term."
                    : typeFilter
                      ? "Try a different filter, or scan the library to add content."
                      : "This library is empty. Trigger a scan to add content."
                }
                action={
                  !hasLocalFilters
                    ? { label: "Go to Admin", onClick: () => { window.location.href = "/admin/libraries"; } }
                    : undefined
                }
              />
            </div>
          )}

          {hasMore && !isLoading && !hasLocalFilters && (
            <div className="flex justify-center mt-8">
              <Button variant="secondary" onClick={() => setPage((p) => p + 1)}>
                Load more ({items.length} of {totalItems})
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
