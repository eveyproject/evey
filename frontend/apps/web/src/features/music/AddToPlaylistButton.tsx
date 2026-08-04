import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  usePlaylistServiceListPlaylists,
  usePlaylistServiceAddPlaylistItem,
  usePlaylistServiceCreatePlaylist,
  getPlaylistServiceListPlaylistsQueryKey,
  V1PlaylistType,
} from "@evey/api";
import { toast } from "@evey/store";
import { Plus } from "lucide-react";
import { ICON } from "@/constants/iconSizes";

export function AddToPlaylistButton({ trackID }: { trackID: string }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const queryClient = useQueryClient();
  const { data: playlistsData } = usePlaylistServiceListPlaylists(undefined, {
    query: { enabled: open, staleTime: 30_000 },
  });
  const addItem = usePlaylistServiceAddPlaylistItem();
  const createPlaylist = usePlaylistServiceCreatePlaylist();

  useEffect(() => {
    if (!open) return;
    function onClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, [open]);

  const playlists = (playlistsData?.playlists ?? []).filter(
    (p) => p.type !== V1PlaylistType.PLAYLIST_TYPE_SMART,
  );

  async function addToPlaylist(playlistId: string) {
    try {
      await addItem.mutateAsync({
        id: playlistId,
        data: { mediaItemId: trackID, mediaType: "music" },
      });
      toast.success(t("media.music.addedToPlaylist"));
    } catch (err) {
      console.warn("add to playlist failed", err);
      toast.error(t("media.playlist.addError"));
    }
    setOpen(false);
  }

  async function createAndAdd() {
    const name = window.prompt(t("media.music.createPlaylistPrompt"));
    if (!name || !name.trim()) {
      setOpen(false);
      return;
    }
    try {
      const created = await createPlaylist.mutateAsync({
        data: { name: name.trim(), type: V1PlaylistType.PLAYLIST_TYPE_MANUAL },
      });
      await queryClient.invalidateQueries({ queryKey: getPlaylistServiceListPlaylistsQueryKey() });
      toast.success(t("media.music.playlistCreated"));
      if (created?.id) {
        await addItem.mutateAsync({
          id: created.id,
          data: { mediaItemId: trackID, mediaType: "music" },
        });
        toast.success(t("media.music.addedToPlaylist"));
      }
    } catch (err) {
      // The success toast for the creation has already been shown by the time
      // the add can fail, so silence here read as "the track is in it".
      console.warn("create + add failed", err);
      toast.error(t("media.playlist.addError"));
    }
    setOpen(false);
  }

  return (
    <div className="relative inline-block" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-label={t("media.music.addToPlaylist")}
        title={t("media.music.addToPlaylist")}
        className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--accent)] p-1 rounded"
      >
        <Plus size={ICON.lg} />
      </button>
      {open && (
        <div
          className="absolute right-0 mt-1 z-popover min-w-[200px] max-h-[300px] overflow-y-auto rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface-raised)] shadow-lg"
          role="menu"
        >
          {playlists.length === 0 && (
            <div className="px-3 py-2 text-xs text-[var(--text-muted)]">
              {t("media.music.addToPlaylist")}
            </div>
          )}
          {playlists.map((p) => (
            <button
              key={p.id}
              type="button"
              role="menuitem"
              onClick={() => p.id && addToPlaylist(p.id)}
              className="w-full text-left px-3 py-2 text-sm bg-transparent border-none cursor-pointer text-[var(--text)] hover:bg-[var(--surface)]"
            >
              {p.name || "-"}
            </button>
          ))}
          <button
            type="button"
            role="menuitem"
            onClick={createAndAdd}
            className="w-full text-left px-3 py-2 text-sm border-t border-[var(--border)] bg-transparent border-l-0 border-r-0 border-b-0 cursor-pointer text-[var(--accent)] hover:bg-[var(--surface)]"
          >
            {t("media.music.newPlaylist")}
          </button>
        </div>
      )}
    </div>
  );
}
