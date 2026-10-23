import { useSyncExternalStore } from "react";

// "Still watching?": the player stops moving on to the next episode by itself
// once it has done so `limit` times in a row with nobody touching anything. A
// viewer asleep in front of a series otherwise woke up episodes further on,
// those episodes marked watched and "Continue watching" moved past them; on a
// transcode an encoder also ran for nobody. The card of the next episode stays
// up with its button: the next touch, key or click resumes the chain.
//
// One count for the app, not per player: a next episode is often a new player.

/** Profile preference: how many episodes may follow on their own; "0" = never ask. */
export const STILL_WATCHING_PREF_KEY = "still_watching_after";
export const STILL_WATCHING_DEFAULT = 3;
/** The choices the settings offer; 0 = never ask, 1 = after every episode. */
export const STILL_WATCHING_STEPS = [0, 1, 2, 3, 5] as const;
/** How long the card waits for an answer before the player leaves. */
export const STILL_WATCHING_EXIT_MS = 5 * 60_000;

let autoAdvances = 0;
let limit = STILL_WATCHING_DEFAULT;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

/** The profile's threshold, from its preferences (absent or invalid = the default). */
export function stillWatchingLimit(prefs: Record<string, string> | undefined): number {
  const raw = prefs?.[STILL_WATCHING_PREF_KEY];
  if (raw === undefined || raw === "") return STILL_WATCHING_DEFAULT;
  const n = Number.parseInt(raw, 10);
  return Number.isFinite(n) && n >= 0 ? n : STILL_WATCHING_DEFAULT;
}

/** The preferences map to save, merged like every other preference write. */
export function stillWatchingPreferences(current: Record<string, string> | undefined, after: number): Record<string, string> {
  return { ...current, [STILL_WATCHING_PREF_KEY]: String(Math.max(0, Math.floor(after))) };
}

export function setStillWatchingLimit(next: number): void {
  if (next === limit) return;
  limit = next;
  emit();
}

/** Anything the viewer does: a touch, a key, a click, a pick in a menu. */
export function noteViewerActivity(): void {
  if (autoAdvances === 0) return;
  autoAdvances = 0;
  emit();
}

/** The player moved on to the next episode by itself. */
export function noteAutoAdvance(): void {
  autoAdvances++;
  emit();
}

/** True when the next episode must wait for the viewer. */
export function stillWatchingHold(): boolean {
  return limit > 0 && autoAdvances >= limit;
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => { listeners.delete(listener); };
}

export function useStillWatchingHold(): boolean {
  return useSyncExternalStore(subscribe, stillWatchingHold, stillWatchingHold);
}

/** Tests only: back to a fresh app. */
export function resetStillWatchingForTests(): void {
  autoAdvances = 0;
  limit = STILL_WATCHING_DEFAULT;
  emit();
}
