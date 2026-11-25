import { useEffect, useMemo, useState } from "react";
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
import { ArrowLeft, Check, Download, GripVertical, Pencil, Play, Plus, Search, Shuffle, Trash2, Sparkles, X } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { startTokenDownload } from "@/utils/downloadToken";
import { useAddToPlaylist, useGeneratePlaylist, useReorderPlaylistItems, useUpdatePlaylist } from "@/features/playlist/api";
import { MusicPlaylistModal } from "./MusicPlaylistModal";
import { formatTime } from "@evey/api";
import { useDebounce } from "@/hooks/useDebounce";
import { useDraggable, useDropZone, moveTargetIndex } from "@/lib/dnd";
import { shuffleTracks } from "./musicFormat";
import { playlistItemsToTracks, useMusicTracks, type Track } from "./musicQueries";
import { TrackArt } from "./TrackArt";
import { AddToQueueButton } from "./AddToQueueButton";
import { addsTracks, isRowOf, resolveDragTracks, type MusicDragData } from "./musicDrag";
import { useDemoLock } from "@/hooks/useDemoMode";

type RawItem = { mediaItemId?: string; title?: string; artist?: string; album?: string; coverArtUrl?: string; duration?: number; mediaType?: string };

const playlistList = (id: string) => `playlist:${id}`;

/**
 * The full member order after the music rows take `musicOrder`: a playlist
 * can also hold videos, which keep their slots, and the server wants every
 * member in a reorder.
 */
export function mergeMusicOrder(allIds: string[], musicIds: Set<string>, musicOrder: string[]): string[] {
  const queue = [...musicOrder];
  return allIds.map((id) => (musicIds.has(id) ? (queue.shift() ?? id) : id));
}

/**
 * Adds what a drag names to a playlist, then moves the new tracks to `at` when
 * the drop gave a position. The server ignores a track already in it.
 */
function useAddDragToPlaylist() {
  const { t } = useTranslation();
  const add = useAddToPlaylist();
  const reorder = useReorderPlaylistItems();
  return async (playlistId: string, d: MusicDragData, at?: number | null, current?: { allIds: string[]; musicIds: string[] }) => {
    try {
      const tracks = await resolveDragTracks(d);
      const known = new Set(current?.allIds ?? []);
      const fresh = tracks.filter((tr) => tr.ID && !known.has(tr.ID));
      for (const tr of fresh) {
        await add.mutateAsync({ playlistId, mediaItemId: tr.ID, mediaType: "music" });
      }
      if (fresh.length === 0) {
        toast.info(t("media.music.alreadyInPlaylist"));
        return;
      }
      toast.success(t("media.music.addedToPlaylistCount", { count: fresh.length }));
      if (current && at !== undefined && at !== null && at < current.musicIds.length) {
        const freshIds = fresh.map((tr) => tr.ID);
        const musicOrder = [...current.musicIds.slice(0, at), ...freshIds, ...current.musicIds.slice(at)];
        const allIds = mergeMusicOrder([...current.allIds, ...freshIds], new Set(musicOrder), musicOrder);
        await reorder.mutateAsync({ playlistId, items: allIds.map((mediaItemId, sortOrder) => ({ mediaItemId, sortOrder })) });
      }
    } catch (err) {
      console.warn("add to playlist failed", err);
      toast.error(t("media.playlist.addError"));
    }
  };
}

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
  const [selected, setSelected] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const generate = useGeneratePlaylist();
  const addDrag = useAddDragToPlaylist();

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
  const deletePlaylist = usePlaylistServiceDeletePlaylist();

  const playlists = (playlistsData?.playlists ?? []).filter(
    (p) => p.type !== V1PlaylistType.PLAYLIST_TYPE_SMART,
  );
  const selectedPlaylist = playlists.find((p) => p.id === selected);

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
      <PlaylistDetail
        id={selected}
        name={selectedPlaylist?.name ?? ""}
        allTracks={allTracks}
        libraryID={libraryID}
        onPlay={onPlay}
        onBack={() => setSelected(null)}
      />
    );
  }

  if (listLoading) {
    return <p className="text-[var(--text-muted)] text-sm">{t("media.music.loading")}</p>;
  }
  return (
    <div className="space-y-2">
      <div className="flex justify-end gap-2 flex-wrap">
        <button
          {...lock}
          type="button"
          onClick={() => setCreating(true)}
          disabled={!!lock.disabled}
          data-testid="new-playlist-btn"
          className="inline-flex items-center gap-2 px-4 py-1.5 rounded-[var(--radius-sm)] text-sm font-medium border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] cursor-pointer disabled:opacity-50 disabled:cursor-default"
        >
          <Plus size={ICON.md} aria-hidden="true" />
          {t("media.playlist.createPlaylist")}
        </button>
        {libraryID && (
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
        )}
      </div>
      {playlists.length === 0 && (
        <p className="text-[var(--text-muted)] text-sm">{t("media.music.noPlaylists")}</p>
      )}
      {playlists.map((p) => (
        <PlaylistRow
          key={p.id}
          id={p.id ?? ""}
          name={p.name ?? ""}
          description={p.description}
          onOpen={() => p.id && setSelected(p.id)}
          onDelete={() => p.id && confirm(t("media.playlist.deleteConfirm", { name: p.name ?? "" })) && handleDeletePlaylist(p.id)}
          onDropTracks={(d) => p.id && void addDrag(p.id, d)}
        />
      ))}
      <MusicPlaylistModal open={creating} onClose={() => setCreating(false)} libraryID={libraryID} onCreated={(id) => setSelected(id)} />
    </div>
  );
}

