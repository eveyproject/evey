import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { getZustandStorage } from "@evey/store";

export interface QueueTrack {
  id: string;
  title: string;
  artist: string;
  album: string;
  duration: number;
}

// MusicSource records where the current queue was launched from (album,
// playlist, artist), so the mini-player can link back to it. Mirrors the web
// musicPlayer store's MusicSource shape.
export interface MusicSource {
  type: "album" | "playlist" | "artist";
  id: string;
  label: string;
}

interface MusicQueueState {
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

// Picks a random index other than the current one. Queues of length 1 have
// no other track to pick, so callers must guard that case themselves.
function randomOtherIndex(length: number, currentIndex: number): number {
  let idx = currentIndex;
  while (idx === currentIndex) idx = Math.floor(Math.random() * length);
  return idx;
}

export const useMusicQueueStore = create<MusicQueueState>()(
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
        if (index >= 0 && index < queue.length) {
          set({ currentIndex: index, isPlaying: true });
        }
      },

      next: () => {
        const { queue, currentIndex, shuffle } = get();
        if (queue.length === 0) return;
        if (shuffle) {
          if (queue.length === 1) return;
          set({ currentIndex: randomOtherIndex(queue.length, currentIndex), isPlaying: true });
          return;
        }
        if (currentIndex < queue.length - 1) {
          set({ currentIndex: currentIndex + 1, isPlaying: true });
          return;
        }
        // The last track finished: nothing plays any more and the bar must
        // say so. Left "playing", it showed a pause button over silence and
        // its play/pause toggle then had to be pressed twice.
        set({ isPlaying: false });
      },

      previous: () => {
        const { queue, currentIndex, shuffle } = get();
        if (queue.length === 0) return;
        if (shuffle) {
          if (queue.length === 1) return;
          set({ currentIndex: randomOtherIndex(queue.length, currentIndex), isPlaying: true });
          return;
        }
        if (currentIndex > 0) {
          set({ currentIndex: currentIndex - 1, isPlaying: true });
        }
      },

      togglePlay: () => set((s) => ({ isPlaying: !s.isPlaying })),
      toggleShuffle: () => set((s) => ({ shuffle: !s.shuffle })),

      clear: () => set({ queue: [], currentIndex: -1, isPlaying: false, source: null }),

      removeFromQueue: (index) => {
        const { queue, currentIndex } = get();
        const newQueue = queue.filter((_, i) => i !== index);
        let newIndex = currentIndex;
        if (index < currentIndex) newIndex--;
        if (index === currentIndex) newIndex = Math.min(newIndex, newQueue.length - 1);
        set({ queue: newQueue, currentIndex: newIndex >= 0 ? newIndex : -1, isPlaying: newQueue.length > 0 });
      },
    }),
    {
      name: "evey-music-queue",
      storage: createJSONStorage(() => getZustandStorage()),
      // Only the shuffle toggle survives a restart - the queue itself is
      // ephemeral session state (track objects aren't versioned for storage).
      partialize: (state) => ({ shuffle: state.shuffle }),
    }
  )
);
