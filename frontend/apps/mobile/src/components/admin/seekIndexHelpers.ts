import type { V1SeekIndexItem } from "@evey/api";

// Pure helpers extracted from the seek-index admin screen so the byte/delay
// formatting logic can be unit-tested without mounting react-native
// components or the generated @evey/api hooks.

// Mirror of the backend's maintenance.AudioDelayFlagSec: above it the file's
// audio starts late enough to be a mux defect, not codec priming.
export const AUDIO_DELAY_FLOOR_SEC = 0.1;

export function audioDelayed(it: Pick<V1SeekIndexItem, "audioDelaySec">): boolean {
  return (it.audioDelaySec ?? 0) > AUDIO_DELAY_FLOOR_SEC;
}

export function formatSize(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

export function baseName(p: string): string {
  const i = p.lastIndexOf("/");
  return i >= 0 ? p.slice(i + 1) : p;
}
