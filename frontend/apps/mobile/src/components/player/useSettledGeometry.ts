import { useEffect, useRef, useState } from "react";

export interface EdgeInsets {
  top: number;
  right: number;
  bottom: number;
  left: number;
}

export interface PlayerGeometry {
  width: number;
  height: number;
  insets: EdgeInsets;
}

// Window size and safe-area insets are two INDEPENDENT native events. Going
// fullscreen fires both (orientation lock, then system bars), and they reach JS
// one or more frames apart, so the control bar was laid out two or three times
// in a row - new width first (the volume slider pops in at its breakpoint), new
// insets after (every edge padding shifts). That is the placement "computed in
// two passes" the viewer sees. Coalescing window: long enough to swallow the
// gap between the two events, short enough not to feel like a freeze.
export const GEOMETRY_SETTLE_MS = 120;
// Ceiling on the whole hold, so a device that keeps emitting geometry events
// can never keep the controls hidden indefinitely.
export const GEOMETRY_MAX_HOLD_MS = 600;

function sameInsets(a: EdgeInsets, b: EdgeInsets): boolean {
  return a.top === b.top && a.right === b.right && a.bottom === b.bottom && a.left === b.left;
}

/**
 * Publishes the player's geometry only once the native events have stopped
 * arriving, and tells the caller when a RESIZE is still in flight.
 *
 * Only a size change flags `resizing`: that is the one the controls cannot be
 * painted through (wrong scale, wrong breakpoint). An inset-only change - the
 * system bars appearing when the controls come up - leaves them painted where
 * they are until the new value is published, so tapping to reveal the bar never
 * blinks.
 */
export function useSettledGeometry(
  raw: PlayerGeometry,
  settleMs: number = GEOMETRY_SETTLE_MS,
  maxHoldMs: number = GEOMETRY_MAX_HOLD_MS,
): { geometry: PlayerGeometry; resizing: boolean } {
  const [settled, setSettled] = useState<PlayerGeometry>(raw);
  const holdStartedAt = useRef<number | null>(null);

  const { width, height, insets } = raw;
  const resizing = width !== settled.width || height !== settled.height;
  const stable = !resizing && sameInsets(insets, settled.insets);

  useEffect(() => {
    if (stable) {
      holdStartedAt.current = null;
      return;
    }
    const now = Date.now();
    if (holdStartedAt.current === null) holdStartedAt.current = now;
    const remaining = maxHoldMs - (now - holdStartedAt.current);
    const id = setTimeout(() => {
      holdStartedAt.current = null;
      setSettled({ width, height, insets: { top: insets.top, right: insets.right, bottom: insets.bottom, left: insets.left } });
    }, Math.max(0, Math.min(settleMs, remaining)));
    return () => clearTimeout(id);
  }, [stable, width, height, insets.top, insets.right, insets.bottom, insets.left, settleMs, maxHoldMs]);

  return { geometry: settled, resizing };
}
