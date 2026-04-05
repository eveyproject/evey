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

function setup() {
  const cbs = {
    onSeekBy: vi.fn(),
    onTogglePlayPause: vi.fn(),
    onToggleControls: vi.fn(),
    onVolumeChange: vi.fn(),
    onBrightnessChange: vi.fn(),
    getVolume: vi.fn(() => 0.5),
    getBrightness: vi.fn(() => 0.5),
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
    it("double tap left zone triggers seekBy(-10)", () => {
      const { config, cbs } = setup();

      config.onPanResponderGrant(makeEvt(100), makeGS());
      config.onPanResponderRelease(makeEvt(100), makeGS());

      config.onPanResponderGrant(makeEvt(100), makeGS());
      config.onPanResponderRelease(makeEvt(100), makeGS());

      expect(cbs.onSeekBy).toHaveBeenCalledWith(-10);
    });

    it("double tap right zone triggers seekBy(10)", () => {
      const { config, cbs } = setup();

      config.onPanResponderGrant(makeEvt(800), makeGS());
      config.onPanResponderRelease(makeEvt(800), makeGS());

      config.onPanResponderGrant(makeEvt(800), makeGS());
      config.onPanResponderRelease(makeEvt(800), makeGS());

      expect(cbs.onSeekBy).toHaveBeenCalledWith(10);
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
});
