import { useState } from "react";
import { Folder, List, Trash2 } from "lucide-react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { Skeleton } from "@/components/ui/Skeleton";
import { usePlaylistList, useDeletePlaylist, type Playlist } from "@/features/playlist/api";
import { CreatePlaylistModal } from "@/features/playlist/CreatePlaylistModal";
import { toast } from "@evey/store";
import { useDemoLock } from "@/hooks/useDemoMode";

export const Route = createLazyFileRoute("/_auth/playlists/")({
  component: PlaylistsIndex,
});

function PlaylistCardSkeleton() {
  return (
    <div className="p-5 bg-[var(--surface)] border border-[var(--border)] rounded-[var(--radius-lg)]">
      <div className="flex items-start gap-3">
        <Skeleton className="w-10 h-10 rounded-lg flex-shrink-0" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-5 w-2/3" />
          <Skeleton className="h-3 w-1/2" />
        </div>
      </div>
    </div>
  );
}

function PlaylistCard({ playlist, onDelete }: { playlist: Playlist; onDelete: () => void }) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  return (
    <div
      className="group p-5 transition-all duration-200 bg-[var(--surface)] border border-[var(--border)] rounded-[var(--radius-lg)]"
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLElement).style.borderColor = "var(--accent)";
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLElement).style.borderColor = "var(--border)";
      }}
    >
      <div className="flex items-start gap-3">
        <div
          className="flex-shrink-0 w-10 h-10 rounded-lg flex items-center justify-center bg-[var(--surface-raised)] text-[var(--accent)]"
        >
          <List size={20} strokeWidth={1.5} />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <Link to="/playlists/$playlistId" params={{ playlistId: playlist.id }} className="text-base font-semibold truncate hover:underline text-[var(--text)]">
              {playlist.name}
            </Link>
            <Badge variant={playlist.type === "smart" ? "warning" : "info"}>
              {playlist.type}
            </Badge>
          </div>
          {playlist.description && (
            <p className="text-xs truncate text-[var(--text-muted)]">{playlist.description}</p>
          )}
          <p className="text-xs mt-1 text-[var(--text-muted)]">
            {t("media.created")} {new Date(playlist.created_at).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}
          </p>
        </div>
        <button
          {...lock}
          onClick={(e) => { e.preventDefault(); onDelete(); }}
          className="opacity-0 group-hover:opacity-100 p-1.5 rounded-full hover:bg-[var(--surface-raised)] transition-all text-[var(--text-muted)]"
          title={t("media.playlist.deletePlaylist")}
          aria-label={t("media.playlist.deletePlaylist")}
        >
          <Trash2 size={16} aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}

function PlaylistsIndex() {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const { data: playlists, isLoading, isError, error, refetch } = usePlaylistList();
  const deletePlaylist = useDeletePlaylist();
  const [createOpen, setCreateOpen] = useState(false);

  function handleDelete(id: string, name: string) {
    if (!confirm(t("media.playlist.deleteConfirm", { name }))) return;
    deletePlaylist.mutate(id, {
      onSuccess: () => toast.success(t("media.playlist.deleted", { name })),
      onError: () => toast.error(t("media.playlist.deleteFailed")),
    });
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-2xl font-bold text-[var(--text)]">{t("media.playlist.playlists")}</h1>
        <Button {...lock} onClick={() => setCreateOpen(true)} data-testid="playlist-create-btn">{t("media.playlist.newPlaylist")}</Button>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
          {Array.from({ length: 3 }).map((_, i) => <PlaylistCardSkeleton key={i} />)}
        </div>
      ) : isError ? (
        // Failing to load them is not the same as having none, and the empty
        // state invited the person to create a playlist they already own.
        <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
          <FetchErrorState error={error} onRetry={() => void refetch()} />
        </div>
      ) : !playlists?.length ? (
        <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
          <EmptyState
            icon={<Folder size={48} strokeWidth={1.5} />}
            title={t("media.playlist.noPlaylists")}
            description={t("media.playlist.noPlaylistsDesc")}
            // No create offer on the demo, where playlists are the shared account's.
            action={lock.disabled ? undefined : { label: t("media.playlist.createPlaylist"), onClick: () => setCreateOpen(true) }}
          />
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
          {playlists.map((pl) => (
            <PlaylistCard key={pl.id} playlist={pl} onDelete={() => handleDelete(pl.id, pl.name)} />
          ))}
        </div>
      )}

      <CreatePlaylistModal open={createOpen} onClose={() => setCreateOpen(false)} />
    </div>
  );
}
