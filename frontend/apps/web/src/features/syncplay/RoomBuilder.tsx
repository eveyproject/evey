import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMusicServiceGetAlbums, useMusicServiceListTracks } from "@evey/api";
import {
  addToRoomQueue,
  removeFromRoomQueue,
  moveInRoomQueue,
  albumRoomQueueItems,
  playlistRoomQueueItems,
  type RoomQueueItem,
} from "@evey/client";
import { useAuthStore, toast } from "@evey/store";
import { useDebounce } from "@/hooks/useDebounce";
import { usePlaylistList } from "@/features/playlist/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { ICON } from "@/constants/iconSizes";
import { coverArtSrc } from "../../lib/coverArtSrc";
import {
  Music2,
  ListMusic,
  Disc3,
  Plus,
  X,
  ChevronUp,
  ChevronDown,
  Play,
  Trash2,
} from "lucide-react";

export type QueueItem = RoomQueueItem;

interface AlbumLite {
  id: string;
  title: string;
  artist: string;
  coverArtUrl: string;
  trackCount: number;
}

interface RoomBuilderProps {
  libraryId: string | undefined;
  creating: boolean;
  // Receives the full queue items (id + title + artist), not just ids, so the
  // caller can seed a metadata map and label the in-room "now playing" / queue
  // for tracks beyond the top-100 the page's own track list fetches.
  onCreate: (items: QueueItem[]) => void;
}

type Tab = "albums" | "tracks" | "playlists";


/**
 * RoomBuilder lets the host assemble a music room queue from a mix of whole
 * albums, individual tracks and playlists, then create the room. The backend
 * room model already supports a multi-item queue; this is the UI for it.
 */

