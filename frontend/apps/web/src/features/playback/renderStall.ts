// renderStall measures the symptom nobody could pin down from the logs: the
// picture slows down (visibly fewer frames per second) while the sound stays
// perfectly fluid, and the decoder reports ZERO dropped frames.
//
// Those three facts together rule out the usual suspects. A decoder that
// cannot keep up DROPS frames, and getVideoPlaybackQuality() has reported
// 0/44022 on every production beacon so far, so the decode path is keeping up.
// The audio is scheduled ahead of time into the Web Audio graph, which runs on
// its own real-time thread, so it keeps playing through anything that blocks
// the page. What is left is the PRESENTATION: frames are decoded but not put
// on screen on time, which is what a blocked main thread looks like.
//
// So we measure two things and put them side by side:
//   - the frames actually PRESENTED per second (requestVideoFrameCallback),
//     which is the symptom itself rather than a proxy for it;
//   - how late a timer that asks to run every 250 ms actually runs, which is
//     main-thread lag. Long Tasks would be the direct instrument but Firefox
//     does not implement it, and Firefox is where this was reported.
//
// Nothing here changes playback. It only watches.

/** Cadence of the lag watchdog. Short enough to catch a 100 ms block, long
 *  enough to cost nothing. */
const WATCHDOG_MS = 250;
/** Length of one measurement window. One second is what "frames per second"
 *  means, and it is short enough to sit inside a slowdown episode. */
const WINDOW_MS = 1000;
/** A window is a slowdown when presented frames fall below this share of the
 *  baseline. Two thirds: a 25 fps source dipping under ~17 fps is visible,
 *  while ordinary jitter is not. */
const SLOWDOWN_RATIO = 0.66;
/** Below this the number is noise (a still frame, a paused decoder), not a
 *  measurement worth reporting. */
const MIN_BASELINE_FPS = 5;
/** Never report more often than this. The beacon route is rate limited, and
 *  one report per episode is what a diagnosis needs. */
const REPORT_COOLDOWN_MS = 30_000;

export interface RenderWindow {
  /** Frames presented per second over the last window. */
  fps: number;
  /** The best rate seen so far this session: the content's own cadence, which
   *  we cannot read from the manifest for a transcoded stream. */
  baselineFps: number;
  /** Worst lateness of the 250 ms watchdog during the window, in ms. This is
   *  the main-thread block, if there was one. */
  lagMaxMs: number;
  playing: boolean;
  visible: boolean;
}

/**
 * Pure verdict for one window, so the threshold is testable without a browser.
 *
 * Deliberately silent while paused or hidden: a background tab throttles
 * timers and stops presenting frames, which would otherwise look exactly like
 * the bug.
 */
export function isRenderSlowdown(w: RenderWindow): boolean {
  if (!w.playing || !w.visible) return false;
  if (w.baselineFps < MIN_BASELINE_FPS) return false;
  return w.fps < w.baselineFps * SLOWDOWN_RATIO;
}

/**
 * Formats a window for the beacon's `state` field, which the server clamps.
 * Kept terse for that reason: `fps=11.0/25.0 lag=340ms`.
 */
export function formatRenderWindow(w: RenderWindow): string {
  return `fps=${w.fps.toFixed(1)}/${w.baselineFps.toFixed(1)} lag=${Math.round(w.lagMaxMs)}ms`;
}

export interface RenderStallWatch {
  /** Latest window, for the shared playback snapshot. Null before the first
   *  window closes. */
  latest(): RenderWindow | null;
  stop(): void;
}

interface WatchOpts {
  onSlowdown: (w: RenderWindow) => void;
  /** Injectable for tests. */
  now?: () => number;
}

/**
 * Starts watching a video element. Returns a stop() the caller MUST call on
 * unmount: both the frame callback and the watchdog re-arm themselves.
 *
 * Falls back to lag-only measurement when requestVideoFrameCallback is absent
 * (older Safari): the fps half is then always 0, which isRenderSlowdown treats
 * as "no baseline", so nothing is reported rather than everything.
 */
