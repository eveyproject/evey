/**
 * useTouchGestures - unit tests
 *
 * Strategy: the hook exposes handleTouchStart/Move/End as plain
 * React callbacks.  We call them directly with shaped plain-objects that
 * satisfy the property accesses in the source.  No jsdom TouchEvent dispatch
 * is needed; the callbacks never call addEventListener on any element.
 *
 * What we DO NOT test:
 *  - setTimeout-driven seekFlash clearing / singleTap timer firing (those
 *    require fake-timer coordination across two separate act() calls; brittle
 *    for minimal gain - the timer logic is a one-liner wrapping setSeekFlash).
 */
import { renderHook, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { useTouchGestures } from "./useTouchGestures";

// ─── helpers ────────────────────────────────────────────────────────────────

function createVideoElement() {
  const el = document.createElement("video");
  Object.defineProperty(el, "volume", { value: 0.5, writable: true, configurable: true });
  Object.defineProperty(el, "muted", { value: false, writable: true, configurable: true });
  return el;
}

/** Build a minimal element whose getBoundingClientRect reports a 400×300 box. */
function makeCurrentTarget(left = 0, width = 400): Partial<HTMLElement> {
  return {
    getBoundingClientRect: () =>
      ({ left, width, top: 0, height: 300, right: left + width, bottom: 300 }) as DOMRect,
    closest: (_selector: string) => null,
  } as Partial<HTMLElement>;
}

type TouchPoint = { clientX: number; clientY: number };

function makeTouch(p: TouchPoint): Touch {
  return p as unknown as Touch;
}

function makeTouchEvent(
  type: "start" | "move" | "end",
  touch: TouchPoint,
  overrides: Partial<{
    target: Partial<HTMLElement>;
    currentTarget: Partial<HTMLElement>;
    changedTouches: Touch[];
  }> = {},
): React.TouchEvent {
  const t = makeTouch(touch);
  const defaultTarget: Partial<HTMLElement> = { closest: () => null };
  return {
    touches: type !== "end" ? [t] : [],
    changedTouches: overrides.changedTouches ?? [t],
    target: overrides.target ?? defaultTarget,
    currentTarget: overrides.currentTarget ?? makeCurrentTarget(),
    preventDefault: vi.fn(),
  } as unknown as React.TouchEvent;
}

function defaultArgs(video: HTMLVideoElement) {
  return {
    videoRef: { current: video },
    positionRef: { current: 60 },
    knownDurationSecRef: { current: 120 },
    seekRef: { current: vi.fn() },
    skipFwdRef: { current: 10 },
    skipBwdRef: { current: 10 },
    onToggleFullscreen: vi.fn(),
    onSingleTap: vi.fn(),
  };
}

// ─── tests ───────────────────────────────────────────────────────────────────

describe("useTouchGestures", () => {
  let video: HTMLVideoElement;

  beforeEach(() => {
    video = createVideoElement();
    localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ── brightness init ───────────────────────────────────────────────────────

  it("initialises brightness to 1 when localStorage is empty", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));
    expect(result.current.brightness).toBe(1);
  });

  it("loads persisted brightness from localStorage", () => {
    localStorage.setItem("evey-player-brightness", "1.3");
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));
    expect(result.current.brightness).toBeCloseTo(1.3);
  });

  it("clamps brightness from localStorage to [0.2, 1.5]", () => {
    localStorage.setItem("evey-player-brightness", "9");
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));
    expect(result.current.brightness).toBe(1.5);
  });

  // ── setBrightnessValue ────────────────────────────────────────────────────

  it("setBrightnessValue updates brightness state and persists to localStorage", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    act(() => {
      result.current.setBrightnessValue(1.2);
    });

    expect(result.current.brightness).toBeCloseTo(1.2);
    expect(localStorage.getItem("evey-player-brightness")).toBe("1.2");
  });

  it("setBrightnessValue clamps below MIN_BRIGHTNESS (0.2)", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    act(() => {
      result.current.setBrightnessValue(0.0);
    });

    expect(result.current.brightness).toBe(0.2);
  });

  it("setBrightnessValue clamps above MAX_BRIGHTNESS (1.5)", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    act(() => {
      result.current.setBrightnessValue(2.5);
    });

    expect(result.current.brightness).toBe(1.5);
  });

  // handleWheel moved to PlayerContext (33040c337: wheel spans 0-200% via the
  // boost gain, registered passive:false on the container) - covered there.


  // ── handleTouchStart: control target guard ────────────────────────────────

  it("handleTouchStart ignores touches on button elements", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    const buttonTarget: Partial<HTMLElement> = {
      closest: (sel: string) => (sel.includes("button") ? ({} as Element) : null),
    };

    act(() => {
      result.current.handleTouchStart(
        makeTouchEvent("start", { clientX: 200, clientY: 150 }, { target: buttonTarget }),
      );
    });

    // After a guarded start, a tap-end should not fire onSingleTap.
    // We'll confirm by checking that a subsequent End without a valid start is a no-op.
    act(() => {
      result.current.handleTouchEnd(
        makeTouchEvent("end", { clientX: 201, clientY: 151 }, { target: buttonTarget }),
      );
    });

    expect(args.onSingleTap).not.toHaveBeenCalled();
  });

  // ── tap sequence: single tap calls onSingleTap (via timer) ───────────────

  it("single tap queues onSingleTap via a 300 ms timer", () => {
    vi.useFakeTimers();
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 200, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 200, clientY: 150 }, { currentTarget: ct }));
    });
    // Timer not fired yet
    expect(args.onSingleTap).not.toHaveBeenCalled();

    act(() => { vi.runAllTimers(); });

    expect(args.onSingleTap).toHaveBeenCalledTimes(1);
    vi.useRealTimers();
  });

  // ── double tap: center zone → fullscreen ─────────────────────────────────

  it("double tap in center zone calls onToggleFullscreen", () => {
    vi.useFakeTimers();
    const now = Date.now();
    vi.setSystemTime(now);

    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    const ct = makeCurrentTarget(0, 400);
    // center X = 200, relX = 200/400 = 0.5 → center zone

    // First tap
    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 200, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 200, clientY: 150 }, { currentTarget: ct }));
    });

    // Second tap within 400ms
    vi.setSystemTime(now + 200);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 200, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 200, clientY: 150 }, { currentTarget: ct }));
    });

    expect(args.onToggleFullscreen).toHaveBeenCalledTimes(1);
    expect(args.onSingleTap).not.toHaveBeenCalled();
    vi.useRealTimers();
  });

  // ── double tap: left zone → seek backward ────────────────────────────────

  it("double tap on the left zone seeks backward", () => {
    vi.useFakeTimers();
    const now = Date.now();
    vi.setSystemTime(now);

    const args = defaultArgs(video);
    // position=60, skipBwd=10 → expect seek(50)
    const { result } = renderHook(() => useTouchGestures(args));

    // ct: width=400, left X=80 → relX=0.2 (<0.35, left zone)
    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 80, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 80, clientY: 150 }, { currentTarget: ct }));
    });

    vi.setSystemTime(now + 200);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 80, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 80, clientY: 150 }, { currentTarget: ct }));
    });

    expect(args.seekRef.current).toHaveBeenCalledWith(50); // 60 - 10
    expect(result.current.seekFlash).toBe("left");
    vi.useRealTimers();
  });

  // ── double tap: right zone → seek forward ────────────────────────────────

  it("double tap on the right zone seeks forward", () => {
    vi.useFakeTimers();
    const now = Date.now();
    vi.setSystemTime(now);

    const args = defaultArgs(video);
    // position=60, skipFwd=10, knownDuration=120 → expect seek(70)
    const { result } = renderHook(() => useTouchGestures(args));

    // right zone: X=340, relX=0.85 (>0.65)
    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 340, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 340, clientY: 150 }, { currentTarget: ct }));
    });

    vi.setSystemTime(now + 200);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 340, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 340, clientY: 150 }, { currentTarget: ct }));
    });

    expect(args.seekRef.current).toHaveBeenCalledWith(70); // 60 + 10
    expect(result.current.seekFlash).toBe("right");
    vi.useRealTimers();
  });

  it("forward seek is clamped to knownDuration", () => {
    vi.useFakeTimers();
    const now = Date.now();
    vi.setSystemTime(now);

    const args = defaultArgs(video);
    args.positionRef.current = 118;   // 118 + 10 = 128 > 120
    args.knownDurationSecRef.current = 120;

    const { result } = renderHook(() => useTouchGestures(args));
    const ct = makeCurrentTarget(0, 400);

    // Two right-zone taps
    const doTap = (t: number) => {
      vi.setSystemTime(t);
      act(() => {
        result.current.handleTouchStart(makeTouchEvent("start", { clientX: 340, clientY: 150 }));
      });
      act(() => {
        result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 340, clientY: 150 }, { currentTarget: ct }));
      });
    };

    doTap(now);
    doTap(now + 150);

    expect(args.seekRef.current).toHaveBeenCalledWith(120); // clamped
    vi.useRealTimers();
  });

  it("backward seek is clamped to 0", () => {
    vi.useFakeTimers();
    const now = Date.now();
    vi.setSystemTime(now);

    const args = defaultArgs(video);
    args.positionRef.current = 3; // 3 - 10 = -7 → should be 0

    const { result } = renderHook(() => useTouchGestures(args));
    const ct = makeCurrentTarget(0, 400);

    const doTap = (t: number) => {
      vi.setSystemTime(t);
      act(() => {
        result.current.handleTouchStart(makeTouchEvent("start", { clientX: 80, clientY: 150 }));
      });
      act(() => {
        result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 80, clientY: 150 }, { currentTarget: ct }));
      });
    };

    doTap(now);
    doTap(now + 150);

    expect(args.seekRef.current).toHaveBeenCalledWith(0);
    vi.useRealTimers();
  });

  // ── double tap: too far apart in time → two single taps ──────────────────

  it("two taps more than 400 ms apart are treated as separate single taps", () => {
    vi.useFakeTimers();
    const now = Date.now();
    vi.setSystemTime(now);

    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));
    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 200, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 200, clientY: 150 }, { currentTarget: ct }));
    });

    // Advance past the 400 ms double-tap window
    vi.setSystemTime(now + 500);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 200, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchEnd(makeTouchEvent("end", { clientX: 200, clientY: 150 }, { currentTarget: ct }));
    });

    expect(args.onToggleFullscreen).not.toHaveBeenCalled();
    expect(args.seekRef.current).not.toHaveBeenCalled();
    vi.useRealTimers();
  });

  // ── touchEnd: long-press (>300 ms) is not a tap ──────────────────────────

  it("touch lasting longer than 300 ms is not treated as a tap", () => {
    vi.useFakeTimers();
    const now = Date.now();
    vi.setSystemTime(now);

    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 200, clientY: 150 }));
    });

    // Advance time before ending to simulate a long press
    vi.setSystemTime(now + 400);

    act(() => {
      result.current.handleTouchEnd(
        makeTouchEvent("end", { clientX: 200, clientY: 150 }, { currentTarget: makeCurrentTarget() }),
      );
    });

    act(() => { vi.runAllTimers(); });

    expect(args.onSingleTap).not.toHaveBeenCalled();
    vi.useRealTimers();
  });

  // ── vertical swipe: right side → volume ──────────────────────────────────

  it("vertical swipe up on the right side increases video volume", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    // ct: width=400; X=320 → relX=0.8 (>0.6, right=volume)
    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 320, clientY: 200 }));
    });

    // Swipe up 50px (dy positive)
    act(() => {
      result.current.handleTouchMove({
        touches: [makeTouch({ clientX: 320, clientY: 150 })], // 200-150=50 up
        currentTarget: ct,
        preventDefault: vi.fn(),
      } as unknown as React.TouchEvent);
    });

    // volume should have increased: 0.5 + 50 * 0.004 = 0.7
    expect(video.volume).toBeCloseTo(0.7, 2);
    expect(result.current.swipeIndicator?.type).toBe("volume");
    expect(result.current.swipeIndicator?.value).toBeCloseTo(0.7, 2);
  });

  it("vertical swipe down on the right side decreases video volume", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 320, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchMove({
        touches: [makeTouch({ clientX: 320, clientY: 200 })], // 50 px down → dy=-50
        currentTarget: ct,
        preventDefault: vi.fn(),
      } as unknown as React.TouchEvent);
    });

    // volume: 0.5 + (-50)*0.004 = 0.3
    expect(video.volume).toBeCloseTo(0.3, 2);
  });

  it("volume swipe clears muted when new volume > 0", () => {
    Object.defineProperty(video, "muted", { value: true, writable: true, configurable: true });
    Object.defineProperty(video, "volume", { value: 0, writable: true, configurable: true });

    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 320, clientY: 200 }));
    });
    act(() => {
      result.current.handleTouchMove({
        touches: [makeTouch({ clientX: 320, clientY: 150 })],
        currentTarget: ct,
        preventDefault: vi.fn(),
      } as unknown as React.TouchEvent);
    });

    expect(video.muted).toBe(false);
  });

  // ── vertical swipe: left side → brightness ───────────────────────────────

  it("vertical swipe up on the left side increases brightness", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    const ct = makeCurrentTarget(0, 400);
    // X=80 → relX=0.2 (<0.4, left=brightness)

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 80, clientY: 200 }));
    });
    act(() => {
      result.current.handleTouchMove({
        touches: [makeTouch({ clientX: 80, clientY: 150 })], // 50px up
        currentTarget: ct,
        preventDefault: vi.fn(),
      } as unknown as React.TouchEvent);
    });

    // brightness: 1 + 50*0.004 = 1.2
    expect(result.current.brightness).toBeCloseTo(1.2, 2);
    expect(result.current.swipeIndicator?.type).toBe("brightness");
  });

  // ── swipe end: finalises + hides indicator ────────────────────────────────

  it("touchEnd after volume swipe clears swipeIndicator", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 320, clientY: 200 }));
    });
    act(() => {
      result.current.handleTouchMove({
        touches: [makeTouch({ clientX: 320, clientY: 150 })],
        currentTarget: ct,
        preventDefault: vi.fn(),
      } as unknown as React.TouchEvent);
    });
    act(() => {
      result.current.handleTouchEnd(
        makeTouchEvent("end", { clientX: 320, clientY: 150 }, { currentTarget: ct }),
      );
    });

    expect(result.current.swipeIndicator).toBeNull();
  });

  it("touchEnd after brightness swipe persists value to localStorage", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 80, clientY: 200 }));
    });
    act(() => {
      result.current.handleTouchMove({
        touches: [makeTouch({ clientX: 80, clientY: 150 })], // +50px up
        currentTarget: ct,
        preventDefault: vi.fn(),
      } as unknown as React.TouchEvent);
    });
    act(() => {
      result.current.handleTouchEnd(
        makeTouchEvent("end", { clientX: 80, clientY: 150 }, { currentTarget: ct }),
      );
    });

    const stored = parseFloat(localStorage.getItem("evey-player-brightness") ?? "0");
    expect(stored).toBeCloseTo(1.2, 2);
  });

  // ── middle zone swipe: ignored ────────────────────────────────────────────

  it("vertical swipe in the middle zone (relX 0.4-0.6) is ignored", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    // X=200, width=400 → relX=0.5, middle zone
    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 200, clientY: 200 }));
    });
    act(() => {
      result.current.handleTouchMove({
        touches: [makeTouch({ clientX: 200, clientY: 150 })],
        currentTarget: ct,
        preventDefault: vi.fn(),
      } as unknown as React.TouchEvent);
    });

    expect(result.current.swipeIndicator).toBeNull();
    expect(result.current.brightness).toBe(1); // unchanged
    expect(video.volume).toBe(0.5); // unchanged
  });

  // ── horizontal swipe is ignored ──────────────────────────────────────────

  it("horizontal swipe (|dx| > |dy|) does not trigger volume/brightness", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    const ct = makeCurrentTarget(0, 400);

    act(() => {
      result.current.handleTouchStart(makeTouchEvent("start", { clientX: 100, clientY: 150 }));
    });
    act(() => {
      result.current.handleTouchMove({
        touches: [makeTouch({ clientX: 200, clientY: 130 })], // dx=100, dy=20 → horizontal
        currentTarget: ct,
        preventDefault: vi.fn(),
      } as unknown as React.TouchEvent);
    });

    expect(result.current.swipeIndicator).toBeNull();
  });

  // ── cleanupTimers ─────────────────────────────────────────────────────────

  it("cleanupTimers does not throw", () => {
    const args = defaultArgs(video);
    const { result } = renderHook(() => useTouchGestures(args));

    expect(() => {
      act(() => { result.current.cleanupTimers(); });
    }).not.toThrow();
  });
});
