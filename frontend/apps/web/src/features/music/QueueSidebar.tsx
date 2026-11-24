import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast, useMusicPlayerStore } from "@evey/store";
import { Play, Save, Trash2 } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { useAddToPlaylist, useCreatePlaylist } from "@/features/playlist/api";
import { useDemoLock } from "@/hooks/useDemoMode";
import { QueueList } from "./QueueList";

// The queue beside the music page: a playlist of the moment, built by
// dragging tracks, albums and playlists onto it (or with their "+" button),
// played as it stands, and kept as a real playlist when it deserves it.
export function QueueSidebar({ className = "" }: { className?: string }) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const queue = useMusicPlayerStore((s) => s.queue);
  const playing = useMusicPlayerStore((s) => s.playing);
  const play = useMusicPlayerStore((s) => s.play);
  const clearQueue = useMusicPlayerStore((s) => s.clearQueue);
  const createPlaylist = useCreatePlaylist();
  const addToPlaylist = useAddToPlaylist();
  const [saving, setSaving] = useState(false);

  async function saveAsPlaylist() {
    const name = window.prompt(t("media.music.createPlaylistPrompt"));
    if (!name || !name.trim()) return;
    setSaving(true);
    try {
      const created = await createPlaylist.mutateAsync({ name: name.trim(), type: "manual" });
      // One call per track, in queue order: the server keeps the order they
      // arrive in.
      for (const track of queue) {
        await addToPlaylist.mutateAsync({ playlistId: created.id, mediaItemId: track.ID, mediaType: "music" });
      }
      toast.success(t("media.music.queueSaved", { name: name.trim() }));
    } catch {
      toast.error(t("media.playlist.createError"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <aside
      aria-label={t("media.music.queue")}
      className={`min-w-0 max-h-[calc(100vh-10rem)] flex flex-col gap-2 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface)] p-3 ${className}`}
    >
      <div className="flex items-center gap-2 min-w-0">
        <h2 className="flex-1 min-w-0 truncate text-sm font-semibold text-[var(--text)]">
          {t("media.music.queue")} <span className="text-[var(--text-muted)] font-normal">({queue.length})</span>
        </h2>
        {queue.length > (playing ? 1 : 0) && (
          <button
            type="button"
            onClick={clearQueue}
            aria-label={t("media.syncplay.clearQueue")}
            title={t("media.syncplay.clearQueue")}
            className="shrink-0 p-1 rounded flex items-center text-[var(--text-muted)] hover:text-[var(--color-danger)] bg-transparent border-none cursor-pointer"
          >
            <Trash2 size={ICON.sm} aria-hidden="true" />
          </button>
        )}
      </div>
      <QueueList zoneId="queue-sidebar" className="flex-1 min-h-24 overflow-y-auto -mx-1" />
      {queue.length > 0 && (
        <div className="flex flex-col gap-2">
          {!playing && (
            <button
              type="button"
              onClick={() => play(queue[0], queue, null)}
              className="min-w-0 inline-flex items-center justify-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-sm)] text-sm font-medium bg-[var(--accent)] text-[var(--filter-active-text)] border-none cursor-pointer"
            >
              <Play size={ICON.sm} aria-hidden="true" className="shrink-0" />
              <span className="truncate">{t("media.music.playQueue")}</span>
            </button>
          )}
          <button
            {...lock}
            type="button"
            onClick={() => void saveAsPlaylist()}
            disabled={!!lock.disabled || saving}
            className="min-w-0 inline-flex items-center justify-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-sm)] text-sm border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] cursor-pointer disabled:opacity-50 disabled:cursor-default"
          >
            <Save size={ICON.sm} aria-hidden="true" className="shrink-0" />
            <span className="truncate">{t("media.music.saveQueueAsPlaylist")}</span>
          </button>
        </div>
      )}
    </aside>
  );
}
