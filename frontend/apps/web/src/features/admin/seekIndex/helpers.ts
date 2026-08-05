import type { V1SeekIndexItem, V1SeekIndexVolume } from "@evey/api";

// Base path of the cached-scan GET - invalidating it refreshes every scope's
// cached result once a job finishes (the job key has a different base string).
export const SCAN_QUERY_BASE = "/api/v1/admin/media/seek-index";

export const KIND_KEYS: Record<string, string> = {
  scan: "admin.seekIndex.jobScan",
  reindex: "admin.seekIndex.jobReindex",
  rollback: "admin.seekIndex.jobRollback",
  purge: "admin.seekIndex.jobPurge",
  remux_audio: "admin.seekIndex.jobRemuxAudio",
};

// Mirror of the backend's maintenance.AudioDelayFlagSec: above it the file's
// audio starts late enough to be a mux defect, not codec priming.
const AUDIO_DELAY_FLOOR_SEC = 0.1;

export function audioDelayed(it: V1SeekIndexItem): boolean {
  return (it.audioDelaySec ?? 0) > AUDIO_DELAY_FLOOR_SEC;
}

export function formatSize(bytes: number): string {
  if (!bytes) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

// Mirror of the server's rewrite space-preflight margin: the numbers shown to
// the operator should predict what the repair itself will accept.
const VOLUME_HEADROOM_BYTES = 256 * 1024 * 1024;

// Rolling-backup window for whole-library repairs: at most this many
// .mkvgobak copies exist at any moment during the sweep.
export const BACKUP_WINDOW = 5;

// Keeping backups persists the SUM of the candidates on the volume.
export function volumeBackupsRisky(v: V1SeekIndexVolume): boolean {
  return Number(v.missingBytes ?? 0) + VOLUME_HEADROOM_BYTES > Number(v.freeBytes ?? 0);
}

// One rewrite needs the largest candidate free as a temporary copy.
export function volumeTempRisky(v: V1SeekIndexVolume): boolean {
  return Number(v.largestMissingBytes ?? 0) + VOLUME_HEADROOM_BYTES > Number(v.freeBytes ?? 0);
}

export function errText(err: { message?: string } | null | undefined, fallback: string): string {
  return err?.message || fallback;
}

export function baseName(p: string): string {
  const i = p.lastIndexOf("/");
  return i >= 0 ? p.slice(i + 1) : p;
}
