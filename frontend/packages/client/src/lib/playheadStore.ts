import { useCallback, useRef, useSyncExternalStore } from "react";

// The playhead, twice a second, outside React state: committed to a player
// screen's state it re-rendered the whole screen on every tick. The parts
// that need it finely (a subtitle overlay, a timeline) subscribe here; the
// screen itself keeps a once-a-second copy.

let playhead = 0;
const listeners = new Set<() => void>();

export function setPlayhead(sec: number): void {
  if (sec === playhead) return;
  playhead = sec;
  for (const l of listeners) l();
}

export function getPlayhead(): number {
  return playhead;
}

function subscribe(l: () => void): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

export function usePlayhead(): number {
  return useSyncExternalStore(subscribe, getPlayhead, getPlayhead);
}

/**
 * Subscribe to a value DERIVED from the playhead, re-rendering only when that
 * value changes (by Object.is), not on every twice-a-second tick. A subtitle
 * overlay follows the active cue, a skip button whether the playhead is in a
 * segment: both are constant for seconds or minutes at a time, so a selector
 * that returns a cue index or a quantised second lets React bail the render
 * while it holds. The selector must return a primitive (or a stable reference);
 * a fresh object every call would defeat the bail-out and churn as before.
 */
export function usePlayheadSelector<T>(selector: (sec: number) => T): T {
  const selectorRef = useRef(selector);
  selectorRef.current = selector;
  const getSnapshot = useCallback(() => selectorRef.current(getPlayhead()), []);
  return useSyncExternalStore(subscribe, getSnapshot, getSnapshot);
}