function isMusicPlaylist(d: MusicDragData): d is Extract<MusicDragData, { kind: "music-playlist" }> {
  return d.kind === "music-playlist";
}

// A playlist in the list: opens on click, drags onto the queue whole, and
// takes tracks dropped on it.
function PlaylistRow({
  id,
  name,
  description,
  onOpen,
  onDelete,
  onDropTracks,
}: {
  id: string;
  name: string;
  description?: string;
  onOpen: () => void;
  onDelete: () => void;
  onDropTracks: (d: MusicDragData) => void;
}) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const drag = useDraggable(`playlist:${id}`, (): MusicDragData => ({ kind: "music-playlist", id, name }), name || "-");
  const zone = useDropZone<MusicDragData>(
    `playlist-row:${id}`,
    (d): d is MusicDragData => !lock.disabled && addsTracks(playlistList(id))(d) && !(isMusicPlaylist(d) && d.id === id),
    (d) => onDropTracks(d),
  );
  return (
    <div
      ref={zone.ref}
      {...zone.props}
      {...drag.props}
      className={`flex items-center gap-3 px-4 py-3 rounded-[var(--radius-md)] bg-[var(--surface-raised)] hover:bg-[var(--surface)] ${
        zone.over ? "outline outline-2 outline-[var(--accent)]" : ""
      } ${drag.dragging ? "opacity-40" : ""}`}
    >
      <button
        type="button"
        onClick={onOpen}
        className="flex-1 text-left bg-transparent border-none cursor-pointer p-0"
      >
        <p className="font-medium text-[var(--text)]">{name || "-"}</p>
        {description && (
          <p className="text-xs text-[var(--text-muted)] truncate">{description}</p>
        )}
      </button>
      <AddToQueueButton data={{ kind: "music-playlist", id, name }} />
      <button
        {...lock}
        type="button"
        data-no-drag
        onClick={onDelete}
        aria-label={t("common.delete")}
        className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--color-danger)] p-1 rounded"
      >
        <Trash2 size={ICON.md} />
      </button>
    </div>
  );
}

