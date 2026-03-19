import { useState } from "react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Skeleton } from "@/components/ui/Skeleton";
import { usePlaylistList, useDeletePlaylist, type Playlist } from "@/features/playlist/api";
import { CreatePlaylistModal } from "@/features/playlist/CreatePlaylistModal";
import { toast } from "@evey/store";

export const Route = createLazyFileRoute("/_auth/playlists/")({
  component: PlaylistsIndex,
});

function PlaylistCardSkeleton() {
  return (
    <div className="p-5" style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--radius-lg)" }}>
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
  return (
    <div
      className="group p-5 transition-all duration-200"
      style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)", borderRadius: "var(--radius-lg)" }}
      onMouseEnter={(e) => {
        (e.currentTarget as HTMLElement).style.borderColor = "var(--accent)";
      }}
      onMouseLeave={(e) => {
        (e.currentTarget as HTMLElement).style.borderColor = "var(--border)";
      }}
    >
      <div className="flex items-start gap-3">
        <div
          className="flex-shrink-0 w-10 h-10 rounded-lg flex items-center justify-center"
          style={{ backgroundColor: "var(--surface-raised)", color: "var(--accent)" }}
        >
          <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 12h16.5m-16.5 3.75h16.5M3.75 19.5h16.5M5.625 4.5h12.75a1.875 1.875 0 010 3.75H5.625a1.875 1.875 0 010-3.75z" />
          </svg>
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <Link to="/playlists/$playlistId" params={{ playlistId: playlist.id }} className="text-base font-semibold truncate hover:underline" style={{ color: "var(--text)" }}>
              {playlist.name}
            </Link>
            <Badge variant={playlist.type === "smart" ? "warning" : "info"}>
              {playlist.type}
            </Badge>
          </div>
          {playlist.description && (
            <p className="text-xs truncate" style={{ color: "var(--text-muted)" }}>{playlist.description}</p>
          )}
          <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>
            {t("media.created")} {new Date(playlist.created_at).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" })}
          </p>
        </div>
        <button
          onClick={(e) => { e.preventDefault(); onDelete(); }}
          className="opacity-0 group-hover:opacity-100 p-1.5 rounded-full hover:bg-[var(--surface-raised)] transition-all"
          style={{ color: "var(--text-muted)" }}
          title={t("media.playlist.deletePlaylist")}
        >
          <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
            <path strokeLinecap="round" strokeLinejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
          </svg>
        </button>
      </div>
    </div>
  );
}

function PlaylistsIndex() {
  const { t } = useTranslation();
  const { data: playlists, isLoading } = usePlaylistList();
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
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>{t("media.playlist.playlists")}</h1>
        <Button onClick={() => setCreateOpen(true)}>{t("media.playlist.newPlaylist")}</Button>
      </div>

      {isLoading ? (
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
          {Array.from({ length: 3 }).map((_, i) => <PlaylistCardSkeleton key={i} />)}
        </div>
      ) : !playlists?.length ? (
        <div className="rounded-[var(--radius-lg)]" style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}>
          <EmptyState
            icon="library"
            title={t("media.playlist.noPlaylists")}
            description={t("media.playlist.noPlaylistsDesc")}
            action={{ label: t("media.playlist.createPlaylist"), onClick: () => setCreateOpen(true) }}
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
