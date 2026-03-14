import { useState, useMemo, useCallback, useEffect } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceMarkAsUnwatched } from "@evey/api";
import { useFavoritesList } from "@/hooks/useFavorite";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";
import { EmptyState } from "@/components/ui/EmptyState";
import { MediaGridSkeleton } from "@/components/ui/Skeleton";
import { BulkActionBar } from "@/components/BulkActionBar";
import { useSelection } from "@/hooks/useSelection";
import { usePlaylistList, useAddToPlaylist } from "@/features/playlist/api";
import { AddToCollectionModal } from "@/features/collection/AddToCollectionModal";
import { Modal } from "@/components/ui/Modal";
import { toast, useAuthStore } from "@evey/store";
import { useQueryClient } from "@tanstack/react-query";

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

function FavoritesPage() {
  const { data: favorites, isLoading } = useFavoritesList(200);
  const [sort, setSort] = useState<SortKey>("dateAdded");
  const [typeFilter, setTypeFilter] = useState<TypeFilter>("all");
  const [search, setSearch] = useState("");
  const [bulkMode, setBulkMode] = useState(false);
  const [bulkPlaylistOpen, setBulkPlaylistOpen] = useState(false);
  const [bulkCollectionOpen, setBulkCollectionOpen] = useState(false);

  const { selected, toggle, selectAll, clear, isSelected, count } = useSelection<string>();
  const profileId = useAuthStore((s) => s.profileId);
  const queryClient = useQueryClient();
  const markWatched = usePlaybackServiceMarkAsWatched();
  const markUnwatched = usePlaybackServiceMarkAsUnwatched();

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape" && bulkMode) {
        setBulkMode(false);
        clear();
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [bulkMode, clear]);

  const handleSelect = useCallback((_id: string, _sel: boolean) => {
    toggle(_id);
  }, [toggle]);

  async function bulkMarkWatchedFn(watched: boolean) {
    if (!profileId || count === 0) return;
    const ids = Array.from(selected);
    const fn = watched ? markWatched.mutateAsync : markUnwatched.mutateAsync;
    await Promise.all(ids.map((mediaItemId) => fn({ mediaItemId, params: { profileId } })));
    ids.forEach((id) => queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${id}`] }));
    toast.success(`${ids.length} item${ids.length > 1 ? "s" : ""} marked as ${watched ? "watched" : "unwatched"}`);
    clear();
    setBulkMode(false);
  }

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
        break;
    }
    return list;
  }, [favorites, sort, typeFilter, search]);

  return (
    <div>
      {bulkPlaylistOpen && (
        <BulkPlaylistModal
          open={bulkPlaylistOpen}
          mediaIds={Array.from(selected)}
          onClose={() => { setBulkPlaylistOpen(false); clear(); setBulkMode(false); }}
        />
      )}
      {bulkCollectionOpen && (
        <AddToCollectionModal
          open={bulkCollectionOpen}
          mediaItemIds={Array.from(selected)}
          onClose={() => { setBulkCollectionOpen(false); clear(); setBulkMode(false); }}
        />
      )}

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
            <button
              type="button"
              onClick={() => { setBulkMode((v) => !v); clear(); }}
              className="px-2 py-1 rounded-[var(--radius-md)] border text-xs transition-colors"
              style={{
                backgroundColor: bulkMode ? "var(--accent)" : "var(--surface-raised)",
                color: bulkMode ? "#fff" : "var(--text-muted)",
                borderColor: bulkMode ? "var(--accent)" : "var(--border)",
              }}
            >
              Select
            </button>
            {bulkMode && (
              <button
                type="button"
                onClick={() => selectAll(filtered.map((f) => f.mediaItemId))}
                className="text-xs hover:underline"
                style={{ color: "var(--accent)" }}
              >
                Select all
              </button>
            )}
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
              selected={isSelected(fav.mediaItemId)}
              selectionMode={bulkMode}
              onSelect={bulkMode ? handleSelect : undefined}
            />
          ))}
        </MediaGrid>
      )}

      <BulkActionBar
        count={count}
        onClear={() => { clear(); setBulkMode(false); }}
        onMarkWatched={() => void bulkMarkWatchedFn(true)}
        onMarkUnwatched={() => void bulkMarkWatchedFn(false)}
        onAddToPlaylist={() => setBulkPlaylistOpen(true)}
        onAddToCollection={() => setBulkCollectionOpen(true)}
        markWatchedPending={markWatched.isPending}
        markUnwatchedPending={markUnwatched.isPending}
      />
    </div>
  );
}
