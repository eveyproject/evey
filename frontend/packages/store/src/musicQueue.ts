import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { lazyZustandStorage } from "./storage";

// The music queue every client keeps, in ONE place. The phone, the Android
// television and the browser television each had their own copy of these
// rules; the browser's own player (musicPlayer.ts) states them a fourth time
// over track objects instead of an index, and reuses the reducers below so
// the two shapes can never drift apart.

export interface QueueTrack {
  id: string;
  title: string;
  artist: string;
  album: string;
  duration: number;
  coverArtUrl?: string;
}

/**
 * Where the queue was started from, so a mini player can link back to it.
 * The browser and the phone both carry it; the televisions simply leave it
 * null today.
 */
export interface MusicSource {
  type: "album" | "playlist" | "artist";
  id: string;
  label: string;
}

/** A random index other than the current one. A queue of one has none. */
export function randomOtherIndex(length: number, currentIndex: number, random: () => number = Math.random): number {
  if (length <= 1) return currentIndex;
  let idx = currentIndex;
  while (idx === currentIndex) idx = Math.floor(random() * length);
  return idx;
}

/**
 * Where "next" goes: another track under shuffle, the following one in order,
 * and "stop" once the last has finished - which is what tells a bar to stop
 * showing a pause button over silence.
 */
export function nextQueueIndex(
  length: number,
  currentIndex: number,
  shuffle: boolean,
  random: () => number = Math.random,
): number | "stop" {
  if (length === 0) return "stop";
  if (shuffle) return length === 1 ? currentIndex : randomOtherIndex(length, currentIndex, random);
  if (currentIndex < length - 1) return currentIndex + 1;
  return "stop";
}

/**
 * Where "previous" goes. At the head of the queue it stays put: a viewer who
 * presses it once more expects the first track, not silence.
 */
export function previousQueueIndex(
  length: number,
  currentIndex: number,
  shuffle: boolean,
  random: () => number = Math.random,
): number {
  if (length === 0) return currentIndex;
  if (shuffle) return length === 1 ? currentIndex : randomOtherIndex(length, currentIndex, random);
  return currentIndex > 0 ? currentIndex - 1 : currentIndex;
}

/** Dropping one entry, keeping the cursor on the same track where possible. */
export function removeFromQueueAt<T>(queue: T[], currentIndex: number, index: number): { queue: T[]; currentIndex: number } {
  const next = queue.filter((_, i) => i !== index);
  let cursor = currentIndex;
  if (index < currentIndex) cursor--;
  if (index === currentIndex) cursor = Math.min(cursor, next.length - 1);
  return { queue: next, currentIndex: cursor >= 0 ? cursor : -1 };
}

export interface MusicQueueState {
  queue: QueueTrack[];
  currentIndex: number;
  isPlaying: boolean;
  shuffle: boolean;
  source: MusicSource | null;

  setQueue: (tracks: QueueTrack[], startIndex?: number, source?: MusicSource | null) => void;
  playTrack: (index: number) => void;
  next: () => void;
  previous: () => void;
  togglePlay: () => void;
  toggleShuffle: () => void;
  clear: () => void;
  removeFromQueue: (index: number) => void;
}

/**
 * One queue store per client, told where to persist. Only the shuffle toggle
 * survives a restart: the tracks themselves are session state, and their
 * shape is not versioned for storage.
 */
export function createMusicQueueStore(storageName: string) {
  return create<MusicQueueState>()(
    persist(
      (set, get) => ({
        queue: [],
        currentIndex: -1,
        isPlaying: false,
        shuffle: false,
        source: null,

        setQueue: (tracks, startIndex = 0, source = null) =>
          set({ queue: tracks, currentIndex: startIndex, isPlaying: true, source }),

        playTrack: (index) => {
          const { queue } = get();
          if (index >= 0 && index < queue.length) set({ currentIndex: index, isPlaying: true });
        },

        next: () => {
          const { queue, currentIndex, shuffle } = get();
          const target = nextQueueIndex(queue.length, currentIndex, shuffle);
          if (target === "stop") {
            if (queue.length > 0) set({ isPlaying: false });
            return;
          }
          if (target === currentIndex) return;
          set({ currentIndex: target, isPlaying: true });
        },

        previous: () => {
          const { queue, currentIndex, shuffle } = get();
          const target = previousQueueIndex(queue.length, currentIndex, shuffle);
          if (target === currentIndex) return;
          set({ currentIndex: target, isPlaying: true });
        },

        togglePlay: () => set((s) => ({ isPlaying: !s.isPlaying })),
        toggleShuffle: () => set((s) => ({ shuffle: !s.shuffle })),

        clear: () => set({ queue: [], currentIndex: -1, isPlaying: false, source: null }),

        removeFromQueue: (index) => {
          const { queue, currentIndex } = get();
          const next = removeFromQueueAt(queue, currentIndex, index);
          set({ ...next, isPlaying: next.queue.length > 0 });
        },
      }),
      {
        name: storageName,
        storage: createJSONStorage(() => lazyZustandStorage),
        partialize: (state) => ({ shuffle: state.shuffle }),
      },
    ),
  );
}

/** A server track, as the queue keeps it. */
export function trackToQueueTrack(t: {
  id?: string;
  title?: string;
  artist?: string;
  album?: string;
  duration?: number;
  coverArtUrl?: string;
}): QueueTrack {
  return {
    id: t.id ?? "",
    title: t.title ?? "",
    artist: t.artist ?? "",
    album: t.album ?? "",
    duration: t.duration ?? 0,
    coverArtUrl: t.coverArtUrl,
  };
}
