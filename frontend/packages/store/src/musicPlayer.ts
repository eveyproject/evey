import { create } from "zustand";
import { nextQueueIndex, previousQueueIndex, type MusicSource } from "./musicQueue";

export interface MusicTrack {
  ID: string;
  Title: string;
  Artist: string;
  Album: string;
  AlbumArtist: string;
  Year: number;
  TrackNumber: number;
  DiskNumber: number;
  Duration: number;
  FilePath: string;
  MimeType: string;
  Genre: string;
  CoverArtURL: string;
}

// Where the playback was launched from, so the mini player can link back to
// it. One declaration for every client, next to the queue.
export type { MusicSource } from "./musicQueue";

const SHUFFLE_KEY = "evey-music-shuffle";

function loadShuffle(): boolean {
  try { return localStorage.getItem(SHUFFLE_KEY) === "1"; } catch { return false; }
}
function saveShuffle(s: boolean) {
  try { localStorage.setItem(SHUFFLE_KEY, s ? "1" : "0"); } catch { /* quota */ }
}

interface MusicPlayerState {
  playing: MusicTrack | null;
  queue: MusicTrack[];
  shuffle: boolean;
  // externally-forced pause - e.g. a trailer modal is open.
  // The <audio> element in GlobalAudioPlayer reacts to this flag.
  externallyPaused: boolean;
  // Where the current playback was started from (album/playlist/artist), or
  // null when launched from an ad-hoc context (search, single track).
  source: MusicSource | null;

  play: (track: MusicTrack, queue?: MusicTrack[], source?: MusicSource | null) => void;
  setQueue: (queue: MusicTrack[]) => void;
  // The queue edited by hand, a playlist of the moment: tracks added at the
  // end or at a given position, moved, removed. Returns how many were added
  // (a track already queued is not queued twice: next/prev find the playing
  // track by id).
  addToQueue: (tracks: MusicTrack[], at?: number) => number;
  moveInQueue: (from: number, to: number) => void;
  removeFromQueue: (id: string) => void;
  // Empties the queue but the track that is playing, which keeps playing.
  clearQueue: () => void;
  toggleShuffle: () => void;
  pauseExternal: () => void;
  resumeExternal: () => void;
  next: () => void;
  prev: () => void;
  close: () => void;
}

export const useMusicPlayerStore = create<MusicPlayerState>()((set, get) => ({
  playing: null,
  queue: [],
  shuffle: typeof window !== "undefined" ? loadShuffle() : false,
  externallyPaused: false,
  source: null,

  play: (track, queue, source) => set((s) => ({
    playing: track,
    queue: queue ?? s.queue,
    externallyPaused: false,
    // A fresh play() call replaces the source; omitting it (e.g. ad-hoc track)
    // clears it so the mini-player won't link to an unrelated album.
    source: source ?? null,
  })),

  setQueue: (queue) => set({ queue }),

  addToQueue: (tracks, at) => {
    const { queue } = get();
    const next = insertTracks(queue, tracks, at);
    const added = next.length - queue.length;
    if (added > 0) set({ queue: next });
    return added;
  },

  moveInQueue: (from, to) => set((s) => ({ queue: moveTrack(s.queue, from, to) })),

  // Removing the track that is playing moves on to its former neighbour
  // rather than leaving playback on a track the queue no longer holds.
  removeFromQueue: (id) => {
    const { queue, playing } = get();
    const idx = queue.findIndex((t) => t.ID === id);
    if (idx < 0) return;
    const next = queue.filter((t) => t.ID !== id);
    if (playing?.ID !== id) {
      set({ queue: next });
      return;
    }
    const neighbour = next[idx] ?? next[idx - 1];
    if (neighbour) set({ queue: next, playing: neighbour, externallyPaused: false });
    else set({ playing: null, queue: [], externallyPaused: false, source: null });
  },

  clearQueue: () => set((s) => ({ queue: s.playing ? [s.playing] : [] })),

  toggleShuffle: () => set((s) => {
    const next = !s.shuffle;
    saveShuffle(next);
    return { shuffle: next };
  }),

  pauseExternal: () => set({ externallyPaused: true }),
  resumeExternal: () => set({ externallyPaused: false }),

  // next / prev answer to the SAME rules as the two televisions and the
  // phone (@evey/store's musicQueue): shuffle never repeats the track that is
  // playing, the head holds on previous, and the end of the queue stops
  // rather than looping. This store simply keeps a track rather than an index.
  next: () => {
    const { queue, playing, shuffle } = get();
    if (!playing || queue.length === 0) return;
    const idx = queue.findIndex((t) => t.ID === playing.ID);
    if (idx < 0) return;
    const target = nextQueueIndex(queue.length, idx, shuffle);
    if (target === "stop" || target === idx) return;
    set({ playing: queue[target], externallyPaused: false });
  },

  prev: () => {
    const { queue, playing, shuffle } = get();
    if (!playing || queue.length === 0) return;
    const idx = queue.findIndex((t) => t.ID === playing.ID);
    if (idx < 0) return;
    const target = previousQueueIndex(queue.length, idx, shuffle);
    if (target === idx) return;
    set({ playing: queue[target], externallyPaused: false });
  },

  close: () => set({ playing: null, queue: [], externallyPaused: false, source: null }),
}));

/**
 * The queue with `tracks` inserted at `at` (the end when absent or out of
 * range), skipping any track already queued or repeated in `tracks`.
 */
export function insertTracks(queue: MusicTrack[], tracks: MusicTrack[], at?: number): MusicTrack[] {
  const seen = new Set(queue.map((t) => t.ID));
  const fresh: MusicTrack[] = [];
  for (const t of tracks) {
    if (!t.ID || seen.has(t.ID)) continue;
    seen.add(t.ID);
    fresh.push(t);
  }
  if (fresh.length === 0) return queue;
  const pos = at === undefined || at < 0 || at > queue.length ? queue.length : at;
  return [...queue.slice(0, pos), ...fresh, ...queue.slice(pos)];
}

/**
 * The list with the entry at `from` moved so that it lands at index `to` of
 * the result. Out of range is a no-op.
 */
export function moveTrack<T>(list: T[], from: number, to: number): T[] {
  if (from < 0 || from >= list.length || to < 0 || to >= list.length || from === to) return list;
  const next = [...list];
  const [item] = next.splice(from, 1);
  next.splice(to, 0, item);
  return next;
}
