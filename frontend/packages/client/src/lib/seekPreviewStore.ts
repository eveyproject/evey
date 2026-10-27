import { useSyncExternalStore } from "react";

// The seek the remote is building up, outside React state: held by the
// player screen, every press re-rendered the whole screen; the bar and the
// seek badge subscribe here instead. Null when nothing is pending.

let preview: number | null = null;
const listeners = new Set<() => void>();

export function setSeekPreview(sec: number | null): void {
  if (sec === preview) return;
  preview = sec;
  for (const l of listeners) l();
}

export function getSeekPreview(): number | null {
  return preview;
}

function subscribe(l: () => void): () => void {
  listeners.add(l);
  return () => {
    listeners.delete(l);
  };
}

export function useSeekPreview(): number | null {
  return useSyncExternalStore(subscribe, getSeekPreview, getSeekPreview);
}
