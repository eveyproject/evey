import { Folder, X } from "lucide-react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/Badge";
import { Spinner } from "@/components/ui/Spinner";
import { BackButton } from "@/components/ui/BackButton";
import { EmptyState } from "@/components/ui/EmptyState";
import { toast } from "@evey/store";
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
      className="flex items-center gap-3 px-4 py-3 transition-colors-fast hover:bg-[var(--surface-raised)]"
      style={{ color: "var(--text)" }}
    >
      <span className="text-sm w-8 flex-shrink-0 font-mono" style={{ color: "var(--accent)" }}>
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
        <span className="text-xs flex-shrink-0" style={{ color: "var(--text-muted)" }}>{item.year}</span>
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
          className="p-1 rounded hover:bg-[var(--surface-raised)] transition-colors-fast flex-shrink-0"
          style={{ color: "var(--text-muted)" }}
          title={t("common.remove")}
        >
          <X size={16} />
        </button>
      )}
    </div>
  );
}

function PlaylistDetail() {
  const { t } = useTranslation();
  const { playlistId } = Route.useParams();
  const { data: playlist, isLoading, isError } = usePlaylistDetail(playlistId);

  if (isLoading) {
    return (
      <div className="flex justify-center py-16"><Spinner /></div>
    );
  }

  if (isError || !playlist) {
    return (
      <div className="text-center py-16" style={{ color: "var(--text-muted)" }}>
        {t("media.playlist.notFound")}
      </div>
    );
  }

  const items = playlist.items ?? [];

  return (
    <div>
      <BackButton fallback="/playlists" className="mb-4" />
      <div className="flex items-center gap-3 mb-2">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>{playlist.name}</h1>
        <Badge variant={playlist.type === "smart" ? "warning" : "info"}>{playlist.type}</Badge>
      </div>
      {playlist.description && (
        <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>{playlist.description}</p>
      )}

      {playlist.type === "smart" && (
        <SmartPlaylistRulesEditor
          playlistId={playlistId}
          initialRules={playlist.rules ?? []}
        />
      )}

      {items.length === 0 ? (
        <div className="rounded-[var(--radius-lg)]" style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}>
          <EmptyState
            icon={<Folder size={48} strokeWidth={1.5} />}
            title={t("media.playlist.noItems")}
            description={playlist.type === "smart" ? t("media.playlist.noItemsSmart") : t("media.playlist.noItemsManual")}
          />
        </div>
      ) : (
        <div style={{ border: "1px solid var(--border)", borderRadius: "var(--radius-md)" }}>
          <div className="divide-y" style={{ borderColor: "var(--border)" }}>
            {items.map((item) => (
              <PlaylistItemRow
                key={item.media_item_id}
                item={item}
                playlistId={playlistId}
                playlistType={playlist.type}
              />
            ))}
          </div>
          <div className="px-4 py-2 text-xs" style={{ color: "var(--text-muted)", borderTop: "1px solid var(--border)" }}>
            {items.length === 1 ? t("common.item", { count: items.length }) : t("common.items", { count: items.length })}
          </div>
        </div>
      )}
    </div>
  );
}
