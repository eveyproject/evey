import { useCallback, useEffect, useRef, useState } from "react";
import {
  clampSeekTarget,
  holdScrubRate,
  HOLD_SCRUB_TICK_MS,
  SEEK_COMMIT_DELAY_MS,
  shouldReleaseSeekPreview,
  tapSeekStep,
} from "../lib/seekGesture";

/**
 * A seek driven by a remote, the same on every television player (the
 * Android TV app, the browser client).
 *
 * Presses move a PREVIEW at once (the bar and the clock follow it) and ONE
 * real seek lands once the pressing stops: a seek per tap restarted the
 * stream per tap and made the timeline lurch. Taps in one gesture take a
 * growing step (tapSeekStep); a held key scrubs at a rate that climbs with
 * the hold (holdScrubRate) and commits on release. The preview is dropped
 * once the real playhead has caught up with it, not at the commit, so the bar
 * never jumps back to the old position while the player seeks.
 */
const RELEASE_CHECK_MS = 500;

export function useRemoteSeek({
  positionSec,
  getPosition,
  getDuration,
  seekTo,
  onActivity,
  onPreviewChange,
}: {
  /** The player's playhead, as rendered: drives the release of the preview. */
  positionSec: number;
  /** The playhead right now (a ref read: a hold tick outruns the renders). */
  getPosition: () => number;
  getDuration: () => number;
  /** The one real seek, in seconds. */
  seekTo: (sec: number) => void;
  /** Every press: keeps the controls up. */
  onActivity?: () => void;
  /**
   * Given, the preview goes here instead of this hook's own state, so a host
   * that only forwards it is not re-rendered on every press.
   */
  onPreviewChange?: (sec: number | null) => void;
}) {
  const [previewSec, setPreviewSec] = useState<number | null>(null);
  const previewRef = useRef<number | null>(null);
  const tapCountRef = useRef(0);
  const commitTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const holdRef = useRef<{ dir: 1 | -1; startedAt: number; lastAt: number; moved: boolean; timer: ReturnType<typeof setInterval> } | null>(null);
  // Read by the timers: the latest callbacks, not the ones of the render that armed them.
  const cb = useRef({ getPosition, getDuration, seekTo, onActivity, onPreviewChange });
  cb.current = { getPosition, getDuration, seekTo, onActivity, onPreviewChange };

  const external = onPreviewChange !== undefined;
  const setPreview = useCallback((sec: number | null) => {
    previewRef.current = sec;
    if (external) cb.current.onPreviewChange?.(sec);
    else setPreviewSec(sec);
  }, [external]);

  const land = useCallback(() => {
    commitTimerRef.current = null;
    tapCountRef.current = 0;
    const target = previewRef.current;
    if (target !== null) cb.current.seekTo(target);
  }, []);

  const scheduleCommit = useCallback(() => {
    if (commitTimerRef.current) clearTimeout(commitTimerRef.current);
    commitTimerRef.current = setTimeout(land, SEEK_COMMIT_DELAY_MS);
  }, [land]);

  /** A tap (or an on-screen skip button): move by `deltaSec`, growing with the taps of the gesture. */
  const bump = useCallback((deltaSec: number) => {
    const base = previewRef.current ?? cb.current.getPosition();
    tapCountRef.current += 1;
    const step = Math.sign(deltaSec) * tapSeekStep(tapCountRef.current, Math.abs(deltaSec));
    setPreview(clampSeekTarget(base, step, cb.current.getDuration()));
    scheduleCommit();
    cb.current.onActivity?.();
  }, [setPreview, scheduleCommit]);

  const stopHold = useCallback(() => {
    if (holdRef.current) clearInterval(holdRef.current.timer);
    holdRef.current = null;
  }, []);

  /** A key held down: scrub until endHold. */
  const beginHold = useCallback((dir: 1 | -1) => {
    if (holdRef.current) return;
    // No commit may land in the middle of the scrub.
    if (commitTimerRef.current) { clearTimeout(commitTimerRef.current); commitTimerRef.current = null; }
    const now = Date.now();
    const timer = setInterval(() => {
      const hold = holdRef.current;
      if (!hold) return;
      const t = Date.now();
      const dt = (t - hold.lastAt) / 1000;
      hold.lastAt = t;
      hold.moved = true;
      const base = previewRef.current ?? cb.current.getPosition();
      setPreview(clampSeekTarget(base, hold.dir * holdScrubRate(t - hold.startedAt) * dt, cb.current.getDuration()));
      cb.current.onActivity?.();
    }, HOLD_SCRUB_TICK_MS);
    holdRef.current = { dir, startedAt: now, lastAt: now, moved: false, timer };
  }, [setPreview]);

  /**
   * The held key is released: the scrub commits. A hold released before its
   * first tick was a tap, worth one `tapSec` skip when given.
   */
  const endHold = useCallback((tapSec?: number) => {
    const hold = holdRef.current;
    stopHold();
    if (hold?.moved) { scheduleCommit(); cb.current.onActivity?.(); return; }
    if (hold && tapSec) bump(hold.dir * tapSec);
  }, [stopHold, scheduleCommit, bump]);

  /** Land the pending seek now (OK on the timeline). False when nothing was pending. */
  const commitNow = useCallback((): boolean => {
    if (!commitTimerRef.current) return false;
    clearTimeout(commitTimerRef.current);
    land();
    cb.current.onActivity?.();
    return true;
  }, [land]);

  const isHolding = useCallback(() => holdRef.current !== null, []);

  // The real playhead caught up: the bar follows the player again. Checked
  // on the host's position when it changes, and on a slow clock against the
  // live one, for a host that commits its position rarely.
  const releaseIfCaughtUp = useCallback((position: number) => {
    const preview = previewRef.current;
    if (preview === null || commitTimerRef.current || holdRef.current) return;
    if (shouldReleaseSeekPreview(position, preview)) setPreview(null);
  }, [setPreview]);
  useEffect(() => { releaseIfCaughtUp(positionSec); }, [positionSec, releaseIfCaughtUp]);
  useEffect(() => {
    const id = setInterval(() => { if (previewRef.current !== null) releaseIfCaughtUp(cb.current.getPosition()); }, RELEASE_CHECK_MS);
    return () => clearInterval(id);
  }, [releaseIfCaughtUp]);

  useEffect(() => () => {
    stopHold();
    if (commitTimerRef.current) clearTimeout(commitTimerRef.current);
  }, [stopHold]);

  return { previewSec, previewRef, bump, beginHold, endHold, commitNow, isHolding };
}
