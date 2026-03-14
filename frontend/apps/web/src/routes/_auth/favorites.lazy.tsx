import { useState, useMemo } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useFavoritesList } from "@/hooks/useFavorite";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";
import { EmptyState } from "@/components/ui/EmptyState";
import { MediaGridSkeleton } from "@/components/ui/Skeleton";

export const Route = createLazyFileRoute("/_auth/favorites")({
  component: FavoritesPage,
});

type SortKey = "dateAdded" | "titleAZ" | "titleZA" | "yearNewest" | "rating";
type TypeFilter = "all" | "MEDIA_ITEM_TYPE_MOVIE" | "MEDIA_ITEM_TYPE_SERIES";

const selectStyle: React.CSSProperties = {
  padding: "4px 8px",
  borderRadius: "var(--radius-sm)",
  border: "1px solid var(--border)",
  backgroundColor: "var(--surface-raised)",
  color: "var(--text)",
  fontSize: "0.8rem",
  cursor: "pointer",
};

const searchInputStyle: React.CSSProperties = {
  padding: "4px 8px",
  borderRadius: "var(--radius-sm)",
  border: "1px solid var(--border)",
  backgroundColor: "var(--surface-raised)",
  color: "var(--text)",
  fontSize: "0.8rem",
  outline: "none",
  width: "160px",
};

function FavoritesPage() {
  const { data: favorites, isLoading } = useFavoritesList(200);
  const [sort, setSort] = useState<SortKey>("dateAdded");
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");
  const [search, setSearch] = useState("");

  const filtered = useMemo(() => {
    if (!favorites) return [];
    const query = search.trim().toLowerCase();
    let list = favorites.filter((f) => {
      if (typeFilter !== "all" && f.type !== typeFilter) return false;
      if (query && !(f.title ?? "").toLowerCase().includes(query)) return false;
      return true;
    });
    switch (sort) {
      case "titleAZ":
        list = [...list].sort((a, b) => (a.title ?? "").localeCompare(b.title ?? ""));
        break;
      case "titleZA":
        list = [...list].sort((a, b) => (b.title ?? "").localeCompare(a.title ?? ""));
        break;
      case "yearNewest":
        list = [...list].sort((a, b) => (b.year ?? 0) - (a.year ?? 0));
        break;
      case "rating":
        list = [...list].sort((a, b) => (b.communityRating ?? 0) - (a.communityRating ?? 0));
        break;
      default:
        // dateAdded — server order preserved
        break;
    }
    return list;
  }, [favorites, sort, typeFilter, search]);

  return (
    <div>
      <div className="flex items-center justify-between mb-4 gap-3 flex-wrap">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          Favorites
        </h1>
        {!isLoading && favorites && favorites.length > 0 && (
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm" style={{ color: "var(--text-muted)" }}>
              {filtered.length} {filtered.length === 1 ? "item" : "items"}
            </span>
            <input
              type="search"
              placeholder="Search…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              style={searchInputStyle}
              aria-label="Search favorites"
            />
            <select style={selectStyle} value={typeFilter} onChange={(e) => setTypeFilter(e.target.value as TypeFilter)}>
              <option value="all">All</option>
              <option value="MEDIA_ITEM_TYPE_MOVIE">Movies</option>
              <option value="MEDIA_ITEM_TYPE_SERIES">Series</option>
            </select>
            <select style={selectStyle} value={sort} onChange={(e) => setSort(e.target.value as SortKey)}>
              <option value="dateAdded">Recently Added</option>
              <option value="titleAZ">Title A–Z</option>
              <option value="titleZA">Title Z–A</option>
              <option value="yearNewest">Year (newest)</option>
              <option value="rating">Rating</option>
            </select>
          </div>
        )}
      </div>

      {isLoading ? (
        <MediaGridSkeleton />
      ) : !favorites || favorites.length === 0 ? (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <EmptyState
            icon="library"
            title="No favorites yet"
            description="Click the heart icon on any media item to add it to your favorites."
          />
        </div>
      ) : filtered.length === 0 ? (
        <p className="text-sm" style={{ color: "var(--text-muted)" }}>No items match the current filter.</p>
      ) : (
        <MediaGrid>
          {filtered.map((fav) => (
            <MediaCard
              key={fav.mediaItemId}
              item={{ id: fav.mediaItemId, title: fav.title, type: fav.type }}
            />
          ))}
        </MediaGrid>
      )}
    </div>
  );
}