export function RoomBuilder({ libraryId, creating, onCreate }: RoomBuilderProps) {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const tokenParam = accessToken ? `?token=${encodeURIComponent(accessToken)}` : "";

  const [tab, setTab] = useState<Tab>("albums");
  const [search, setSearch] = useState("");
  // Debounce before hitting the query: each keystroke would otherwise fire an
  // ILIKE search over title/artist/album on the (weak) server, like every other
  // search box in the app.
  const debouncedSearch = useDebounce(search, 300);
  const [queue, setQueue] = useState<QueueItem[]>([]);
  const [expanded, setExpanded] = useState<{ title: string; tracks: QueueItem[] } | null>(null);
  const [busy, setBusy] = useState(false);

  const { data: albums } = useMusicServiceGetAlbums(
    { libraryId },
    {
      query: {
        enabled: !!libraryId,
        select: (d): AlbumLite[] =>
          (d.albums ?? []).map((a) => ({
            id: a.id ?? "",
            title: a.title ?? "",
            artist: a.artist ?? "",
            coverArtUrl: a.coverArtUrl ?? "",
            trackCount: a.trackCount ?? 0,
          })),
        staleTime: 60_000,
      },
    },
  );

  const { data: tracks } = useMusicServiceListTracks(
    { libraryId, limit: 100, search: debouncedSearch || undefined },
    {
      query: {
        enabled: !!libraryId && tab === "tracks",
        select: (d): QueueItem[] =>
          (d.tracks ?? []).map((tr) => ({ id: tr.id ?? "", title: tr.title ?? "", artist: tr.artist ?? "" })),
        staleTime: 30_000,
      },
    },
  );

  const { data: playlists } = usePlaylistList();

  const queueIds = useMemo(() => new Set(queue.map((q) => q.id)), [queue]);

  const addItems = useCallback((items: QueueItem[]) => {
    setQueue((prev) => addToRoomQueue(prev, items));
  }, []);

  const removeItem = useCallback((id: string) => {
    setQueue((prev) => removeFromRoomQueue(prev, id));
  }, []);

  const move = useCallback((idx: number, dir: -1 | 1) => {
    setQueue((prev) => moveInRoomQueue(prev, idx, dir));
  }, []);

  const openAlbum = useCallback(
    async (album: AlbumLite) => {
      if (!libraryId || !album.id) return;
      setExpanded({ title: album.title, tracks: [] });
      setBusy(true);
      try {
        // Scope server-side by the album's opaque id (returns exactly that
        // album's tracks) instead of a fuzzy title search + client filter.
        setExpanded({ title: album.title, tracks: await albumRoomQueueItems(libraryId, album.id) });
      } catch {
        // Collapse rather than show a fake empty album: a failed load used to
        // render exactly like an album with zero tracks. Same error surface as
        // the rest of the feature (SyncPlayBar): a toast.
        setExpanded(null);
        toast.error(t("media.syncplay.albumLoadFailed"));
      } finally {
        setBusy(false);
      }
    },
    [libraryId, t],
  );

  const addAlbum = useCallback(
    async (album: AlbumLite) => {
      if (!libraryId || !album.id) return;
      setBusy(true);
      try {
        addItems(await albumRoomQueueItems(libraryId, album.id));
      } catch {
        // The queue stays unchanged - but the user asked for an action, so a
        // silent no-op reads as a broken button. Say it failed.
        toast.error(t("media.syncplay.addAlbumFailed"));
      } finally {
        setBusy(false);
      }
    },
    [libraryId, addItems, t],
  );

  const addPlaylist = useCallback(
    async (playlistId: string) => {
      setBusy(true);
      try {
        addItems(await playlistRoomQueueItems(playlistId));
      } catch {
        toast.error(t("media.syncplay.addPlaylistFailed"));
      } finally {
        setBusy(false);
      }
    },
    [addItems, t],
  );

  const tabs: { key: Tab; label: string; icon: typeof Disc3 }[] = [
    { key: "albums", label: t("media.syncplay.tabAlbums"), icon: Disc3 },
    { key: "tracks", label: t("media.syncplay.tabTracks"), icon: Music2 },
    { key: "playlists", label: t("media.syncplay.tabPlaylists"), icon: ListMusic },
  ];

  return (
    <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
      {/* ── Picker ─────────────────────────────────────────── */}
      <div className="flex flex-col gap-3 min-w-0">
        <div className="flex gap-1" role="tablist">
          {tabs.map(({ key, label, icon: Icon }) => (
            <button
              key={key}
              type="button"
              role="tab"
              aria-selected={tab === key}
              onClick={() => setTab(key)}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-[var(--radius-sm)] text-sm transition-colors-fast ${
                tab === key
                  ? "bg-[var(--accent)] text-[var(--filter-active-text)]"
                  : "bg-[var(--surface-raised)] text-[var(--text-muted)] hover:text-[var(--text)]"
              }`}
            >
              <Icon size={14} />
              {label}
            </button>
          ))}
        </div>

        {tab === "albums" && (
          <div className="flex flex-col gap-3">
            {expanded ? (
              <div className="flex flex-col gap-2">
                <div className="flex items-center justify-between gap-2">
                  <button
                    type="button"
                    onClick={() => setExpanded(null)}
                    className="text-sm text-[var(--accent)] hover:underline"
                  >
                    ← {t("media.syncplay.tabAlbums")}
                  </button>
                  <span className="font-medium text-sm truncate text-[var(--text)]">{expanded.title}</span>
                </div>
                <div className="max-h-72 overflow-y-auto flex flex-col gap-1 border border-[var(--border)] rounded-[var(--radius-sm)] p-1">
                  {expanded.tracks.map((tr) => (
                    <TrackRow key={tr.id} item={tr} inQueue={queueIds.has(tr.id)} onAdd={() => addItems([tr])} />
                  ))}
                  {expanded.tracks.length === 0 && (
                    <div className="text-xs text-[var(--text-muted)] p-2">{t("media.syncplay.noTracks")}</div>
                  )}
                </div>
              </div>
            ) : (
              <div className="grid grid-cols-[repeat(auto-fill,minmax(110px,1fr))] gap-2 max-h-80 overflow-y-auto pr-1">
                {(albums ?? []).map((a) => (
                  <div
                    key={a.id}
                    className="group relative rounded-[var(--radius-md)] overflow-hidden bg-[var(--surface-raised)] border border-[var(--border)] hover:border-[var(--accent)] transition-colors-fast"
                  >
                    <button
                      type="button"
                      onClick={() => void openAlbum(a)}
                      className="block w-full text-left cursor-pointer"
                    >
                      <div
                        className="w-full aspect-square bg-cover bg-center bg-[var(--surface)] flex items-center justify-center"
                        style={{ backgroundImage: a.coverArtUrl ? `url(${coverArtSrc(a.coverArtUrl, tokenParam, 320)})` : undefined }}
                      >
                        {!a.coverArtUrl && <Disc3 size={ICON["2xl"]} className="opacity-30 text-[var(--text-muted)]" />}
                      </div>
                      <div className="p-1.5">
                        <p className="font-medium text-xs truncate text-[var(--text)]">{a.title || "-"}</p>
                        <p className="text-xs truncate text-[var(--text-muted)]">{a.artist || "-"}</p>
                      </div>
                    </button>
                    <button
                      type="button"
                      onClick={(e) => { e.stopPropagation(); void addAlbum(a); }}
                      aria-label={t("media.syncplay.addAlbum")}
                      title={t("media.syncplay.addAlbum")}
                      className="absolute top-1.5 right-1.5 p-1 rounded-full bg-[var(--surface)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--accent)] cursor-pointer opacity-0 group-hover:opacity-100 transition-opacity-fast"
                    >
                      <Plus size={14} />
                    </button>
                  </div>
                ))}
                {(!albums || albums.length === 0) && (
                  <div className="text-xs text-[var(--text-muted)] p-2 col-span-full">{t("media.syncplay.noAlbums")}</div>
                )}
              </div>
            )}
          </div>
        )}

        {tab === "tracks" && (
          <div className="flex flex-col gap-2">
            <Input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder={t("common.search")}
              aria-label={t("common.search")}
            />
            <div className="max-h-72 overflow-y-auto flex flex-col gap-1 border border-[var(--border)] rounded-[var(--radius-sm)] p-1">
              {(tracks ?? []).map((tr) => (
                <TrackRow key={tr.id} item={tr} inQueue={queueIds.has(tr.id)} onAdd={() => addItems([tr])} />
              ))}
              {(!tracks || tracks.length === 0) && (
                <div className="text-xs text-[var(--text-muted)] p-2">{t("media.syncplay.noTracks")}</div>
              )}
            </div>
          </div>
        )}

        {tab === "playlists" && (
          <div className="max-h-80 overflow-y-auto flex flex-col gap-1 border border-[var(--border)] rounded-[var(--radius-sm)] p-1">
            {(playlists ?? []).map((pl) => (
              <div
                key={pl.id}
                className="flex items-center gap-2 px-2 py-1.5 rounded-[var(--radius-sm)] hover:bg-[var(--surface-raised)]"
              >
                <ListMusic size={14} className="text-[var(--text-muted)] flex-shrink-0" />
                <span className="flex-1 truncate text-sm text-[var(--text)]">{pl.name}</span>
                <button
                  type="button"
                  onClick={() => void addPlaylist(pl.id)}
                  aria-label={t("media.syncplay.addToQueue")}
                  title={t("media.syncplay.addToQueue")}
                  className="p-1 rounded-full text-[var(--text-muted)] hover:text-[var(--accent)] cursor-pointer"
                >
                  <Plus size={16} />
                </button>
              </div>
            ))}
            {(!playlists || playlists.length === 0) && (
              <div className="text-xs text-[var(--text-muted)] p-2">{t("media.syncplay.noPlaylists")}</div>
            )}
          </div>
        )}
      </div>

      {/* ── Queue basket ───────────────────────────────────── */}
      <div className="flex flex-col gap-2 min-w-0">
        <div className="flex items-center justify-between">
          <h3 className="text-sm font-semibold text-[var(--text)]">
            {t("media.syncplay.queue")} ({queue.length})
          </h3>
          {queue.length > 0 && (
            <button
              type="button"
              onClick={() => setQueue([])}
              className="flex items-center gap-1 text-xs text-[var(--text-muted)] hover:text-[var(--color-danger)] cursor-pointer"
            >
              <Trash2 size={12} />
              {t("media.syncplay.clearQueue")}
            </button>
          )}
        </div>

        {queue.length === 0 ? (
          <div className="flex-1 flex items-center justify-center text-center text-xs text-[var(--text-muted)] border border-dashed border-[var(--border)] rounded-[var(--radius-md)] p-6 min-h-40">
            {t("media.syncplay.queueEmpty")}
          </div>
        ) : (
          <div className="flex-1 max-h-80 overflow-y-auto flex flex-col gap-1 border border-[var(--border)] rounded-[var(--radius-sm)] p-1">
            {queue.map((q, idx) => (
              <div
                key={q.id}
                className="flex items-center gap-2 px-2 py-1 rounded-[var(--radius-sm)] hover:bg-[var(--surface-raised)]"
              >
                <span className="text-xs text-[var(--text-muted)] w-5 text-right flex-shrink-0">{idx + 1}</span>
                <div className="flex-1 min-w-0">
                  <p className="text-sm truncate text-[var(--text)]">{q.title || q.id}</p>
                  {q.artist && <p className="text-xs truncate text-[var(--text-muted)]">{q.artist}</p>}
                </div>
                <button
                  type="button"
                  onClick={() => move(idx, -1)}
                  disabled={idx === 0}
                  aria-label={t("media.syncplay.moveUp")}
                  title={t("media.syncplay.moveUp")}
                  className="p-0.5 text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-30 cursor-pointer disabled:cursor-default"
                >
                  <ChevronUp size={14} />
                </button>
                <button
                  type="button"
                  onClick={() => move(idx, 1)}
                  disabled={idx === queue.length - 1}
                  aria-label={t("media.syncplay.moveDown")}
                  title={t("media.syncplay.moveDown")}
                  className="p-0.5 text-[var(--text-muted)] hover:text-[var(--text)] disabled:opacity-30 cursor-pointer disabled:cursor-default"
                >
                  <ChevronDown size={14} />
                </button>
                <button
                  type="button"
                  onClick={() => removeItem(q.id)}
                  aria-label={t("media.syncplay.removeFromQueue")}
                  title={t("media.syncplay.removeFromQueue")}
                  className="p-0.5 text-[var(--text-muted)] hover:text-[var(--color-danger)] cursor-pointer"
                >
                  <X size={14} />
                </button>
              </div>
            ))}
          </div>
        )}

        <Button
          variant="primary"
          loading={creating}
          disabled={queue.length === 0 || busy}
          onClick={() => onCreate(queue)}
        >
          <Play size={16} className="mr-2" />
          {t("media.syncplay.createRoom")}
        </Button>
      </div>
    </div>
  );
}

function TrackRow({ item, inQueue, onAdd }: { item: QueueItem; inQueue: boolean; onAdd: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="flex items-center gap-2 px-2 py-1 rounded-[var(--radius-sm)] hover:bg-[var(--surface-raised)]">
      <Music2 size={14} className="text-[var(--text-muted)] flex-shrink-0" />
      <div className="flex-1 min-w-0">
        <p className="text-sm truncate text-[var(--text)]">{item.title || item.id}</p>
        {item.artist && <p className="text-xs truncate text-[var(--text-muted)]">{item.artist}</p>}
      </div>
      <button
        type="button"
        onClick={onAdd}
        disabled={inQueue}
        aria-label={t("media.syncplay.addToQueue")}
        title={t("media.syncplay.addToQueue")}
        className="p-1 rounded-full text-[var(--text-muted)] hover:text-[var(--accent)] disabled:opacity-30 cursor-pointer disabled:cursor-default flex-shrink-0"
      >
        <Plus size={16} />
      </button>
    </div>
  );
}
