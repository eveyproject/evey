import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";

// The music queue, the same rules as the Android TV and the phone: a queue
// with a cursor, next/previous, a shuffle that picks any other track, a
// finished album that reads as stopped. Only shuffle survives a restart.
export interface QueueTrack {
  id: string;
  title: string;
  artist: string;
  album: string;
  duration: number;
  coverArtUrl?: string;
}

interface MusicQueueState {
  queue: QueueTrack[];
  currentIndex: number;
  isPlaying: boolean;
  shuffle: boolean;
  setQueue: (tracks: QueueTrack[], startIndex?: number) => void;
  next: () => void;
  previous: () => void;
  togglePlay: () => void;
  toggleShuffle: () => void;
  clear: () => void;
}

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
      setQueue: (tracks, startIndex = 0) => set({ queue: tracks, currentIndex: startIndex, isPlaying: true }),
      next: () => {
        const { queue, currentIndex, shuffle } = get();
        if (queue.length === 0) return;
        if (shuffle) {
          if (queue.length === 1) return;
          set({ currentIndex: randomOtherIndex(queue.length, currentIndex), isPlaying: true });
          return;
        }
        if (currentIndex < queue.length - 1) set({ currentIndex: currentIndex + 1, isPlaying: true });
        else set({ isPlaying: false });
      },
      previous: () => {
        const { queue, currentIndex, shuffle } = get();
        if (queue.length === 0) return;
        if (shuffle) {
          if (queue.length === 1) return;
          set({ currentIndex: randomOtherIndex(queue.length, currentIndex), isPlaying: true });
          return;
        }
        if (currentIndex > 0) set({ currentIndex: currentIndex - 1, isPlaying: true });
      },
      togglePlay: () => set((s) => ({ isPlaying: !s.isPlaying })),
      toggleShuffle: () => set((s) => ({ shuffle: !s.shuffle })),
      clear: () => set({ queue: [], currentIndex: -1, isPlaying: false }),
    }),
    {
      name: "evey-tv-web-music-queue",
      storage: createJSONStorage(() => localStorage),
      partialize: (state) => ({ shuffle: state.shuffle }),
    },
  ),
);

export function trackToQueueTrack(t: { id?: string; title?: string; artist?: string; album?: string; duration?: number; coverArtUrl?: string }): QueueTrack {
  return { id: t.id ?? "", title: t.title ?? "", artist: t.artist ?? "", album: t.album ?? "", duration: t.duration ?? 0, coverArtUrl: t.coverArtUrl };
}
