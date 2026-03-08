import { useState, useEffect } from "react";
import { createFileRoute } from "@tanstack/react-router";
import {
  useLibraryServiceListLibraries,
  useLibraryServiceListMediaItems,
} from "@evey/api";
import type {
  LibraryServiceListMediaItemsType,
  LibraryServiceListMediaItemsSortField,
} from "@evey/api/src/generated/library.schemas";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";

export const Route = createFileRoute("/_auth/library/$libraryId")({
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

function LibraryPage() {
  const { libraryId } = Route.useParams();
  const [typeFilter, setTypeFilter] = useState<LibraryServiceListMediaItemsType | "">("");
  const [sortField, setSortField] = useState<LibraryServiceListMediaItemsSortField>(
    "MEDIA_SORT_FIELD_TITLE"
  );
  const [page, setPage] = useState(1);

  const { data: librariesData } = useLibraryServiceListLibraries();
  const library = librariesData?.libraries?.find((l) => l.id === libraryId);
  const isShowLibrary = library?.type === "LIBRARY_TYPE_SHOWS";

  // Set default type filter based on library type once library data loads
  useEffect(() => {
    if (!library) return;
    if (library.type === "LIBRARY_TYPE_SHOWS") {
      setTypeFilter("MEDIA_ITEM_TYPE_SERIES");
    } else if (library.type === "LIBRARY_TYPE_MOVIES") {
      setTypeFilter("MEDIA_ITEM_TYPE_MOVIE");
    }
  }, [library?.type]);

  const typeOptions = isShowLibrary ? SHOW_TYPE_OPTIONS : MOVIE_TYPE_OPTIONS;

  const params = {
    libraryId,
    ...(typeFilter ? { type: typeFilter } : {}),
    sortField,
    sortOrder: "SORT_ORDER_ASC" as const,
    "pagination.page": 1,
    "pagination.pageSize": page * PAGE_SIZE,
  };

  const { data, isLoading } = useLibraryServiceListMediaItems(params);

  const items = data?.items ?? [];
  const totalItems = data?.pagination?.totalItems ?? 0;
  const hasMore = items.length < totalItems;

  const selectStyle: React.CSSProperties = {
    backgroundColor: "var(--surface-raised)",
    color: "var(--text)",
    border: "1px solid var(--border)",
    borderRadius: "var(--radius-sm)",
    padding: "0.375rem 0.75rem",
    fontSize: "0.875rem",
  };

  return (
    <div>
      <div className="flex items-center justify-between gap-4 mb-6 flex-wrap">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          {library?.name ?? "Library"}
        </h1>
        <div className="flex items-center gap-3">
          {typeOptions.length > 1 && (
            <select
              value={typeFilter}
              onChange={(e) => {
                setTypeFilter(e.target.value as LibraryServiceListMediaItemsType | "");
                setPage(1);
              }}
              style={selectStyle}
            >
              {typeOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  {o.label}
                </option>
              ))}
            </select>
          )}
          <select
            value={sortField}
            onChange={(e) => {
              setSortField(e.target.value as LibraryServiceListMediaItemsSortField);
              setPage(1);
            }}
            style={selectStyle}
          >
            {SORT_OPTIONS.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
      </div>

      <MediaGrid isLoading={isLoading}>
        {items.map((item) => (
          <MediaCard key={item.id} item={item} />
        ))}
      </MediaGrid>

      {!isLoading && items.length === 0 && (
        <div
          className="text-center py-16 rounded-[var(--radius-lg)]"
          style={{
            backgroundColor: "var(--surface)",
            border: "1px solid var(--border)",
          }}
        >
          <p className="text-lg mb-2" style={{ color: "var(--text)" }}>
            No items found.
          </p>
          <p className="mb-4" style={{ color: "var(--text-muted)" }}>
            {typeFilter
              ? "Try a different filter, or scan the library to add content."
              : "This library is empty. Trigger a scan to add content."}
          </p>
          <a
            href="/admin/libraries"
            className="inline-block px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium"
            style={{ backgroundColor: "var(--accent)", color: "#fff" }}
          >
            Go to Admin → Libraries
          </a>
        </div>
      )}

      {hasMore && !isLoading && (
        <div className="flex justify-center mt-8">
          <Button variant="secondary" onClick={() => setPage((p) => p + 1)}>
            Load more
          </Button>
        </div>
      )}

      {isLoading && items.length > 0 && (
        <div className="flex justify-center mt-8">
          <Spinner />
        </div>
      )}
    </div>
  );
}
