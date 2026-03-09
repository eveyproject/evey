import { useState, useEffect, useMemo } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
} from "@evey/api";
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
import { MediaCard } from "@/features/library/MediaCard";

export const Route = createLazyFileRoute("/_auth/library/$libraryId")({
  component: LibraryPage,
});

const MOVIE_TYPE_OPTIONS: Array<{ label: string; value: LibraryServiceListMediaItemsType | "" }> = [
  { label: "All", value: "MEDIA_ITEM_TYPE_MOVIE" },
];

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

function LibraryPage() {
  const { libraryId } = Route.useParams();
  const [typeFilter, setTypeFilter] = useState<LibraryServiceListMediaItemsType | "">("");
  const [genreFilter, setGenreFilter] = useState("");
  const [sortField, setSortField] = useState<LibraryServiceListMediaItemsSortField>(
    "MEDIA_SORT_FIELD_TITLE"
  );
  const [sortOrder, setSortOrder] = useState<LibraryServiceListMediaItemsSortOrder>("SORT_ORDER_ASC");
  const [page, setPage] = useState(1);
  const [searchTerm, setSearchTerm] = useState("");

  const { data: librariesData } = useLibraryServiceListLibraries();
  const library = librariesData?.libraries?.find((l) => l.id === libraryId);
  const isShowLibrary = library?.type === "LIBRARY_TYPE_SHOWS";

  useEffect(() => {
    if (!library) return;
    if (library.type === "LIBRARY_TYPE_SHOWS") {
      setTypeFilter("MEDIA_ITEM_TYPE_SERIES");
    } else if (library.type === "LIBRARY_TYPE_MOVIES") {
      setTypeFilter("MEDIA_ITEM_TYPE_MOVIE");
    }
  }, [library?.type]);

  const resetPage = () => setPage(1);

  const typeOptions = isShowLibrary ? SHOW_TYPE_OPTIONS : MOVIE_TYPE_OPTIONS;

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

  const { data: genreData } = useLibraryServiceListMediaItems({
    libraryId,
    ...(typeFilter ? { type: typeFilter } : {}),
    "pagination.page": 1,
    "pagination.pageSize": PAGE_SIZE * 2,
  });

  const genres = useMemo(() => {
    const allGenres = new Set<string>();
    for (const item of genreData?.items ?? []) {
      for (const g of item.genres ?? []) {
        allGenres.add(g);
      }
    }
    return Array.from(allGenres).sort();
  }, [genreData?.items]);

  const items = data?.items ?? [];
  const totalItems = data?.pagination?.totalItems ?? 0;
  const hasMore = items.length < totalItems;

  const filteredItems = useMemo(() => {
    if (!searchTerm.trim()) return items;
    const lower = searchTerm.toLowerCase();
    return items.filter((item) => item.title?.toLowerCase().includes(lower));
  }, [items, searchTerm]);

  const displayItems = searchTerm.trim() ? filteredItems : items;
  const displayCount = searchTerm.trim() ? filteredItems.length : totalItems;

  return (
    <div data-testid="library-page">
      <div className="flex items-center justify-between gap-4 mb-2 flex-wrap">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }} data-testid="library-name">
          {library?.name ?? "Library"}
        </h1>
        <span className="text-sm" style={{ color: "var(--text-muted)" }} data-testid="library-item-count">
          {displayCount} {displayCount === 1 ? "item" : "items"}
          {searchTerm.trim() && ` matching "${searchTerm.trim()}"`}
        </span>
      </div>

      <div className="flex items-end gap-3 mb-6 flex-wrap">
        <div className="w-48">
          <Input
            type="search"
            placeholder="Filter by name..."
            value={searchTerm}
            onChange={(e) => setSearchTerm(e.target.value)}
            data-testid="library-search-input"
          />
        </div>
        {typeOptions.length > 1 && (
          <Select
            value={typeFilter}
            onChange={(v) => {
              setTypeFilter(v as LibraryServiceListMediaItemsType | "");
              setGenreFilter("");
              resetPage();
            }}
            options={typeOptions.map((o) => ({ value: o.value, label: o.label }))}
            className="w-auto"
          />
        )}
        {genres.length > 0 && (
          <Select
            value={genreFilter}
            onChange={(v) => {
              setGenreFilter(v);
              resetPage();
            }}
            options={[
              { value: "", label: "All genres" },
              ...genres.map((g) => ({ value: g, label: g })),
            ]}
            className="w-auto"
          />
        )}
        <Select
          value={sortField}
          onChange={(v) => {
            setSortField(v as LibraryServiceListMediaItemsSortField);
            resetPage();
          }}
          options={SORT_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
          className="w-auto"
        />
        <SortOrderButton order={sortOrder} onChange={(o) => { setSortOrder(o); resetPage(); }} />
      </div>

      {isLoading && items.length === 0 ? (
        <MediaGridSkeleton />
      ) : (
        <MediaGrid>
          {displayItems.map((item) => (
            <MediaCard key={item.id} item={item} />
          ))}
        </MediaGrid>
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
              genreFilter || searchTerm.trim()
                ? "Try adjusting your filters or search term."
                : typeFilter
                  ? "Try a different filter, or scan the library to add content."
                  : "This library is empty. Trigger a scan to add content."
            }
            action={!searchTerm.trim() && !genreFilter ? { label: "Go to Admin", onClick: () => { window.location.href = "/admin/libraries"; } } : undefined}
          />
        </div>
      )}

      {hasMore && !isLoading && !searchTerm.trim() && (
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
  );
}