function PlaylistDetail({
  id,
  name,
  allTracks,
  libraryID,
  onPlay,
  onBack,
}: {
  id: string;
  name: string;
  allTracks: Track[];
  libraryID?: string;
  onPlay: (tr: Track, all: Track[], source?: MusicSource) => void;
  onBack: () => void;
}) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const accessToken = useAuthStore((s) => s.accessToken);
  const [picking, setPicking] = useState(false);
  const list = playlistList(id);
  const { data: itemsData, isLoading: itemsLoading } = usePlaylistServiceGetPlaylistItems(
    id,
    undefined,
    { query: { enabled: !!id, staleTime: 15_000 } },
  );
  const removeItem = usePlaylistServiceRemovePlaylistItem();
  const reorder = useReorderPlaylistItems();
  const addDrag = useAddDragToPlaylist();
  const source: MusicSource = { type: "playlist", id, label: name || id };

  const tracksByID = useMemo(() => {
    const m = new Map<string, Track>();
    allTracks.forEach((tr) => m.set(tr.ID, tr));
    return m;
  }, [allTracks]);

  const rawItems: RawItem[] = useMemo(() => itemsData?.items ?? [], [itemsData]);
  // A reorder shows at once; the refetched members replace it.
  const [optimistic, setOptimistic] = useState<string[] | null>(null);
  useEffect(() => setOptimistic(null), [itemsData]);
  // Every music member, from what the server sends with it: the track list of
  // this screen holds only the pages loaded so far, and a playlist resolved
  // against it lost every track beyond them. A loaded track is preferred for
  // its fuller fields; a member the server could not name (its track gone)
  // is left out.
  const playlistTracks: Track[] = useMemo(() => {
    const members = playlistItemsToTracks(rawItems)
      .map((tr) => tracksByID.get(tr.ID) ?? tr)
      .filter((tr) => tracksByID.has(tr.ID) || !!tr.Title);
    if (!optimistic) return members;
    const byID = new Map(members.map((tr) => [tr.ID, tr]));
    return optimistic.map((tid) => byID.get(tid)).filter((tr): tr is Track => !!tr);
  }, [rawItems, tracksByID, optimistic]);
  const allIds = rawItems.map((it) => it.mediaItemId ?? "").filter(Boolean);
  const musicIds = playlistTracks.map((tr) => tr.ID);

  async function handleRemove(trackID: string) {
    try {
      await removeItem.mutateAsync({ id, mediaItemId: trackID });
      await queryClient.invalidateQueries({
        queryKey: getPlaylistServiceGetPlaylistItemsQueryKey(id),
      });
    } catch (err) {
      console.warn("remove playlist item failed", err);
      toast.error(t("media.playlist.failedRemove"));
    }
  }

  async function move(from: number, to: number) {
    if (from === to || to < 0 || to >= musicIds.length) return;
    const order = [...musicIds];
    const [moved] = order.splice(from, 1);
    order.splice(to, 0, moved);
    const full = mergeMusicOrder(allIds, new Set(musicIds), order);
    setOptimistic(order);
    try {
      await reorder.mutateAsync({ playlistId: id, items: full.map((mediaItemId, sortOrder) => ({ mediaItemId, sortOrder })) });
    } catch (err) {
      setOptimistic(null);
      console.warn("reorder playlist failed", err);
      toast.error(t("media.music.reorderFailed"));
    }
  }

  const zone = useDropZone<MusicDragData>(
    `playlist-detail:${id}`,
    (d): d is MusicDragData =>
      !lock.disabled && (isRowOf(list)(d) || (addsTracks(list)(d) && !(isMusicPlaylist(d) && d.id === id))),
    (d, index) => {
      if (isRowOf(list)(d)) {
        void move(d.index, moveTargetIndex(d.index, index ?? musicIds.length));
        return;
      }
      void addDrag(id, d, index, { allIds, musicIds });
    },
  );

  return (
    <div>
      <div className="flex items-center gap-3 mb-4 flex-wrap">
        <button
          type="button"
          onClick={onBack}
          className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--accent)] p-1 rounded"
          aria-label={t("common.back")}
        >
          <ArrowLeft size={ICON.lg} />
        </button>
        <PlaylistTitle id={id} name={name} canEdit={!lock.disabled} />
        <div className="ml-auto flex items-center gap-2 flex-wrap">
          {playlistTracks.length > 0 && (
            <>
              <button
                type="button"
                onClick={() => onPlay(playlistTracks[0], playlistTracks, source)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-[var(--radius-sm)] bg-[var(--accent)] text-[var(--filter-active-text)] border-none cursor-pointer"
              >
                <Play size={ICON.sm} />
                {t("media.detail.play")}
              </button>
              <button
                type="button"
                onClick={() => { const shuffled = shuffleTracks(playlistTracks); onPlay(shuffled[0], shuffled, source); }}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)] text-[var(--text)] cursor-pointer"
              >
                <Shuffle size={ICON.sm} />
                {t("media.music.shuffle")}
              </button>
              <AddToQueueButton data={{ kind: "music-tracks", tracks: playlistTracks }} />
            </>
          )}
          {playlistTracks.length > 0 && accessToken && (
            <a
              href="#"
              onClick={async (e) => {
                e.preventDefault();
                // Click-time mint - the JWT must not ride in the URL.
                try {
                  await startTokenDownload(`/api/v1/playlists/${id}/download`, id, `${name || "playlist"}.zip`);
                } catch {
                  toast.error(t("common.error"));
                }
              }}
              data-testid="playlist-download-btn"
              className="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-[var(--radius-sm)] border border-[var(--border)] text-[var(--text-muted)] hover:bg-[var(--surface-raised)] transition-colors-fast"
              title={t("media.playlist.download")}
            >
              <Download size={ICON.sm} />
              {t("media.playlist.download")}
            </a>
          )}
          {libraryID && !lock.disabled && (
            <button
              type="button"
              onClick={() => setPicking((v) => !v)}
              aria-expanded={picking}
              data-testid="playlist-add-tracks-btn"
              className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-[var(--radius-sm)] border border-[var(--border)] cursor-pointer ${
                picking ? "bg-[var(--accent)] text-[var(--filter-active-text)]" : "bg-[var(--surface-raised)] text-[var(--text)]"
              }`}
            >
              <Plus size={ICON.sm} />
              {t("media.music.addTracks")}
            </button>
          )}
        </div>
      </div>

      <div className={picking ? "grid gap-4 md:grid-cols-2" : ""}>
        <div
          ref={zone.ref}
          {...zone.props}
          className={`rounded-[var(--radius-sm)] min-h-24 ${
            zone.over ? "outline outline-2 outline-[var(--accent)]" : zone.armed ? "outline outline-1 outline-dashed outline-[var(--accent)]" : ""
          }`}
        >
          {itemsLoading ? (
            <p className="text-[var(--text-muted)] text-sm">{t("media.music.loading")}</p>
          ) : playlistTracks.length === 0 ? (
            <p className="text-[var(--text-muted)] text-sm p-4 text-center">{t("media.music.emptyPlaylistDropHint")}</p>
          ) : (
            <div className="space-y-1">
              {playlistTracks.map((tr, i) => (
                <PlaylistTrackRow
                  key={tr.ID}
                  list={list}
                  track={tr}
                  index={i}
                  count={playlistTracks.length}
                  dropBefore={zone.index === i}
                  dropAfter={zone.index === playlistTracks.length && i === playlistTracks.length - 1}
                  canEdit={!lock.disabled}
                  onPlay={() => onPlay(tr, playlistTracks, source)}
                  onRemove={() => void handleRemove(tr.ID)}
                  onMove={(to) => void move(i, to)}
                />
              ))}
            </div>
          )}
        </div>
        {picking && libraryID && (
          <TrackPicker
            libraryID={libraryID}
            inPlaylist={new Set(musicIds)}
            onAdd={(tr) => void addDrag(id, { kind: "music-tracks", tracks: [tr] }, null, { allIds, musicIds })}
          />
        )}
      </div>
    </div>
  );
}

function PlaylistTrackRow({
  list,
  track: tr,
  index,
  count,
  dropBefore,
  dropAfter,
  canEdit,
  onPlay,
  onRemove,
  onMove,
}: {
  list: string;
  track: Track;
  index: number;
  count: number;
  dropBefore: boolean;
  dropAfter: boolean;
  canEdit: boolean;
  onPlay: () => void;
  onRemove: () => void;
  onMove: (to: number) => void;
}) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const drag = useDraggable(
    `${list}:${tr.ID}`,
    (): MusicDragData => ({ kind: "music-row", list, index, track: tr }),
    tr.Title || "-",
  );
  return (
    <div
      data-drop-index={index}
      {...drag.props}
      className={`flex items-center gap-2 p-2 rounded-[var(--radius-sm)] hover:bg-[var(--surface)] ${drag.dragging ? "opacity-40" : ""} ${
        dropBefore ? "shadow-[inset_0_2px_0_var(--accent)]" : ""
      } ${dropAfter ? "shadow-[inset_0_-2px_0_var(--accent)]" : ""}`}
    >
      {canEdit && (
        <button
          type="button"
          aria-label={t("media.music.moveInQueue", { title: tr.Title || "-" })}
          title={t("media.music.moveInQueueHint")}
          onKeyDown={(e) => {
            if (e.key === "ArrowUp" && index > 0) { e.preventDefault(); onMove(index - 1); }
            if (e.key === "ArrowDown" && index < count - 1) { e.preventDefault(); onMove(index + 1); }
          }}
          className="bg-transparent border-none cursor-grab text-[var(--text-muted)] hover:text-[var(--text)] p-1 flex items-center shrink-0"
        >
          <GripVertical size={ICON.sm} aria-hidden="true" />
        </button>
      )}
      <button
        type="button"
        onClick={onPlay}
        className="flex items-center gap-3 flex-1 min-w-0 bg-transparent border-none cursor-pointer text-left p-0"
      >
        <TrackArt trackID={tr.ID} coverArtURL={tr.CoverArtURL} />
        <div className="flex-1 min-w-0">
          <p className="text-sm text-[var(--text)] truncate">{tr.Title}</p>
          <p className="text-xs text-[var(--text-muted)] truncate">{tr.Artist}</p>
        </div>
        <span className="text-xs text-[var(--text-muted)]">{formatTime(tr.Duration)}</span>
      </button>
      <AddToQueueButton data={{ kind: "music-tracks", tracks: [tr] }} />
      <button
        {...lock}
        type="button"
        data-no-drag
        onClick={onRemove}
        aria-label={t("common.delete")}
        className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--color-danger)] p-1 rounded"
      >
        <Trash2 size={ICON.md} />
      </button>
    </div>
  );
}

// A search over the library beside an open playlist: each result adds with
// its "+" or drags onto the playlist at the spot it is dropped.
function TrackPicker({ libraryID, inPlaylist, onAdd }: { libraryID: string; inPlaylist: Set<string>; onAdd: (tr: Track) => void }) {
  const { t } = useTranslation();
  const [search, setSearch] = useState("");
  const debounced = useDebounce(search, 300);
  const { data: results, isLoading } = useMusicTracks(libraryID, debounced);
  return (
    <div className="flex flex-col gap-2 rounded-[var(--radius-md)] border border-[var(--border)] bg-[var(--surface)] p-3" data-testid="playlist-track-picker">
      <label className="flex items-center gap-2 px-3 py-2 rounded-[var(--radius-sm)] border border-[var(--border)] bg-[var(--surface-raised)]">
        <Search size={ICON.sm} className="text-[var(--text-muted)]" aria-hidden="true" />
        <input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t("media.music.searchPlaceholder")}
          aria-label={t("common.search")}
          className="flex-1 bg-transparent border-none outline-none text-sm text-[var(--text)]"
        />
      </label>
      <div className="max-h-[50vh] overflow-y-auto flex flex-col gap-0.5">
        {isLoading && <p className="text-xs text-[var(--text-muted)] p-2">{t("media.music.loading")}</p>}
        {!isLoading && results.length === 0 && <p className="text-xs text-[var(--text-muted)] p-2">{t("media.music.noTracksSearch")}</p>}
        {results.slice(0, 100).map((tr) => (
          <PickerRow key={tr.ID} track={tr} added={inPlaylist.has(tr.ID)} onAdd={() => onAdd(tr)} />
        ))}
      </div>
    </div>
  );
}

function PickerRow({ track: tr, added, onAdd }: { track: Track; added: boolean; onAdd: () => void }) {
  const { t } = useTranslation();
  const drag = useDraggable(`pick:${tr.ID}`, (): MusicDragData => ({ kind: "music-tracks", tracks: [tr] }), tr.Title || "-");
  return (
    <div {...drag.props} className={`flex items-center gap-2 px-2 py-1 rounded-[var(--radius-sm)] hover:bg-[var(--surface-raised)] ${drag.dragging ? "opacity-40" : ""}`}>
      <div className="flex-1 min-w-0">
        <p className="text-sm truncate text-[var(--text)]">{tr.Title || "-"}</p>
        <p className="text-xs truncate text-[var(--text-muted)]">{tr.Artist || "-"}</p>
      </div>
      <button
        type="button"
        data-no-drag
        onClick={onAdd}
        disabled={added}
        aria-label={t("media.music.addToPlaylist")}
        title={t("media.music.addToPlaylist")}
        className="p-1 rounded-full bg-transparent border-none text-[var(--text-muted)] hover:text-[var(--accent)] disabled:opacity-30 cursor-pointer disabled:cursor-default"
      >
        <Plus size={ICON.md} />
      </button>
    </div>
  );
}

// The open playlist's name, renamed in place: the pencil (or a click on the
// name) turns it into a field, Enter keeps, Escape gives up.
function PlaylistTitle({ id, name, canEdit }: { id: string; name: string; canEdit: boolean }) {
  const { t } = useTranslation();
  const update = useUpdatePlaylist({ ownErrorToast: true });
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(name);
  // The name shown while the list refetches after a rename.
  const [saved, setSaved] = useState<string | null>(null);
  useEffect(() => setSaved(null), [name]);
  const current = saved ?? name;

  function start() {
    setDraft(current);
    setEditing(true);
  }

  async function commit() {
    const next = draft.trim();
    if (!next || next === current) {
      setEditing(false);
      return;
    }
    try {
      await update.mutateAsync({ id, name: next });
      setSaved(next);
      setEditing(false);
      toast.success(t("media.music.playlistRenamed"));
    } catch (err) {
      const status = (err as { httpStatus?: number })?.httpStatus;
      toast.error(status === 409 ? t("media.music.mixNameTaken") : t("media.music.renameFailed"));
    }
  }

  if (editing) {
    return (
      <form
        className="flex items-center gap-1 min-w-0"
        onSubmit={(e) => { e.preventDefault(); void commit(); }}
      >
        <input
          autoFocus
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
          onKeyDown={(e) => { if (e.key === "Escape") { e.preventDefault(); setEditing(false); } }}
          aria-label={t("media.playlist.name")}
          maxLength={200}
          className="min-w-0 w-64 max-w-full px-2 py-1 text-lg font-semibold rounded-[var(--radius-sm)] border border-[var(--accent)] bg-[var(--surface-raised)] text-[var(--text)]"
        />
        <button type="submit" disabled={update.isPending} aria-label={t("common.save")} title={t("common.save")} className="p-1 rounded bg-transparent border-none cursor-pointer text-[var(--accent)]">
          <Check size={ICON.md} />
        </button>
        <button type="button" onClick={() => setEditing(false)} aria-label={t("common.cancel")} title={t("common.cancel")} className="p-1 rounded bg-transparent border-none cursor-pointer text-[var(--text-muted)]">
          <X size={ICON.md} />
        </button>
      </form>
    );
  }

  return (
    <div className="flex items-center gap-1 min-w-0">
      <h2
        className={`text-lg font-semibold text-[var(--text)] truncate ${canEdit ? "cursor-text" : ""}`}
        onClick={canEdit ? start : undefined}
      >
        {current}
      </h2>
      {canEdit && (
        <button
          type="button"
          onClick={start}
          aria-label={t("media.music.renamePlaylist")}
          title={t("media.music.renamePlaylist")}
          data-testid="playlist-rename-btn"
          className="p-1 rounded bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--accent)]"
        >
          <Pencil size={ICON.sm} />
        </button>
      )}
    </div>
  );
}
