import { useState, useEffect } from "react";
import { createFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceSearchMedia } from "@evey/api";
import { Input } from "@/components/ui/Input";
import { Spinner } from "@/components/ui/Spinner";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";

export const Route = createFileRoute("/_auth/search")({
  component: SearchPage,
});

function SearchPage() {
  const [inputValue, setInputValue] = useState("");
  const [query, setQuery] = useState("");

  useEffect(() => {
    const timer = setTimeout(() => setQuery(inputValue.trim()), 300);
    return () => clearTimeout(timer);
  }, [inputValue]);

  const { data, isLoading } = useQuery({
    queryKey: ["/api/v1/media/search", query],
    queryFn: ({ signal }) => libraryServiceSearchMedia({ query }, signal),
    enabled: query.length > 1,
  });

  const items = data?.items ?? [];

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

      {query.length > 1 && isLoading && (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      )}

      {query.length > 1 && !isLoading && items.length === 0 && (
        <div className="text-center py-16" style={{ color: "var(--text-muted)" }}>
          No results for &ldquo;{query}&rdquo;.
        </div>
      )}

      {query.length <= 1 && !isLoading && (
        <div className="text-center py-16" style={{ color: "var(--text-muted)" }}>
          Type at least 2 characters to search.
        </div>
      )}

      {items.length > 0 && (
        <>
          <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
            {data?.totalHits ?? items.length} result{(data?.totalHits ?? items.length) !== 1 ? "s" : ""} for &ldquo;{query}&rdquo;
          </p>
          <MediaGrid>
            {items.map((item) => (
              <MediaCard key={item.id} item={item} />
            ))}
          </MediaGrid>
        </>
      )}
    </div>
  );
}
