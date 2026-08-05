import type { V1SeekIndexItem, V1SeekIndexVolume } from "./generated/admin_media.schemas";

// Pure helpers of the seek-index admin screens, shared by web and mobile: the
// two apps carried byte-identical copies that had already drifted (exports,
// signatures) while mirroring the same server constants.

// Base path of the cached-scan GET - invalidating it refreshes every scope's
// cached result once a job finishes (the job key has a different base string).
export const SEEK_INDEX_SCAN_QUERY_BASE = "/api/v1/admin/media/seek-index";

export const SEEK_INDEX_KIND_KEYS: Record<string, string> = {
  scan: "admin.seekIndex.jobScan",
  reindex: "admin.seekIndex.jobReindex",
  rollback: "admin.seekIndex.jobRollback",
  purge: "admin.seekIndex.jobPurge",
  remux_audio: "admin.seekIndex.jobRemuxAudio",
};

// Mirror of the backend's maintenance.AudioDelayFlagSec: above it the file's
// audio starts late enough to be a mux defect, not codec priming.
// internal/contractguard/seek_index_mirror_test.go pins it to the Go value.
export const AUDIO_DELAY_FLOOR_SEC = 0.1;

export function audioDelayed(it: Pick<V1SeekIndexItem, "audioDelaySec">): boolean {
  return (it.audioDelaySec ?? 0) > AUDIO_DELAY_FLOOR_SEC;
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

// Mirror of the server's rewrite space-preflight margin (maintenance
// volumeHeadroomBytes): the numbers shown to the operator should predict what
// the repair itself will accept. Pinned by the same contractguard test.
export const VOLUME_HEADROOM_BYTES = 256 * 1024 * 1024;

// Rolling-backup window the clients request for whole-library repairs: at most
// this many .mkvgobak copies exist at any moment during the sweep.
export const SEEK_INDEX_BACKUP_WINDOW = 5;

// Keeping backups persists the SUM of the candidates on the volume.
export function volumeBackupsRisky(v: Pick<V1SeekIndexVolume, "missingBytes" | "freeBytes">): boolean {
  return Number(v.missingBytes ?? 0) + VOLUME_HEADROOM_BYTES > Number(v.freeBytes ?? 0);
}

// One rewrite needs the largest candidate free as a temporary copy.
export function volumeTempRisky(v: Pick<V1SeekIndexVolume, "largestMissingBytes" | "freeBytes">): boolean {
  return Number(v.largestMissingBytes ?? 0) + VOLUME_HEADROOM_BYTES > Number(v.freeBytes ?? 0);
}
