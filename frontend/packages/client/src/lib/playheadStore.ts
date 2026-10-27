import { useSyncExternalStore } from "react";

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
