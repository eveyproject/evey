import { create } from "zustand";

export interface QueueTrack {
  id: string;
  title: string;
  artist: string;
  album: string;
  duration: number;
}

interface MusicQueueState {
  queue: QueueTrack[];
  currentIndex: number;
  isPlaying: boolean;

  setQueue: (tracks: QueueTrack[], startIndex?: number) => void;
  playTrack: (index: number) => void;
  next: () => void;
  previous: () => void;
  togglePlay: () => void;
  clear: () => void;
}

export const useMusicQueueStore = create<MusicQueueState>()((set, get) => ({
  queue: [],
  currentIndex: -1,
  isPlaying: false,

  setQueue: (tracks, startIndex = 0) =>
    set({ queue: tracks, currentIndex: startIndex, isPlaying: true }),

  playTrack: (index) => {
    const { queue } = get();
    if (index >= 0 && index < queue.length) {
      set({ currentIndex: index, isPlaying: true });
    }
  },

  next: () => {
    const { queue, currentIndex } = get();
    if (currentIndex < queue.length - 1) {
      set({ currentIndex: currentIndex + 1, isPlaying: true });
      return;
    }
    // The last track finished: nothing plays any more and the bar must say
    // so. Left "playing", it showed a pause button over silence and its
    // play/pause toggle then had to be pressed twice.
    set({ isPlaying: false });
  },

  previous: () => {
    const { currentIndex } = get();
    if (currentIndex > 0) {
      set({ currentIndex: currentIndex - 1, isPlaying: true });
    }
  },

  togglePlay: () => set((s) => ({ isPlaying: !s.isPlaying })),

  clear: () => set({ queue: [], currentIndex: -1, isPlaying: false }),
}));
