import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

vi.mock("react-native", () => ({
  Platform: { OS: "android", select: (obj: Record<string, unknown>) => obj.android },
  Dimensions: { get: () => ({ width: 1000, height: 500 }) },
  PanResponder: {
    create: (config: any) => ({ panHandlers: {}, _config: config }),
  },
}));

import { renderHook, act } from "@testing-library/react";
import { usePlayerGestures, scrubSpanSeconds } from "./usePlayerGestures";

function setup(extra: Partial<Record<string, unknown>> = {}) {
  const cbs = {
    onSeekBy: vi.fn(),
    onTogglePlayPause: vi.fn(),
    onToggleControls: vi.fn(),
    onVolumeChange: vi.fn(),
    onBrightnessChange: vi.fn(),
    getVolume: vi.fn(() => 0.5),
    getBrightness: vi.fn(() => 0.5),
    ...extra,
  } as any;
  const { result } = renderHook(() => usePlayerGestures(cbs));
  // The responder handlers set hook state (indicators, scrub preview): run
  // each one inside act() so the tests drive them the way a real gesture
  // would and React does not warn about a stray update.
  const raw = (result.current.panResponder as any)._config;
  const config: any = {};
  for (const key of Object.keys(raw)) {
    const fn = raw[key];
    config[key] = typeof fn === "function"
      ? (...args: unknown[]) => { let out: unknown; act(() => { out = fn(...args); }); return out; }
      : fn;
  }
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

// --- horizontal scrub (timeline drag) -------------------------
// The screen is mocked at 1000x500, so one full-width drag = 1000px.

function scrubSetup(durationSec: number, positionSec = 0) {
  return setup({
    onScrubCommit: vi.fn(),
    getPosition: vi.fn(() => positionSec),
    getDuration: vi.fn(() => durationSec),
  });
}

describe("horizontal scrub", () => {
  it("spans a tenth of the runtime per full-width drag, clamped to 60..600s", () => {
    expect(scrubSpanSeconds(1200)).toBe(120);   // 20 min episode
    expect(scrubSpanSeconds(120)).toBe(60);     // short clip: floor
    expect(scrubSpanSeconds(10800)).toBe(600);  // 3 h film: ceiling
  });

  it("previews while dragging and seeks once, on release", () => {
    const { config, cbs, result } = scrubSetup(1200, 300); // span = 120s

    config.onPanResponderGrant(makeEvt(500), makeGS());
    act(() => {
      config.onPanResponderMove(makeEvt(750), makeGS({ dx: 250, dy: 4 }));
    });

    // A quarter of the width = a quarter of the 120s span = +30s.
    expect(result.current.scrubPreview).toEqual({
      targetSec: 330,
      deltaSec: 30,
      durationSec: 1200,
    });
    // Nothing has been committed yet: a drag must not hammer the HLS session.
    expect(cbs.onScrubCommit).not.toHaveBeenCalled();

    act(() => {
      config.onPanResponderRelease(makeEvt(750), makeGS({ dx: 250, dy: 4 }));
    });
    expect(cbs.onScrubCommit).toHaveBeenCalledExactlyOnceWith(330);
    expect(result.current.scrubPreview).toBeNull();
  });

  it("drags backwards and clamps at both ends of the timeline", () => {
    const { config, cbs } = scrubSetup(1200, 30); // span = 120s

    config.onPanResponderGrant(makeEvt(500), makeGS());
    act(() => {
      config.onPanResponderMove(makeEvt(100), makeGS({ dx: -400, dy: 0 }));
    });
    act(() => {
      config.onPanResponderRelease(makeEvt(100), makeGS({ dx: -400, dy: 0 }));
    });
    // 30 - 48 would be negative: the timeline starts at 0.
    expect(cbs.onScrubCommit).toHaveBeenCalledExactlyOnceWith(0);
  });

  // The three gestures share one surface; a mostly-sideways drag must not be
  // read as volume just because it started on the right half.
  it("wins over volume/brightness on a mostly-horizontal drag", () => {
    const { config, cbs } = scrubSetup(1200, 300);

    config.onPanResponderGrant(makeEvt(900), makeGS()); // right half = volume zone
    act(() => {
      config.onPanResponderMove(makeEvt(1000), makeGS({ dx: 100, dy: 20 }));
    });
    expect(cbs.onVolumeChange).not.toHaveBeenCalled();

    act(() => {
      config.onPanResponderRelease(makeEvt(1000), makeGS({ dx: 100, dy: 20 }));
    });
    expect(cbs.onScrubCommit).toHaveBeenCalled();
  });

  it("leaves a mostly-vertical drag to volume", () => {
    const { config, cbs } = scrubSetup(1200, 300);

    config.onPanResponderGrant(makeEvt(900), makeGS());
    act(() => {
      // First move classifies the gesture, the second applies it - the
      // volume/brightness path has always worked that way.
      config.onPanResponderMove(makeEvt(900), makeGS({ dx: 10, dy: -100 }));
      config.onPanResponderMove(makeEvt(900), makeGS({ dx: 10, dy: -140 }));
    });
    expect(cbs.onVolumeChange).toHaveBeenCalled();
    expect(cbs.onScrubCommit).not.toHaveBeenCalled();
  });

  // No runtime = nothing to scrub along (live, or metadata not in yet).
  it("does not scrub when the duration is unknown", () => {
    const { config, cbs, result } = scrubSetup(0, 0);

    config.onPanResponderGrant(makeEvt(500), makeGS());
    act(() => {
      config.onPanResponderMove(makeEvt(750), makeGS({ dx: 250, dy: 0 }));
    });
    expect(result.current.scrubPreview).toBeNull();
    act(() => {
      config.onPanResponderRelease(makeEvt(750), makeGS({ dx: 250, dy: 0 }));
    });
    expect(cbs.onScrubCommit).not.toHaveBeenCalled();
  });

  it("commits nothing and clears the overlay when the gesture is terminated", () => {
    const { config, cbs, result } = scrubSetup(1200, 300);

    config.onPanResponderGrant(makeEvt(500), makeGS());
    act(() => {
      config.onPanResponderMove(makeEvt(750), makeGS({ dx: 250, dy: 0 }));
    });
    act(() => { config.onPanResponderTerminate(); });

    expect(result.current.scrubPreview).toBeNull();
    expect(cbs.onScrubCommit).not.toHaveBeenCalled();
  });

  // A double tap that drifts sideways is still a double tap.
  it("leaves a small sideways drift to the tap path", () => {
    const { config, cbs } = scrubSetup(1200, 300);

    config.onPanResponderGrant(makeEvt(100), makeGS());
    config.onPanResponderRelease(makeEvt(100), makeGS({ dx: 6, dy: 2 }));
    config.onPanResponderGrant(makeEvt(100), makeGS());
    config.onPanResponderRelease(makeEvt(100), makeGS({ dx: 6, dy: 2 }));

    expect(cbs.onSeekBy).toHaveBeenCalledWith(-30);
    expect(cbs.onScrubCommit).not.toHaveBeenCalled();
  });
});
