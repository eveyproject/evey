import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { toast } from "@evey/store";
import { usePlaylistList, useAddToPlaylist } from "./api";
import { CreatePlaylistModal } from "./CreatePlaylistModal";

interface Props {
  open: boolean;
  onClose: () => void;
  mediaItemId: string;
}

export function AddToPlaylistModal({ open, onClose, mediaItemId }: Props) {
  const { t } = useTranslation("media");
  const { data: playlists, isLoading } = usePlaylistList();
  const addTo = useAddToPlaylist();
  const [createOpen, setCreateOpen] = useState(false);

  const manualPlaylists = (playlists ?? []).filter((p) => p.type === "manual");

  async function handleAdd(playlistId: string, playlistName: string) {
    try {
      await addTo.mutateAsync({ playlistId, mediaItemId });
      toast.success(t("playlist.addSuccess", { name: playlistName }));
      onClose();
    } catch {
      toast.error(t("playlist.addError"));
    }
  }

  return (
    <>
      <Modal open={open && !createOpen} onClose={onClose} title={t("playlist.addTitle")}>
        {isLoading ? (
          <div className="flex justify-center py-8"><Spinner /></div>
        ) : manualPlaylists.length === 0 ? (
          <div className="text-center py-6">
            <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>{t("playlist.noPlaylists")}</p>
            <Button onClick={() => setCreateOpen(true)}>{t("playlist.createPlaylist")}</Button>
          </div>
        ) : (
          <div className="space-y-2">
            {manualPlaylists.map((pl) => (
              <button
                key={pl.id}
                onClick={() => handleAdd(pl.id, pl.name)}
                disabled={addTo.isPending}
                className="w-full flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] text-left transition-colors hover:bg-[var(--surface-raised)]"
                style={{ color: "var(--text)" }}
              >
                <svg className="w-5 h-5 flex-shrink-0" style={{ color: "var(--accent)" }} fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
                  <path strokeLinecap="round" strokeLinejoin="round" d="M3.75 12h16.5m-16.5 3.75h16.5M3.75 19.5h16.5M5.625 4.5h12.75a1.875 1.875 0 010 3.75H5.625a1.875 1.875 0 010-3.75z" />
                </svg>
                <div className="flex-1 min-w-0">
                  <p className="text-sm font-medium truncate">{pl.name}</p>
                  {pl.description && <p className="text-xs truncate" style={{ color: "var(--text-muted)" }}>{pl.description}</p>}
                </div>
              </button>
            ))}
            <div className="pt-2 border-t" style={{ borderColor: "var(--border)" }}>
              <Button variant="secondary" onClick={() => setCreateOpen(true)} className="w-full">
                {t("playlist.newPlaylist")}
              </Button>
            </div>
          </div>
        )}
      </Modal>
      <CreatePlaylistModal open={createOpen} onClose={() => setCreateOpen(false)} />
    </>
  );
}
