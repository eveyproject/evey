import { useState, useEffect } from "react";
import { createLazyFileRoute, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceSearchMedia } from "@evey/api";
import { Input } from "@/components/ui/Input";
import { EmptyState } from "@/components/ui/EmptyState";
import { MediaGridSkeleton } from "@/components/ui/Skeleton";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";

export const Route = createLazyFileRoute("/_auth/search")({
  component: SearchPage,
});

function SearchPage() {
  const { q } = Route.useSearch();
  const navigate = useNavigate();

  const [inputValue, setInputValue] = useState(q ?? "");
  const [debouncedQuery, setDebouncedQuery] = useState(q?.trim() ?? "");

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
      void navigate({
        to: "/search",
        search: trimmed ? { q: trimmed } : {},
        replace: true,
      });
    }, 300);
    return () => clearTimeout(timer);
  }, [inputValue, navigate]);

  const { data, isLoading } = useQuery({
    queryKey: ["/api/v1/media/search", debouncedQuery],
    queryFn: ({ signal }) => libraryServiceSearchMedia({ query: debouncedQuery }, signal),
    enabled: debouncedQuery.length > 1,
  });

  const items = data?.items ?? [];
  const totalHits = data?.totalHits ?? items.length;

  return (
    <div>
      <h1 className="text-2xl font-bold mb-6" style={{ color: "var(--text)" }}>
        Search
      </h1>

      <div className="mb-6 max-w-lg">
        <Input
          type="search"
          placeholder="Search movies, series, music..."
          value={inputValue}
          onChange={(e) => setInputValue(e.target.value)}
          autoFocus
        />
      </div>

      {debouncedQuery.length > 1 && isLoading && (
        <div>
          <div className="h-4 w-32 mb-4 animate-pulse rounded" style={{ backgroundColor: "var(--surface-raised)" }} />
          <MediaGridSkeleton count={12} />
        </div>
      )}

      {debouncedQuery.length > 1 && !isLoading && items.length === 0 && (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <EmptyState
            icon="search"
            title={`No results for \u201c${debouncedQuery}\u201d`}
            description="Try a different search term or check the spelling."
          />
        </div>
      )}

      {debouncedQuery.length <= 1 && !isLoading && (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <EmptyState
            icon="search"
            title="Type at least 2 characters to search"
            description="Search across all your libraries for movies, series, and more."
          />
        </div>
      )}

      {items.length > 0 && (
        <>
          <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
            {totalHits} result{totalHits !== 1 ? "s" : ""} for &ldquo;{debouncedQuery}&rdquo;
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