export function createRenderStallWatch(video: HTMLVideoElement, opts: WatchOpts): RenderStallWatch {
  const now = opts.now ?? (() => performance.now());
  const v = video as HTMLVideoElement & {
    requestVideoFrameCallback?: (cb: () => void) => number;
    cancelVideoFrameCallback?: (h: number) => void;
  };

  let stopped = false;
  let frames = 0;
  let windowStart = now();
  let lagMax = 0;
  let baselineFps = 0;
  let lastReportAt = -Infinity;
  let latest: RenderWindow | null = null;
  let rvfcHandle: number | null = null;
  let timer: ReturnType<typeof setTimeout> | null = null;

  if (typeof v.requestVideoFrameCallback === "function") {
    const onFrame = () => {
      if (stopped) return;
      frames++;
      rvfcHandle = v.requestVideoFrameCallback!(onFrame);
    };
    rvfcHandle = v.requestVideoFrameCallback(onFrame);
  }

  let expected = now() + WATCHDOG_MS;
  const tick = () => {
    if (stopped) return;
    const t = now();
    // How late this run is compared to when it asked to run. Anything the page
    // did synchronously in the meantime shows up here.
    const lag = Math.max(0, t - expected);
    if (lag > lagMax) lagMax = lag;

    const elapsed = t - windowStart;
    if (elapsed >= WINDOW_MS) {
      const w: RenderWindow = {
        fps: (frames * 1000) / elapsed,
        baselineFps,
        lagMaxMs: lagMax,
        playing: !video.paused && !video.ended,
        visible: typeof document === "undefined" || document.visibilityState === "visible",
      };
      // The baseline is the best rate seen while playing: the content's own
      // cadence. Taken from the same measurement so a 24 fps film and a 60 fps
      // clip are each judged against themselves.
      if (w.playing && w.visible && w.fps > baselineFps) baselineFps = w.fps;
      w.baselineFps = baselineFps;
      latest = w;
      if (isRenderSlowdown(w) && t - lastReportAt >= REPORT_COOLDOWN_MS) {
        lastReportAt = t;
        opts.onSlowdown(w);
      }
      frames = 0;
      windowStart = t;
      lagMax = 0;
    }
    expected = t + WATCHDOG_MS;
    timer = setTimeout(tick, WATCHDOG_MS);
  };
  timer = setTimeout(tick, WATCHDOG_MS);

  return {
    latest: () => latest,
    stop: () => {
      stopped = true;
      if (timer) clearTimeout(timer);
      if (rvfcHandle != null && typeof v.cancelVideoFrameCallback === "function") {
        v.cancelVideoFrameCallback(rvfcHandle);
      }
    },
  };
}

/** Below this much buffered media, a slowdown is the server being late, not
 *  the page. Bounded by the player's own forward-buffer target: at 70 Mb/s the
 *  buffer is capped to 6 s by design, so a fixed 8 s would never let the cap
 *  act on the rung that needs it most (prod 2026-09-11, Firefox on 2160p). */
const CAP_MIN_BUFFER_AHEAD_SEC = 8;
const CAP_MIN_BUFFER_SHARE = 0.75;

export function capMinBufferSec(bufferTargetSec: number): number {
  if (!Number.isFinite(bufferTargetSec) || bufferTargetSec <= 0) return CAP_MIN_BUFFER_AHEAD_SEC;
  return Math.min(CAP_MIN_BUFFER_AHEAD_SEC, bufferTargetSec * CAP_MIN_BUFFER_SHARE);
}
/** Slowdown reports (30 s apart) before a rung is given up. */
const CAP_AFTER_REPORTS = 2;
/** Reports further apart than this start a new streak. */
const CAP_STREAK_WINDOW_MS = 120_000;

export interface RenderCapState {
  streak: number;
  lastAt: number;
}

/** Seconds of media buffered past the playhead, 0 outside any range. */
export function bufferedAheadSec(video: HTMLVideoElement): number {
  try {
    const ct = video.currentTime;
    const b = video.buffered;
    for (let i = 0; i < b.length; i++) {
      if (b.start(i) <= ct + 0.05 && ct <= b.end(i)) return b.end(i) - ct;
    }
  } catch { /* detached element */ }
  return 0;
}

export function nextLowerRung(levelHeights: readonly number[], currentHeight: number): number | null {
  const lower = levelHeights.filter((h) => h > 0 && h < currentHeight);
  return lower.length ? Math.max(...lower) : null;
}

// A page that keeps under-painting a rung it has fully buffered cannot show
// it (prod 2026-09-10: 2160p decoded at 24 fps, painted at 12, drop=0 on two
// browsers). Returns the rung to pin instead, or null to keep watching.
export function decideRenderCap(
  s: RenderCapState,
  now: number,
  bufferAheadSec: number,
  levelHeights: readonly number[],
  currentHeight: number,
  bufferTargetSec = CAP_MIN_BUFFER_AHEAD_SEC,
): number | null {
  if (bufferAheadSec < capMinBufferSec(bufferTargetSec)) {
    s.streak = 0;
    return null;
  }
  s.streak = now - s.lastAt <= CAP_STREAK_WINDOW_MS ? s.streak + 1 : 1;
  s.lastAt = now;
  if (s.streak < CAP_AFTER_REPORTS) return null;
  s.streak = 0;
  return nextLowerRung(levelHeights, currentHeight);
}
