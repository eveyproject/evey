import { create } from "zustand";

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

// MusicSource records where the current playback was launched from, so the
// mini-player can link back to it. `id` is the value needed to navigate:
// the album title (album), the playlist id (playlist), or the artist name.
export interface MusicSource {
  type: "album" | "playlist" | "artist";
  id: string;
  label: string;
}

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

  toggleShuffle: () => set((s) => {
    const next = !s.shuffle;
    saveShuffle(next);
    return { shuffle: next };
  }),

  pauseExternal: () => set({ externallyPaused: true }),
  resumeExternal: () => set({ externallyPaused: false }),

  next: () => {
    const { queue, playing, shuffle } = get();
    if (!playing || queue.length === 0) return;
    if (shuffle) {
      const others = queue.filter((t) => t.ID !== playing.ID);
      if (!others.length) return;
      const rand = others[Math.floor(Math.random() * others.length)];
      set({ playing: rand, externallyPaused: false });
      return;
    }
    const idx = queue.findIndex((t) => t.ID === playing.ID);
    if (idx >= 0 && idx < queue.length - 1) {
      set({ playing: queue[idx + 1], externallyPaused: false });
    }
  },

  prev: () => {
    const { queue, playing, shuffle } = get();
    if (!playing || queue.length === 0) return;
    if (shuffle) {
      const others = queue.filter((t) => t.ID !== playing.ID);
      if (!others.length) return;
      const rand = others[Math.floor(Math.random() * others.length)];
      set({ playing: rand, externallyPaused: false });
      return;
    }
    const idx = queue.findIndex((t) => t.ID === playing.ID);
    if (idx > 0) set({ playing: queue[idx - 1], externallyPaused: false });
  },

  close: () => set({ playing: null, queue: [], externallyPaused: false, source: null }),
}));
