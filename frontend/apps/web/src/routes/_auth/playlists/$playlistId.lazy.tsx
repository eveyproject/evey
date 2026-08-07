import { Folder, X, Download } from "lucide-react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { BackButton } from "@/components/ui/BackButton";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { toast, useAuthStore } from "@evey/store";
import { startTokenDownload } from "@/utils/downloadToken";
import { usePlaylistDetail, useRemoveFromPlaylist, type PlaylistItem } from "@/features/playlist/api";
import { SmartPlaylistRulesEditor } from "@/features/playlist/SmartPlaylistRulesEditor";

export const Route = createLazyFileRoute("/_auth/playlists/$playlistId")({
  component: PlaylistDetail,
});

function PlaylistItemRow({ item, playlistId, playlistType }: { item: PlaylistItem; playlistId: string; playlistType: string }) {
  const { t } = useTranslation();
  const remove = useRemoveFromPlaylist();

  return (
    <div
      className="flex items-center gap-3 px-4 py-3 transition-colors-fast hover:bg-[var(--surface-raised)] text-[var(--text)]"
    >
      <span className="text-sm w-8 flex-shrink-0 font-mono text-[var(--accent)]">
        {item.sort_order != null ? item.sort_order + 1 : "-"}
      </span>
      <Link
        to="/media/$mediaId"
        params={{ mediaId: item.media_item_id }}
        className="text-sm flex-1 truncate hover:underline"
      >
        {item.title ?? item.media_item_id}
      </Link>
      {item.type && (
        <Badge variant="default">{item.type.replace("MEDIA_ITEM_TYPE_", "").toLowerCase()}</Badge>
      )}
      {item.year && (
        <span className="text-xs flex-shrink-0 text-[var(--text-muted)]">{item.year}</span>
      )}
      {playlistType === "manual" && (
        <button
          onClick={() => {
            remove.mutate(
              { playlistId, mediaItemId: item.media_item_id },
              {
                onSuccess: () => toast.success(t("media.playlist.removedFromPlaylist")),
                onError: () => toast.error(t("media.playlist.failedRemove")),
              },
            );
          }}
          disabled={remove.isPending}
          className="p-1 rounded hover:bg-[var(--surface-raised)] transition-colors-fast flex-shrink-0 text-[var(--text-muted)]"
          title={t("common.remove")}
          aria-label={t("common.remove")}
        >
          <X size={16} aria-hidden="true" />
        </button>
      )}
    </div>
  );
}

function PlaylistDetail() {
  const { t } = useTranslation();
  const { playlistId } = Route.useParams();
  const { data: playlist, isLoading, isError, error, refetch } = usePlaylistDetail(playlistId);
  const accessToken = useAuthStore((s) => s.accessToken);

  if (isLoading) {
    return (
      <div className="flex justify-center py-16"><Spinner /></div>
    );
  }

  // A failed fetch is NOT "this playlist does not exist". Both used to render
  // the same sentence, so a server that did not answer told the user their
  // playlist was gone - and offered nothing to do about it.
  if (isError) {
    return <FetchErrorState error={error} onRetry={() => void refetch()} />;
  }

  if (!playlist) {
    return (
      <div className="text-center py-16 text-[var(--text-muted)]">
        {t("media.playlist.notFound")}
      </div>
    );
  }

  const items = playlist.items ?? [];

  return (
    <div>
      <BackButton fallback="/playlists" className="mb-4" />
      <div className="flex items-center justify-between gap-3 mb-2">
        <div className="flex items-center gap-3 min-w-0">
          <h1 className="text-2xl font-bold truncate text-[var(--text)]">{playlist.name}</h1>
          <Badge variant={playlist.type === "smart" ? "warning" : "info"}>{playlist.type}</Badge>
        </div>
        {items.length > 0 && accessToken && (
          <button
            type="button"
            onClick={async () => {
              // Click-time mint: the account JWT must not ride in the URL
              // (proxy logs, history) - see utils/downloadToken.ts.
              try {
                await startTokenDownload(`/api/v1/playlists/${playlistId}/download`, playlistId, `${playlist.name ?? "playlist"}.zip`);
              } catch {
                toast.error(t("common.error"));
              }
            }}
            data-testid="playlist-download-btn"
            className="inline-flex items-center gap-2 px-3 py-1.5 rounded-[var(--radius-md)] text-sm font-medium bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)] transition-colors-fast flex-shrink-0 cursor-pointer"
          >
            <Download size={16} />
            {t("media.playlist.download")}
          </button>
        )}
      </div>
      {playlist.description && (
        <p className="text-sm mb-4 text-[var(--text-muted)]">{playlist.description}</p>
      )}

      {playlist.type === "smart" && (
        <SmartPlaylistRulesEditor
          playlistId={playlistId}
          initialRules={playlist.rules ?? []}
        />
      )}

      {items.length === 0 ? (
        <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
          <EmptyState
            icon={<Folder size={48} strokeWidth={1.5} />}
            title={t("media.playlist.noItems")}
            description={playlist.type === "smart" ? t("media.playlist.noItemsSmart") : t("media.playlist.noItemsManual")}
          />
        </div>
      ) : (
        <div className="border border-[var(--border)] rounded-[var(--radius-md)]">
          <div className="divide-y divide-[var(--border)]">
            {items.map((item) => (
              <PlaylistItemRow
                key={item.media_item_id}
                item={item}
                playlistId={playlistId}
                playlistType={playlist.type}
              />
            ))}
          </div>
          <div className="px-4 py-2 text-xs text-[var(--text-muted)] border-t border-[var(--border)]">
            {t("common.item", { count: items.length })}
          </div>
        </div>
      )}
    </div>
  );
}
