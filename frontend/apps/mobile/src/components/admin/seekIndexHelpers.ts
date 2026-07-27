import type { V1SeekIndexItem, V1SeekIndexVolume } from "@evey/api";

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

export function errText(err: { message?: string } | null | undefined, fallback: string): string {
  return err?.message || fallback;
}

export function baseName(p: string): string {
  const i = p.lastIndexOf("/");
  return i >= 0 ? p.slice(i + 1) : p;
}

// What "Repair all" will actually act on: the missing-index count minus the
// files that no repair can help. Truncated: the tail is gone, no repair
// exists. Coarse: already rebuilt once and still too sparse to seek, because
// the encode's keyframes are the limit, not the index. Counting the raw
// missing-index total instead (a prod bug already fixed on web) advertises a
// fix the repair batch then silently skips.
export function repairableCount(
  items: Pick<V1SeekIndexItem, "hasIndex" | "truncated" | "seekCoarse">[],
): number {
  return items.filter((it) => !it.hasIndex && !it.truncated && !it.seekCoarse).length;
}

// Mirror of the server's rewrite space-preflight margin: the numbers shown to
// the operator should predict what the repair itself will accept.
export const VOLUME_HEADROOM_BYTES = 256 * 1024 * 1024;

// Rolling-backup window for whole-library repairs: at most this many
// .mkvgobak copies exist at any moment during the sweep.
export const BACKUP_WINDOW = 5;

// Keeping backups persists the SUM of the candidates on the volume.
export function volumeBackupsRisky(v: Pick<V1SeekIndexVolume, "missingBytes" | "freeBytes">): boolean {
  return Number(v.missingBytes ?? 0) + VOLUME_HEADROOM_BYTES > Number(v.freeBytes ?? 0);
}

// One rewrite needs the largest candidate free as a temporary copy.
export function volumeTempRisky(v: Pick<V1SeekIndexVolume, "largestMissingBytes" | "freeBytes">): boolean {
  return Number(v.largestMissingBytes ?? 0) + VOLUME_HEADROOM_BYTES > Number(v.freeBytes ?? 0);
}
