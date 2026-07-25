import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("react-native", () => ({
  Platform: { OS: "android", select: (obj: Record<string, unknown>) => obj.android },
  Dimensions: { get: () => ({ width: 1000, height: 500 }) },
  PanResponder: {
    create: (config: any) => ({ panHandlers: {}, _config: config }),
  },
}));

import { renderHook, act } from "@testing-library/react";
import { usePlayerGestures } from "./usePlayerGestures";

function setup(extra: Partial<{ skipForwardSeconds: number; skipBackwardSeconds: number; hasOpenPanel: () => boolean; onCloseOverlays: () => void }> = {}) {
  const cbs = {
    onSeekBy: vi.fn(),
    onTogglePlayPause: vi.fn(),
    onToggleControls: vi.fn(),
    onVolumeChange: vi.fn(),
    onBrightnessChange: vi.fn(),
    getVolume: vi.fn(() => 0.5),
    getBrightness: vi.fn(() => 0.5),
    ...extra,
  };
  const { result } = renderHook(() => usePlayerGestures(cbs));
  const config = (result.current.panResponder as any)._config;
  return { result, config, cbs };
}

function makeEvt(pageX: number, pageY = 250) {
  return { nativeEvent: { pageX, pageY } } as any;
}

function makeGS(overrides: Partial<{ dx: number; dy: number }> = {}) {
  return { dx: 0, dy: 0, ...overrides } as any;
}

beforeEach(() => { vi.useFakeTimers(); });
afterEach(() => { vi.useRealTimers(); });

describe("usePlayerGestures", () => {
  describe("double tap", () => {
    it("double tap left zone triggers seekBy(-skipBackwardSeconds) [default 30]", () => {
      const { config, cbs } = setup();

      config.onPanResponderGrant(makeEvt(100), makeGS());
      config.onPanResponderRelease(makeEvt(100), makeGS());

      config.onPanResponderGrant(makeEvt(100), makeGS());
      config.onPanResponderRelease(makeEvt(100), makeGS());

      expect(cbs.onSeekBy).toHaveBeenCalledWith(-30);
    });

    it("double tap honours the configured skip seconds", () => {
      const { config, cbs } = setup({ skipForwardSeconds: 25, skipBackwardSeconds: 15 });

      config.onPanResponderGrant(makeEvt(100), makeGS());
      config.onPanResponderRelease(makeEvt(100), makeGS());
      config.onPanResponderGrant(makeEvt(100), makeGS());
      config.onPanResponderRelease(makeEvt(100), makeGS());
      expect(cbs.onSeekBy).toHaveBeenCalledWith(-15);

      config.onPanResponderGrant(makeEvt(800), makeGS());
      config.onPanResponderRelease(makeEvt(800), makeGS());
      config.onPanResponderGrant(makeEvt(800), makeGS());
      config.onPanResponderRelease(makeEvt(800), makeGS());
      expect(cbs.onSeekBy).toHaveBeenCalledWith(25);
    });

    it("double tap right zone triggers seekBy(skipForwardSeconds) [default 30]", () => {
      const { config, cbs } = setup();

      config.onPanResponderGrant(makeEvt(800), makeGS());
      config.onPanResponderRelease(makeEvt(800), makeGS());

      config.onPanResponderGrant(makeEvt(800), makeGS());
      config.onPanResponderRelease(makeEvt(800), makeGS());

      expect(cbs.onSeekBy).toHaveBeenCalledWith(30);
    });

    it("double tap center zone triggers togglePlayPause", () => {
      const { config, cbs } = setup();

      config.onPanResponderGrant(makeEvt(500), makeGS());
      config.onPanResponderRelease(makeEvt(500), makeGS());

      config.onPanResponderGrant(makeEvt(500), makeGS());
      config.onPanResponderRelease(makeEvt(500), makeGS());

      expect(cbs.onTogglePlayPause).toHaveBeenCalled();
    });
  });

  describe("single tap", () => {
    it("single tap toggles controls after delay", () => {
      const { config, cbs } = setup();

      config.onPanResponderGrant(makeEvt(500), makeGS());
      config.onPanResponderRelease(makeEvt(500), makeGS());

      expect(cbs.onToggleControls).not.toHaveBeenCalled();
      act(() => { vi.advanceTimersByTime(300); });
      expect(cbs.onToggleControls).toHaveBeenCalled();
    });
  });

  describe("vertical swipe", () => {
    it("vertical swipe on right side triggers volume change", () => {
      const { config, cbs } = setup();

      config.onPanResponderGrant(makeEvt(700, 250), makeGS());
      config.onPanResponderMove(makeEvt(700, 250), makeGS({ dy: -30 }));
      config.onPanResponderMove(makeEvt(700, 220), makeGS({ dy: -60 }));

      expect(cbs.onVolumeChange).toHaveBeenCalled();
    });

    it("vertical swipe on left side triggers brightness change", () => {
      const { config, cbs } = setup();

      config.onPanResponderGrant(makeEvt(200, 250), makeGS());
      config.onPanResponderMove(makeEvt(200, 250), makeGS({ dy: -30 }));
      config.onPanResponderMove(makeEvt(200, 220), makeGS({ dy: -60 }));

      expect(cbs.onBrightnessChange).toHaveBeenCalled();
    });

    it("swipe release resets indicators and does not toggle controls", () => {
      const { config, cbs } = setup();

      config.onPanResponderGrant(makeEvt(700, 250), makeGS());
      config.onPanResponderMove(makeEvt(700, 250), makeGS({ dy: -30 }));
      config.onPanResponderMove(makeEvt(700, 220), makeGS({ dy: -60 }));
      config.onPanResponderRelease(makeEvt(700, 220), makeGS({ dy: -60 }));

      expect(cbs.onToggleControls).not.toHaveBeenCalled();
    });
  });

  // Docked panel (settings) open: the PanResponder must yield - without this
  // it stole the panel's scroll and a vertical swipe there changed the volume.
  describe("panel open", () => {
    it("does not claim the responder while a panel is open", () => {
      const { config } = setup({ hasOpenPanel: () => true });

      expect(config.onStartShouldSetPanResponder()).toBe(false);
      expect(config.onMoveShouldSetPanResponder(makeEvt(700), makeGS({ dy: -60 }))).toBe(false);
    });

    it("claims the responder again once the panel closes", () => {
      let open = true;
      const { config } = setup({ hasOpenPanel: () => open });

      expect(config.onStartShouldSetPanResponder()).toBe(false);
      open = false;
      expect(config.onStartShouldSetPanResponder()).toBe(true);
      expect(config.onMoveShouldSetPanResponder(makeEvt(700), makeGS({ dy: -60 }))).toBe(true);
    });

    it("a tap with a panel open closes it and does nothing else", () => {
      const onCloseOverlays = vi.fn();
      const { config, cbs } = setup({ hasOpenPanel: () => true, onCloseOverlays });

      config.onPanResponderGrant(makeEvt(500), makeGS());
      config.onPanResponderRelease(makeEvt(500), makeGS());
      act(() => { vi.advanceTimersByTime(400); });

      expect(onCloseOverlays).toHaveBeenCalledTimes(1);
      expect(cbs.onToggleControls).not.toHaveBeenCalled();
      expect(cbs.onTogglePlayPause).not.toHaveBeenCalled();
    });
  });
});
