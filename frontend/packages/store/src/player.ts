import { create } from "zustand";

export type PlayerMode = "hidden" | "mini" | "full";

interface PlayerState {
  mediaId: string | null;
  mediaTitle: string | null;
  sessionId: string | null;
  streamToken: string | null;
  mode: PlayerMode;
  isActive: boolean;
  videoElement: HTMLVideoElement | null;
  _stopSessionFn: (() => void) | null;

  play: (mediaId: string, title?: string) => void;
  setSessionId: (sessionId: string) => void;
  setStreamToken: (token: string) => void;
  setMode: (mode: PlayerMode) => void;
  setVideoElement: (el: HTMLVideoElement | null) => void;
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
    const { _stopSessionFn, videoElement } = get();
    if (_stopSessionFn) _stopSessionFn();
    if (videoElement) {
      videoElement.pause();
      videoElement.removeAttribute("src");
      videoElement.load();
    }
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
