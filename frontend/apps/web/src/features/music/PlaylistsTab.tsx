import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import {
  usePlaylistServiceListPlaylists,
  usePlaylistServiceGetPlaylistItems,
  usePlaylistServiceRemovePlaylistItem,
  usePlaylistServiceDeletePlaylist,
  getPlaylistServiceListPlaylistsQueryKey,
  getPlaylistServiceGetPlaylistItemsQueryKey,
  V1PlaylistType,
} from "@evey/api";
import { useAuthStore, toast, type MusicSource } from "@evey/store";
import { ArrowLeft, Download, Shuffle, Trash2, Sparkles } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { startTokenDownload } from "@/utils/downloadToken";
import { useGeneratePlaylist } from "@/features/playlist/api";
import { formatTime } from "@evey/api";
import { shuffleTracks } from "./musicFormat";
import type { Track } from "./musicQueries";
import { TrackArt } from "./TrackArt";
import { useDemoLock } from "@/hooks/useDemoMode";

export function PlaylistsTab({
  allTracks,
  onPlay,
  libraryID,
}: {
  allTracks: Track[];
  onPlay: (tr: Track, all: Track[], source?: MusicSource) => void;
  libraryID?: string;
}) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const accessToken = useAuthStore((s) => s.accessToken);
  const [selected, setSelected] = useState<string | null>(null);
  const generate = useGeneratePlaylist();

  function handleGenerate() {
    if (!libraryID || generate.isPending) return;
    generate.mutate(
      { libraryId: libraryID, name: t("media.playlist.generatedName") },
      {
        onSuccess: (res) => {
          toast.success(t("media.playlist.generated"));
          if (res?.id) setSelected(res.id);
        },
        onError: () => toast.error(t("media.playlist.generateFailed")),
      },
    );
  }
  const { data: playlistsData, isLoading: listLoading } = usePlaylistServiceListPlaylists();
  const { data: itemsData, isLoading: itemsLoading } = usePlaylistServiceGetPlaylistItems(
    selected ?? "",
    undefined,
    { query: { enabled: !!selected, staleTime: 15_000 } },
  );
  const removeItem = usePlaylistServiceRemovePlaylistItem();
  const deletePlaylist = usePlaylistServiceDeletePlaylist();

  const playlists = (playlistsData?.playlists ?? []).filter(
    (p) => p.type !== V1PlaylistType.PLAYLIST_TYPE_SMART,
  );
  const selectedPlaylist = playlists.find((p) => p.id === selected);
  const playlistSource: MusicSource | undefined = selectedPlaylist?.id
    ? { type: "playlist", id: selectedPlaylist.id, label: selectedPlaylist.name ?? selectedPlaylist.id }
    : undefined;

  const tracksByID = useMemo(() => {
    const m = new Map<string, Track>();
    allTracks.forEach((tr) => m.set(tr.ID, tr));
    return m;
  }, [allTracks]);

  const playlistTracks: Track[] = (itemsData?.items ?? [])
    .map((it) => tracksByID.get(it.mediaItemId ?? ""))
    .filter((x): x is Track => !!x);

  async function handleRemove(trackID: string) {
    if (!selected) return;
    try {
      await removeItem.mutateAsync({ id: selected, mediaItemId: trackID });
      await queryClient.invalidateQueries({
        queryKey: getPlaylistServiceGetPlaylistItemsQueryKey(selected),
      });
    } catch (err) {
      console.warn("remove playlist item failed", err);
      toast.error(t("media.playlist.failedRemove"));
    }
  }

  async function handleDeletePlaylist(pid: string) {
    try {
      await deletePlaylist.mutateAsync({ id: pid });
      await queryClient.invalidateQueries({ queryKey: getPlaylistServiceListPlaylistsQueryKey() });
      setSelected(null);
    } catch (err) {
      console.warn("delete playlist failed", err);
      toast.error(t("media.playlist.deleteError"));
    }
  }

  if (selected) {
    return (
      <div>
        <div className="flex items-center gap-3 mb-4">
          <button
            type="button"
            onClick={() => setSelected(null)}
            className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--accent)] p-1 rounded"
            aria-label={t("common.back")}
          >
            <ArrowLeft size={ICON.lg} />
          </button>
          <h2 className="text-lg font-semibold text-[var(--text)]">{selectedPlaylist?.name ?? ""}</h2>
          {playlistTracks.length > 0 && (
            <button
              type="button"
              onClick={() => { const shuffled = shuffleTracks(playlistTracks); onPlay(shuffled[0], shuffled, playlistSource); }}
              className="ml-auto flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-[var(--radius-sm)] bg-[var(--accent)] text-[var(--filter-active-text)] border-none cursor-pointer"
            >
              <Shuffle size={ICON.sm} />
              {t("media.music.shuffle")}
            </button>
          )}
          {selected && playlistTracks.length > 0 && accessToken && (
            <a
              href="#"
              onClick={async (e) => {
                e.preventDefault();
                // Click-time mint - the JWT must not ride in the URL.
                try {
                  await startTokenDownload(`/api/v1/playlists/${selected}/download`, selected, `${selectedPlaylist?.name ?? "playlist"}.zip`);
                } catch {
                  toast.error(t("common.error"));
                }
              }}
              data-testid="playlist-download-btn"
              className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-[var(--radius-sm)] border border-[var(--border)] text-[var(--text-muted)] hover:bg-[var(--surface-raised)] transition-colors-fast ${playlistTracks.length > 0 ? "" : "ml-auto"}`}
              title={t("media.playlist.download")}
            >
              <Download size={ICON.sm} />
              {t("media.playlist.download")}
            </a>
          )}
        </div>
        {itemsLoading ? (
          <p className="text-[var(--text-muted)] text-sm">{t("media.music.loading")}</p>
        ) : playlistTracks.length === 0 ? (
          <p className="text-[var(--text-muted)] text-sm">{t("media.music.emptyPlaylist")}</p>
        ) : (
          <div className="space-y-1">
            {playlistTracks.map((tr, i) => (
              <div
                key={`${tr.ID}-${i}`}
                className="flex items-center gap-3 p-2 rounded-[var(--radius-sm)] hover:bg-[var(--surface)]"
              >
                <button
                  type="button"
                  onClick={() => onPlay(tr, playlistTracks, playlistSource)}
                  className="flex items-center gap-3 flex-1 min-w-0 bg-transparent border-none cursor-pointer text-left p-0"
                >
                  <TrackArt trackID={tr.ID} coverArtURL={tr.CoverArtURL} />
                  <div className="flex-1 min-w-0">
                    <p className="text-sm text-[var(--text)] truncate">{tr.Title}</p>
                    <p className="text-xs text-[var(--text-muted)] truncate">{tr.Artist}</p>
                  </div>
                  <span className="text-xs text-[var(--text-muted)]">{formatTime(tr.Duration)}</span>
                </button>
                <button
                  {...lock}
                  type="button"
                  onClick={() => handleRemove(tr.ID)}
                  aria-label={t("common.delete")}
                  className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--color-danger)] p-1 rounded"
                >
                  <Trash2 size={ICON.md} />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    );
  }

  if (listLoading) {
    return <p className="text-[var(--text-muted)] text-sm">{t("media.music.loading")}</p>;
  }
  return (
    <div className="space-y-2">
      {libraryID && (
        <div className="flex justify-end">
          <button
            {...lock}
            type="button"
            onClick={handleGenerate}
            disabled={!!lock.disabled || (generate.isPending)}
            data-testid="generate-playlist-btn"
            className="inline-flex items-center gap-2 px-4 py-1.5 rounded-[var(--radius-sm)] text-sm font-medium bg-[var(--accent)] text-[var(--filter-active-text)] border-none cursor-pointer disabled:opacity-50 disabled:cursor-default"
          >
            <Sparkles size={ICON.md} aria-hidden="true" />
            {generate.isPending ? t("media.playlist.generating") : t("media.playlist.generate")}
          </button>
        </div>
      )}
      {playlists.length === 0 && (
        <p className="text-[var(--text-muted)] text-sm">{t("media.music.noPlaylists")}</p>
      )}
      {playlists.map((p) => (
        <div
          key={p.id}
          className="flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] bg-[var(--surface-raised)] hover:bg-[var(--surface)]"
        >
          <button
            type="button"
            onClick={() => p.id && setSelected(p.id)}
            className="flex-1 text-left bg-transparent border-none cursor-pointer p-0"
          >
            <p className="font-medium text-[var(--text)]">{p.name || "-"}</p>
            {p.description && (
              <p className="text-xs text-[var(--text-muted)] truncate">{p.description}</p>
            )}
          </button>
          <button
            {...lock}
            type="button"
            onClick={() => p.id && confirm(t("media.playlist.deleteConfirm", { name: p.name ?? "" })) && handleDeletePlaylist(p.id)}
            aria-label={t("common.delete")}
            className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--color-danger)] p-1 rounded"
          >
            <Trash2 size={ICON.md} />
          </button>
        </div>
      ))}
    </div>
  );
}
