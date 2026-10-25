import { useSyncExternalStore } from "react";
import { getDeviceProfile, setLiteMode, type DeviceProfile, type LiteMode } from "@evey/media-caps";

// Light mode: what a modest television gives up so the rest stays fluid - the
// blurhash placeholders and the focus zoom on every card. A box with 2 GB runs
// the full app well; a Chromecast HD or a set's built-in Android TV, with
// less, did not (2026-10-25). Below this total memory, "auto" turns it on.
export const LITE_TOTAL_MB = 1700;

export function decideLite(profile: DeviceProfile | null): boolean {
  if (!profile) return false;
  if (profile.liteMode === "on") return true;
  if (profile.liteMode === "off") return false;
  return profile.lowRam || profile.totalMb < LITE_TOTAL_MB;
}

// Read once, on first use: the memory does not change under a running app,
// and the choice only changes through chooseLiteMode below.
let profile: DeviceProfile | null | undefined;
let choice: LiteMode | undefined;
const listeners = new Set<() => void>();

function readProfile(): DeviceProfile | null {
  if (profile === undefined) {
    // A screen test that mocks @evey/media-caps without this export throws on
    // access; every card reads this, so that must read as "no profile".
    try {
      profile = getDeviceProfile();
    } catch {
      profile = null;
    }
  }
  return profile;
}

export function isLiteMode(): boolean {
  const p = readProfile();
  return decideLite(p && choice ? { ...p, liteMode: choice } : p);
}

export function liteModeChoice(): LiteMode {
  return choice ?? readProfile()?.liteMode ?? "auto";
}

export function chooseLiteMode(mode: LiteMode): void {
  setLiteMode(mode);
  choice = mode;
  listeners.forEach((l) => l());
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function useLiteMode(): boolean {
  return useSyncExternalStore(subscribe, isLiteMode);
}

export function useLiteModeChoice(): LiteMode {
  return useSyncExternalStore(subscribe, liteModeChoice);
}

/** Test seam: forget what was read so a test can stub the native profile. */
export function resetLiteModeForTests(): void {
  profile = undefined;
  choice = undefined;
}
