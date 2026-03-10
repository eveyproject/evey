import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { libraryServiceGetMediaItem } from "@evey/api";
import { useFavoritesList } from "@/hooks/useFavorite";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";
import { EmptyState } from "@/components/ui/EmptyState";
import { MediaGridSkeleton } from "@/components/ui/Skeleton";

export const Route = createLazyFileRoute("/_auth/favorites")({
  component: FavoritesPage,
});

function FavoriteMediaCard({ mediaItemId }: { mediaItemId: string }) {
  const { data: item } = useQuery({
    queryKey: [`/api/v1/media/${mediaItemId}`],
    queryFn: ({ signal }) => libraryServiceGetMediaItem(mediaItemId, signal),
    staleTime: 60_000,
  });

  if (!item) return null;

  return <MediaCard item={item} />;
}

function FavoritesPage() {
  const { data: favorites, isLoading } = useFavoritesList(200);

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
          Favorites
        </h1>
        {!isLoading && favorites && favorites.length > 0 && (
          <span className="text-sm" style={{ color: "var(--text-muted)" }}>
            {favorites.length} {favorites.length === 1 ? "item" : "items"}
          </span>
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
      ) : (
        <MediaGrid>
          {favorites.map((fav) => (
            <FavoriteMediaCard key={fav.mediaItemId} mediaItemId={fav.mediaItemId} />
          ))}
        </MediaGrid>
      )}
    </div>
  );
}
