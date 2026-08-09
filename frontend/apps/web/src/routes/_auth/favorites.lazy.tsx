import { useState, useMemo, useCallback, useEffect } from "react";
import { Folder } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { usePlaybackServiceMarkAsWatched, usePlaybackServiceMarkAsUnwatched } from "@evey/api";
import { useTranslation } from "react-i18next";
import { useFavoritesList } from "@/hooks/useFavorite";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCardWithModals as MediaCard } from "@/features/library/MediaCardWithModals";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
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

const selectClass = "px-2 py-1 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] text-[0.8rem] cursor-pointer";

const searchInputClass = "px-2 py-1 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] text-[0.8rem] outline-none w-40";

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

  const { t } = useTranslation();

  async function handleAdd(playlistId: string, playlistName: string) {
    try {
      await Promise.all(mediaIds.map((id) => addTo.mutateAsync({ playlistId, mediaItemId: id })));
      toast.success(t("media.favorites.itemsAddedToPlaylist", { count: mediaIds.length, name: playlistName }));
      onClose();
    } catch {
      toast.error(t("media.favorites.failedAddPlaylist"));
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("media.favorites.addToPlaylist")}>
      {isLoading ? (
        <div className="flex justify-center py-8">
          <div className="w-6 h-6 border-2 rounded-full animate-spin border-[var(--border)] border-t-[var(--accent)]" />
        </div>
      ) : manualPlaylists.length === 0 ? (
        <p className="text-sm text-center py-6 text-[var(--text-muted)]">{t("media.favorites.noPlaylists")}</p>
      ) : (
        <div className="space-y-1">
          {manualPlaylists.map((pl) => (
            <button
              key={pl.id}
              onClick={() => void handleAdd(pl.id, pl.name)}
              disabled={addTo.isPending}
              className="w-full flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] text-left transition-colors-fast hover:bg-[var(--surface-raised)]"
            >
              <span className="text-sm font-medium truncate text-[var(--text)]">{pl.name}</span>
            </button>
          ))}
        </div>
      )}
    </Modal>
  );
}

function FavoritesPage() {
  const { t } = useTranslation();
    const { data: favorites, isLoading, isError, error, refetch } = useFavoritesList();
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
    try {
      await Promise.all(ids.map((mediaItemId) => fn({ mediaItemId, params: { profileId } })));
      ids.forEach((id) => queryClient.invalidateQueries({ queryKey: [`/api/v1/media/${id}`] }));
      // Mirror the single-card path (MediaCard.invalidateItem): refresh watch
      // history so the "watched" badges (useWatchedMap) update, and the home
      // feeds that filter watched items server-side.
      void queryClient.invalidateQueries({ queryKey: ["/api/v1/playback/history"] });
      void queryClient.invalidateQueries({
        predicate: (q) => {
          const k = q.queryKey?.[0];
          if (typeof k !== "string") return false;
          return k.startsWith("/api/v1/feeds/") || k === "/api/v1/popular";
        },
      });
      toast.success(watched ? t("media.favorites.markedWatched", { count: ids.length }) : t("media.favorites.markedUnwatched", { count: ids.length }));
      clear();
      setBulkMode(false);
    } catch {
      toast.error(t("common.error"));
    }
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
        <h1 className="text-2xl font-bold text-[var(--text)]">
          {t("media.favorites.title")}
        </h1>
        {!isLoading && favorites && favorites.length > 0 && (
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-sm text-[var(--text-muted)]">
              {t("common.itemCount", { count: filtered.length })}
            </span>
            <input
              type="search"
              placeholder={t("media.favorites.searchPlaceholder")}
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className={searchInputClass}
              aria-label={t("media.favorites.searchPlaceholder")}
            />
            <select className={selectClass} value={typeFilter} onChange={(e) => setTypeFilter(e.target.value as TypeFilter)} aria-label={t("media.favorites.filterByType", "Filter by type")}>
              <option value="all">{t("media.favorites.all")}</option>
              <option value="MEDIA_ITEM_TYPE_MOVIE">{t("media.favorites.movies")}</option>
              <option value="MEDIA_ITEM_TYPE_SERIES">{t("media.favorites.series")}</option>
            </select>
            <select className={selectClass} value={sort} onChange={(e) => setSort(e.target.value as SortKey)} aria-label={t("media.favorites.sortBy", "Sort by")}>
              <option value="dateAdded">{t("media.favorites.recentlyAdded")}</option>
              <option value="titleAZ">{t("media.favorites.titleAZ")}</option>
              <option value="titleZA">{t("media.favorites.titleZA")}</option>
              <option value="yearNewest">{t("media.favorites.yearNewest")}</option>
              <option value="rating">{t("media.favorites.rating")}</option>
            </select>
            <button
              type="button"
              onClick={() => { setBulkMode((v) => !v); clear(); }}
              className="px-2 py-1 rounded-[var(--radius-md)] border text-xs transition-colors-fast"
              style={{
                backgroundColor: bulkMode ? "var(--accent)" : "var(--surface-raised)",
                color: bulkMode ? "var(--filter-active-text)" : "var(--text-muted)",
                borderColor: bulkMode ? "var(--accent)" : "var(--border)",
              }}
              aria-label={t("common.select")}
              aria-pressed={bulkMode}
            >
              {t("common.select")}
            </button>
            {bulkMode && (
              <button
                type="button"
                onClick={() => selectAll(filtered.map((f) => f.mediaItemId))}
                className="text-xs hover:underline text-[var(--accent)]"
                aria-label={t("common.selectAll")}
              >
                {t("common.selectAll")}
              </button>
            )}
          </div>
        )}
      </div>

      {isLoading ? (
        <MediaGridSkeleton />
      ) : isError ? (
        // Same failure/emptiness bug as library/photos/music: without this branch
        // a failed fetch renders as "no favorites yet", telling the user their
        // list is empty when the server is actually unreachable.
        <div
          className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <FetchErrorState error={error} onRetry={() => void refetch()} />
        </div>
      ) : !favorites || favorites.length === 0 ? (
        <div
          className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <EmptyState
            icon={<Folder size={48} strokeWidth={1.5} />}
            title={t("media.favorites.noFavorites")}
            description={t("media.favorites.noFavoritesDesc")}
          />
        </div>
      ) : filtered.length === 0 ? (
        <p className="text-sm text-[var(--text-muted)]">{t("common.noItemsMatch")}</p>
      ) : (
        <MediaGrid>
          {filtered.map((fav) => (
            <MediaCard
              key={fav.mediaItemId}
              item={{ id: fav.mediaItemId, title: fav.title, type: fav.type }}
              selected={isSelected(fav.mediaItemId)}
              selectionMode={bulkMode}
              onSelect={bulkMode ? handleSelect : undefined}
              lite
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
