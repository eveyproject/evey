import { useState, useEffect, useRef } from "react";
import { createLazyFileRoute, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceSearchMedia } from "@evey/api";
import { EmptyState } from "@/components/ui/EmptyState";
import { MediaGridSkeleton } from "@/components/ui/Skeleton";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";
import { useRecentSearches } from "@/hooks/useRecentSearches";

export const Route = createLazyFileRoute("/_auth/search")({
  component: SearchPage,
});

type MediaTypeFilter = "MEDIA_ITEM_TYPE_MOVIE" | "MEDIA_ITEM_TYPE_SERIES";

const typeFilters: { label: string; value: MediaTypeFilter | undefined }[] = [
  { label: "All", value: undefined },
  { label: "Movies", value: "MEDIA_ITEM_TYPE_MOVIE" },
  { label: "Series", value: "MEDIA_ITEM_TYPE_SERIES" },
];

function SearchPage() {
  const { q } = Route.useSearch();
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);

  const [inputValue, setInputValue] = useState(q ?? "");
  const [debouncedQuery, setDebouncedQuery] = useState(q?.trim() ?? "");
  const [typeFilter, setTypeFilter] = useState<MediaTypeFilter | undefined>(undefined);
  const { recents, push: pushRecent, remove: removeRecent, clear: clearRecents } = useRecentSearches();

  useEffect(() => {
    if (q !== undefined && q !== inputValue) {
      setInputValue(q);
      setDebouncedQuery(q.trim());
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [q]);

  useEffect(() => {
    const timer = setTimeout(() => {
      const trimmed = inputValue.trim();
      setDebouncedQuery(trimmed);
      if (trimmed.length > 1) pushRecent(trimmed);
      void navigate({
        to: "/search",
        search: trimmed ? { q: trimmed } : {},
        replace: true,
      });
    }, 300);
    return () => clearTimeout(timer);
  }, [inputValue, navigate, pushRecent]);

  // Focus input on mount
  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const { data, isLoading } = useQuery({
    queryKey: ["/api/v1/media/search", debouncedQuery, typeFilter],
    queryFn: ({ signal }) =>
      libraryServiceSearchMedia(
        {
          query: debouncedQuery,
          ...(typeFilter ? { type: typeFilter } : {}),
          limit: 40,
        },
        signal,
      ),
    enabled: debouncedQuery.length > 1,
  });

  const items = data?.items ?? [];
  const totalHits = data?.totalHits ?? items.length;

  const hasQuery = debouncedQuery.length > 1;

  return (
    <div>
      <div className="mb-6">
        <div className="relative max-w-xl">
          <svg
            className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 pointer-events-none"
            style={{ color: "var(--text-muted)" }}
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
            viewBox="0 0 24 24"
          >
            <circle cx="11" cy="11" r="8" />
            <path strokeLinecap="round" d="m21 21-4.35-4.35" />
          </svg>
          <input
            ref={inputRef}
            type="search"
            placeholder="Search movies, series..."
            data-testid="search-page-input"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            className="w-full pl-10 pr-4 py-3 rounded-[var(--radius-lg)] text-base bg-[var(--surface)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] transition-shadow"
          />
          {inputValue && (
            <button
              onClick={() => {
                setInputValue("");
                inputRef.current?.focus();
              }}
              className="absolute right-3 top-1/2 -translate-y-1/2 w-5 h-5 rounded-full flex items-center justify-center hover:bg-[var(--surface-raised)] transition-colors cursor-pointer"
              style={{ color: "var(--text-muted)" }}
              aria-label="Clear search"
            >
              <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                <path strokeLinecap="round" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>
          )}
        </div>
      </div>

      {hasQuery && (
        <div className="flex items-center gap-1 mb-5" data-testid="search-type-filters">
          {typeFilters.map((f) => (
            <button
              key={f.label}
              data-testid={`search-filter-${f.label.toLowerCase()}`}
              onClick={() => setTypeFilter(f.value)}
              className="px-3 py-1.5 rounded-full text-sm font-medium transition-colors cursor-pointer"
              style={{
                backgroundColor:
                  typeFilter === f.value ? "var(--accent)" : "var(--surface)",
                color:
                  typeFilter === f.value ? "#fff" : "var(--text-secondary)",
                border: `1px solid ${typeFilter === f.value ? "var(--accent)" : "var(--border)"}`,
              }}
            >
              {f.label}
            </button>
          ))}
        </div>
      )}

      {hasQuery && isLoading && (
        <div>
          <div
            className="h-4 w-32 mb-4 animate-pulse rounded"
            style={{ backgroundColor: "var(--surface-raised)" }}
          />
          <MediaGridSkeleton count={12} />
        </div>
      )}

      {hasQuery && !isLoading && items.length === 0 && (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{
            backgroundColor: "var(--surface)",
            border: "1px solid var(--border)",
          }}
        >
          <EmptyState
            icon="search"
            title={`No results for \u201c${debouncedQuery}\u201d`}
            description="Try a different search term, check the spelling, or change the filter."
          />
        </div>
      )}

      {!hasQuery && !isLoading && recents.length > 0 && (
        <div className="mb-6">
          <div className="flex items-center justify-between mb-3">
            <span className="text-sm font-semibold" style={{ color: "var(--text-muted)" }}>Recent searches</span>
            <button
              onClick={clearRecents}
              className="text-xs hover:underline cursor-pointer"
              style={{ color: "var(--text-muted)" }}
            >
              Clear all
            </button>
          </div>
          <div className="flex flex-wrap gap-2">
            {recents.map((r) => (
              <div
                key={r}
                className="flex items-center gap-1 px-3 py-1.5 rounded-full text-sm"
                style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)", color: "var(--text)" }}
              >
                <button
                  className="hover:text-[var(--accent)] cursor-pointer transition-colors"
                  onClick={() => setInputValue(r)}
                >
                  {r}
                </button>
                <button
                  onClick={() => removeRecent(r)}
                  className="ml-1 hover:text-red-400 cursor-pointer transition-colors"
                  style={{ color: "var(--text-muted)" }}
                  aria-label={`Remove ${r}`}
                >
                  <svg className="w-3 h-3" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
                    <path strokeLinecap="round" d="M6 18L18 6M6 6l12 12" />
                  </svg>
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {!hasQuery && !isLoading && recents.length === 0 && (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{
            backgroundColor: "var(--surface)",
            border: "1px solid var(--border)",
          }}
        >
          <EmptyState
            icon="search"
            title="Search your media library"
            description="Type at least 2 characters to search across all your movies, series, and more."
          />
        </div>
      )}

      {items.length > 0 && (
        <>
          <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
            {totalHits} result{totalHits !== 1 ? "s" : ""} for &ldquo;
            {debouncedQuery}&rdquo;
          </p>
          <MediaGrid>
            {items.map((item) => (
              <MediaCard key={item.id} item={item} showType />
            ))}
          </MediaGrid>
        </>
      )}
    </div>
  );
}
