import { useTranslation } from "react-i18next";
import { usePlaylistList, useAddToPlaylist } from "@/features/playlist/api";
import { Modal } from "@/components/ui/Modal";
import { toast } from "@evey/store";

export function BulkPlaylistModal({
  open,
  mediaIds,
  onClose,
}: {
  open: boolean;
  mediaIds: string[];
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { data: playlists, isLoading } = usePlaylistList();
  const addTo = useAddToPlaylist();
  const manualPlaylists = (playlists ?? []).filter((p) => p.type === "manual");

  async function handleAdd(playlistId: string, playlistName: string) {
    try {
      await Promise.all(mediaIds.map((id) => addTo.mutateAsync({ playlistId, mediaItemId: id })));
      toast.success(t("media.library.itemsAddedToPlaylist", { count: mediaIds.length, name: playlistName }));
      onClose();
    } catch {
      toast.error(t("media.library.failedAddPlaylist"));
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("media.library.addToPlaylist")}>
      {isLoading ? (
        <div className="flex justify-center py-8">
          <div className="w-6 h-6 border-2 rounded-full animate-spin border-[var(--border)] border-t-[var(--accent)]" />
        </div>
      ) : manualPlaylists.length === 0 ? (
        <p className="text-sm text-center py-6 text-[var(--text-muted)]">{t("media.library.noPlaylists")}</p>
      ) : (
        <div className="space-y-1">
          {manualPlaylists.map((pl) => (
            <button
              key={pl.id}
              onClick={() => void handleAdd(pl.id, pl.name)}
              disabled={addTo.isPending}
              className="w-full flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] text-left transition-colors-fast hover:bg-[var(--surface-raised)] text-[var(--text)]"
            >
              <span className="text-sm font-medium truncate">{pl.name}</span>
            </button>
          ))}
        </div>
      )}
    </Modal>
  );
}
