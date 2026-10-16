import { renderHook, act } from "@testing-library/react";
import { fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { usePlayerKeyboard } from "./usePlayerKeyboard";
import type { SubtitleStream } from "../SubtitleSelector";

function createVideoElement() {
  const el = document.createElement("video");
  Object.defineProperty(el, "volume", { value: 0.5, writable: true, configurable: true });
  Object.defineProperty(el, "muted", { value: false, writable: true, configurable: true });
  Object.defineProperty(el, "paused", { value: true, writable: true, configurable: true });
  Object.defineProperty(el, "currentTime", { value: 50, writable: true, configurable: true });
  Object.defineProperty(el, "duration", { value: 200, writable: true, configurable: true });
  el.play = vi.fn().mockResolvedValue(undefined);
  el.pause = vi.fn();
  return el;
}

function makeArgs(video: HTMLVideoElement, overrides: Partial<ReturnType<typeof defaultArgs>> = {}) {
  return { ...defaultArgs(video), ...overrides };
}

function defaultArgs(video: HTMLVideoElement) {
  return {
    videoRef: { current: video },
    positionRef: { current: 50 },
    knownDurationSecRef: { current: 200 },
    seekRef: { current: vi.fn() },
    skipFwdRef: { current: 10 },
    skipBwdRef: { current: 10 },
    playbackSpeedRef: { current: 1 },
    subtitleStreamsRef: { current: [] as SubtitleStream[] },
    currentSubtitleRef: { current: null as number | null },
    anyPanelOpenRef: { current: false },
    closePanelRef: { current: vi.fn() },
    pipToggleRef: { current: null as (() => void) | null },
    castRef: { current: null as { connected: boolean; isPaused: boolean; playPause: () => void } | null },
    onCloseRef: { current: vi.fn() },
    setPlaybackSpeed: vi.fn(),
    onSubtitleChange: vi.fn(),
    setShowShortcuts: vi.fn(),
    showActionFlash: vi.fn(),
  };
}

function pressKey(key: string) {
  act(() => {
    fireEvent.keyDown(window, { key });
  });
}

describe("usePlayerKeyboard", () => {
  let video: HTMLVideoElement;

  beforeEach(() => {
    video = createVideoElement();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("play/pause (Space and k)", () => {
    it("plays video when paused on Space", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey(" ");

      expect(video.play).toHaveBeenCalled();
      expect(args.showActionFlash).toHaveBeenCalledWith("play");
    });

    it("pauses video when playing on k", () => {
      Object.defineProperty(video, "paused", { value: false });
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("k");

      expect(video.pause).toHaveBeenCalled();
      expect(args.showActionFlash).toHaveBeenCalledWith("pause");
    });
  });

  // While a cast session is live the LOCAL video is paused by the handoff;
  // space must toggle the TV, never wake the local element - pressing it used
  // to start a second, uncontrollable local playback next to the cast.
  describe("play/pause while casting", () => {
    it("routes Space to the cast toggle and never touches the local video", () => {
      const playPause = vi.fn();
      const args = makeArgs(video, {
        castRef: { current: { connected: true, isPaused: true, playPause } },
      });
      renderHook(() => usePlayerKeyboard(args));

      pressKey(" ");

      expect(playPause).toHaveBeenCalledTimes(1);
      expect(video.play).not.toHaveBeenCalled();
      expect(video.pause).not.toHaveBeenCalled();
      expect(args.showActionFlash).toHaveBeenCalledWith("play");
    });

    it("flashes pause when the cast is playing (k)", () => {
      const playPause = vi.fn();
      const args = makeArgs(video, {
        castRef: { current: { connected: true, isPaused: false, playPause } },
      });
      renderHook(() => usePlayerKeyboard(args));

      pressKey("k");

      expect(playPause).toHaveBeenCalledTimes(1);
      expect(args.showActionFlash).toHaveBeenCalledWith("pause");
    });

    it("keeps driving the local video when the cast ref is disconnected", () => {
      const playPause = vi.fn();
      const args = makeArgs(video, {
        castRef: { current: { connected: false, isPaused: true, playPause } },
      });
      renderHook(() => usePlayerKeyboard(args));

      pressKey(" ");

      expect(playPause).not.toHaveBeenCalled();
      expect(video.play).toHaveBeenCalled();
    });
  });

  describe("seek (ArrowLeft/j, ArrowRight/l)", () => {
    it("seeks backward on ArrowLeft", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("ArrowLeft");

      expect(args.seekRef.current).toHaveBeenCalledWith(40);
    });

    it("seeks backward on j", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("j");

      expect(args.seekRef.current).toHaveBeenCalledWith(40);
    });

    it("clamps backward seek to 0", () => {
      const args = makeArgs(video);
      args.positionRef.current = 3;
      renderHook(() => usePlayerKeyboard(args));

      pressKey("ArrowLeft");

      expect(args.seekRef.current).toHaveBeenCalledWith(0);
    });

    it("seeks forward on ArrowRight", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("ArrowRight");

      expect(args.seekRef.current).toHaveBeenCalledWith(60);
    });

    it("seeks forward on l", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("l");

      expect(args.seekRef.current).toHaveBeenCalledWith(60);
    });
  });

  describe("volume (ArrowUp/ArrowDown)", () => {
    it("increases volume on ArrowUp", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("ArrowUp");

      expect(video.volume).toBeCloseTo(0.6);
    });

    it("caps volume at 1", () => {
      Object.defineProperty(video, "volume", { value: 0.95, writable: true });
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("ArrowUp");

      expect(video.volume).toBeLessThanOrEqual(1);
    });

    it("decreases volume on ArrowDown", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("ArrowDown");

      expect(video.volume).toBeCloseTo(0.4);
    });

    it("floors volume at 0", () => {
      Object.defineProperty(video, "volume", { value: 0.05, writable: true });
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("ArrowDown");

      expect(video.volume).toBeGreaterThanOrEqual(0);
    });
  });

  describe("mute (m)", () => {
    it("toggles muted on", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("m");

      expect(video.muted).toBe(true);
    });

    it("toggles muted off", () => {
      Object.defineProperty(video, "muted", { value: true, writable: true });
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("m");

      expect(video.muted).toBe(false);
    });
  });

  describe("subtitle toggle (c)", () => {
    it("enables first subtitle when currently off", () => {
      const subs: SubtitleStream[] = [
        { index: 0, trackIndex: 0, language: "eng", codec: "subrip" },
      ];
      const args = makeArgs(video);
      args.subtitleStreamsRef.current = subs;
      args.currentSubtitleRef.current = null;
      renderHook(() => usePlayerKeyboard(args));

      pressKey("c");

      expect(args.onSubtitleChange).toHaveBeenCalledWith(0);
    });

    it("disables subtitle when currently on", () => {
      const subs: SubtitleStream[] = [
        { index: 0, trackIndex: 0, language: "eng", codec: "subrip" },
      ];
      const args = makeArgs(video);
      args.subtitleStreamsRef.current = subs;
      args.currentSubtitleRef.current = 0;
      renderHook(() => usePlayerKeyboard(args));

      pressKey("c");

      expect(args.onSubtitleChange).toHaveBeenCalledWith(null);
    });

    it("does nothing when no subtitles available", () => {
      const args = makeArgs(video);
      args.subtitleStreamsRef.current = [];
      renderHook(() => usePlayerKeyboard(args));

      pressKey("c");

      expect(args.onSubtitleChange).not.toHaveBeenCalled();
    });
  });

  describe("Escape", () => {
    it("closes panel when panel is open", () => {
      const args = makeArgs(video);
      args.anyPanelOpenRef.current = true;
      renderHook(() => usePlayerKeyboard(args));

      pressKey("Escape");

      expect(args.closePanelRef.current).toHaveBeenCalled();
      expect(args.onCloseRef.current).not.toHaveBeenCalled();
    });

    it("calls onClose when no panel open", () => {
      const args = makeArgs(video);
      args.anyPanelOpenRef.current = false;
      renderHook(() => usePlayerKeyboard(args));

      pressKey("Escape");

      expect(args.onCloseRef.current).toHaveBeenCalled();
    });

    // A viewer refusing the next episode used to be thrown out of the player
    // by the very key they reached for (prod 2026-10-15).
    it("cancels a running credits countdown instead of leaving the player", () => {
      const args = makeArgs(video);
      const cancel = vi.fn();
      const cancelCountdownRef = { current: cancel as (() => void) | null };
      args.anyPanelOpenRef.current = true;
      renderHook(() => usePlayerKeyboard({ ...args, cancelCountdownRef }));

      pressKey("Escape");

      expect(cancel).toHaveBeenCalledTimes(1);
      expect(args.closePanelRef.current).not.toHaveBeenCalled();
      expect(args.onCloseRef.current).not.toHaveBeenCalled();

      cancelCountdownRef.current = null;
      pressKey("Escape");
      expect(args.closePanelRef.current).toHaveBeenCalledTimes(1);
    });

    it("yields Escape to an open aria-modal dialog (does not exit the player)", () => {
      const dialog = document.createElement("div");
      dialog.setAttribute("role", "dialog");
      dialog.setAttribute("aria-modal", "true");
      document.body.appendChild(dialog);
      try {
        const args = makeArgs(video);
        args.anyPanelOpenRef.current = false;
        renderHook(() => usePlayerKeyboard(args));

        pressKey("Escape");

        expect(args.onCloseRef.current).not.toHaveBeenCalled();
      } finally {
        document.body.removeChild(dialog);
      }
    });

    it("ignores playback keys while an aria-modal dialog is open", () => {
      const dialog = document.createElement("div");
      dialog.setAttribute("role", "dialog");
      dialog.setAttribute("aria-modal", "true");
      document.body.appendChild(dialog);
      try {
        const args = makeArgs(video);
        renderHook(() => usePlayerKeyboard(args));

        pressKey(" ");

        expect(video.play).not.toHaveBeenCalled();
      } finally {
        document.body.removeChild(dialog);
      }
    });
  });

  describe("playback speed (< and >)", () => {
    it("decreases speed on < key", () => {
      const args = makeArgs(video);
      args.playbackSpeedRef.current = 1;
      renderHook(() => usePlayerKeyboard(args));

      pressKey("<");

      expect(args.setPlaybackSpeed).toHaveBeenCalledWith(0.75);
    });

    it("increases speed on > key", () => {
      const args = makeArgs(video);
      args.playbackSpeedRef.current = 1;
      renderHook(() => usePlayerKeyboard(args));

      pressKey(">");

      expect(args.setPlaybackSpeed).toHaveBeenCalledWith(1.25);
    });

    it("does not decrease below min speed", () => {
      const args = makeArgs(video);
      args.playbackSpeedRef.current = 0.5;
      renderHook(() => usePlayerKeyboard(args));

      pressKey("<");

      expect(args.setPlaybackSpeed).not.toHaveBeenCalled();
    });

    it("does not increase above max speed", () => {
      const args = makeArgs(video);
      args.playbackSpeedRef.current = 2;
      renderHook(() => usePlayerKeyboard(args));

      pressKey(">");

      expect(args.setPlaybackSpeed).not.toHaveBeenCalled();
    });

    it("also responds to , for decrease", () => {
      const args = makeArgs(video);
      args.playbackSpeedRef.current = 1.25;
      renderHook(() => usePlayerKeyboard(args));

      pressKey(",");

      expect(args.setPlaybackSpeed).toHaveBeenCalledWith(1);
    });

    it("also responds to . for increase", () => {
      const args = makeArgs(video);
      args.playbackSpeedRef.current = 1.25;
      renderHook(() => usePlayerKeyboard(args));

      pressKey(".");

      expect(args.setPlaybackSpeed).toHaveBeenCalledWith(1.5);
    });
  });

  describe("number keys (0-9) seek to percentage", () => {
    it("seeks to 50% on key 5", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("5");

      expect(args.seekRef.current).toHaveBeenCalledWith(100);
    });

    it("seeks to 0% on key 0", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("0");

      expect(args.seekRef.current).toHaveBeenCalledWith(0);
    });

    it("seeks to 90% on key 9", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("9");

      expect(args.seekRef.current).toHaveBeenCalledWith(180);
    });
  });

  describe("? key toggles shortcuts overlay", () => {
    it("calls setShowShortcuts toggler", () => {
      const args = makeArgs(video);
      renderHook(() => usePlayerKeyboard(args));

      pressKey("?");

      expect(args.setShowShortcuts).toHaveBeenCalledWith(expect.any(Function));
    });
  });

  describe("cleanup", () => {
    it("removes keydown listener on unmount", () => {
      const removeSpy = vi.spyOn(window, "removeEventListener");
      const args = makeArgs(video);
      const { unmount } = renderHook(() => usePlayerKeyboard(args));

      unmount();

      expect(removeSpy).toHaveBeenCalledWith("keydown", expect.any(Function), true);
    });
  });
});
