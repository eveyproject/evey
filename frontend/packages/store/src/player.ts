import { create } from "zustand";

// "mini" is still used by the mobile app's MiniPlayer; the web app dropped its
// floating mini-player (back = stop) but the type stays shared across apps.
export type PlayerMode = "hidden" | "mini" | "full";

let _cleanupFn: (() => void) | null = null;
export function setPlayerCleanup(fn: (() => void) | null) { _cleanupFn = fn; }

interface PlayerState {
  mediaId: string | null;
  mediaTitle: string | null;
  sessionId: string | null;
  streamToken: string | null;
  mode: PlayerMode;
  isActive: boolean;
  videoElement: unknown;
  _stopSessionFn: (() => void) | null;

  play: (mediaId: string, title?: string) => void;
  setSessionId: (sessionId: string) => void;
  setStreamToken: (token: string) => void;
  setMode: (mode: PlayerMode) => void;
  setVideoElement: (el: unknown) => void;
  setStopSessionFn: (fn: (() => void) | null) => void;
  stop: () => void;
}

export const usePlayerStore = create<PlayerState>()((set, get) => ({
  mediaId: null,
  mediaTitle: null,
  sessionId: null,
  streamToken: null,
  mode: "hidden",
  isActive: false,
  videoElement: null,
  _stopSessionFn: null,

  play: (mediaId, title) =>
    set((state) => {
      if (state.isActive && state.mediaId === mediaId) {
        return { mode: "full" };
      }
      return {
        mediaId,
        mediaTitle: title ?? null,
        sessionId: null,
        streamToken: null,
        mode: "full",
        isActive: true,
      };
    }),

  setSessionId: (sessionId) => set({ sessionId }),
  setStreamToken: (token) => set({ streamToken: token }),
  setMode: (mode) => set({ mode }),
  setVideoElement: (el) => set({ videoElement: el }),
  setStopSessionFn: (fn) => set({ _stopSessionFn: fn }),

  stop: () => {
    const { _stopSessionFn } = get();
    if (_stopSessionFn) _stopSessionFn();
    _cleanupFn?.();
    set({
      mediaId: null,
      mediaTitle: null,
      sessionId: null,
      streamToken: null,
      mode: "hidden",
      isActive: false,
      videoElement: null,
      _stopSessionFn: null,
    });
  },
}));
