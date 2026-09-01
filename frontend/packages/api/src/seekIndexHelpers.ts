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
  fix_container: "admin.seekIndex.jobFixContainer",
};

// Mirror of the backend's maintenance.AudioDelayFlagSec: above it the file's
// audio starts late enough to be a mux defect, not codec priming.
// internal/contractguard/seek_index_mirror_test.go pins it to the Go value.
export const AUDIO_DELAY_FLOOR_SEC = 0.1;

export function audioDelayed(it: Pick<V1SeekIndexItem, "audioDelaySec">): boolean {
  return (it.audioDelaySec ?? 0) > AUDIO_DELAY_FLOOR_SEC;
}

// Mirror of maintenance.AudioTrackLagFlagSec: how far a file's own audio tracks
// may disagree before the file needs a human. Codec priming leaves tens of
// milliseconds; a quarter of a second is already audible.
export const AUDIO_TRACK_LAG_FLOOR_SEC = 0.25;

// audioTracksDisagree is the only thing that turns a declared audio offset into
// something to look at. An offset on its own is how a dub is aligned to its cut
// and every player honours it - Evey rewrote such a file once and desynchronised
// it for good (a prod title, 2026-07-13). What matters is whether the file's
// OWN tracks, offsets applied, still tell the same story at the same instant.
export function audioTracksDisagree(
  it: Pick<V1SeekIndexItem, "audioTracksLagSec" | "audioTracksChecked">,
): boolean {
  return !!it.audioTracksChecked && Math.abs(Number(it.audioTracksLagSec ?? 0)) > AUDIO_TRACK_LAG_FLOOR_SEC;
}

export function errText(err: { message?: string } | null | undefined, fallback: string): string {
  return err?.message || fallback;
}

export function baseName(p: string): string {
  const i = p.lastIndexOf("/");
  return i >= 0 ? p.slice(i + 1) : p;
}

// Mirror of the backend's maintenance.AudioShortfallFlagSec.
export const AUDIO_SHORTFALL_FLOOR_SEC = 10;

// The ONE answer per file the health page is built on - see the Go Verdict.
// The ONE answer per file, and deliberately only two non-healthy ones:
// "repairable" (Evey fixes it here) and "informational" (something we
// measured and mention - possibly wrong, never a chore). The old "your call"
// verdict is gone: a detector that cannot repair something has not earned the
// right to hand the operator an instruction.
export type SeekVerdict = "healthy" | "repairable" | "informational";

export type SeekVerdictInput = Pick<
  V1SeekIndexItem,
  | "verdict"
  | "hasIndex"
  | "truncated"
  | "seekCoarse"
  | "damagedBytes"
  | "audioDelaySec"
  | "wrongContainer"
  | "audioShortfallSec"
  | "trailingJunk"
  | "audioTracksLagSec"
  | "audioTracksChecked"
>;

// verdictOf trusts the server's verdict and, for a server that predates it,
// derives the same answer from the flags: can Evey act on it here, or is it a
// remark? A server still speaking the retired "manual" verdict is read as the
// remark it always was.
export function verdictOf(it: SeekVerdictInput): SeekVerdict {
  switch (it.verdict) {
    case "healthy":
    case "repairable":
    case "informational":
      return it.verdict;
    case "manual":
      return "informational";
  }
  // Same precedence as the Go SeekStatus.Verdict, and the order matters:
  // rebuilding the index of an incomplete download is wasted work, so those
  // files leave the repair queue BEFORE the missing-index test sees them.
  if (it.truncated || it.seekCoarse) return "informational";
  if (
    it.hasIndex === false ||
    it.hasIndex === undefined ||
    Number(it.damagedBytes ?? 0) > 0 ||
    it.wrongContainer
  ) {
    return "repairable";
  }
  if (
    audioTracksDisagree(it) ||
    audioDelayed(it) ||
    (it.audioShortfallSec ?? 0) > AUDIO_SHORTFALL_FLOOR_SEC ||
    it.trailingJunk
  ) {
    return "informational";
  }
  return "healthy";
}

export function isAcked(it: Pick<V1SeekIndexItem, "ackedAtMs">): boolean {
  return Number(it.ackedAtMs ?? 0) > 0;
}

// Only what nobody here can repair may be acknowledged.
export function isAckable(it: SeekVerdictInput): boolean {
  return verdictOf(it) === "informational";
}

export interface SeekTriage {
  repairable: number;
  informational: number; // still awaiting the operator's eyes (acknowledged ones excluded)
  acked: number;
}

// triageOf counts the items exactly the way the Go rollups do, so the number
// on the "Repair all" button is the number of rows the job will process.
export function triageOf(items: (SeekVerdictInput & Pick<V1SeekIndexItem, "ackedAtMs">)[]): SeekTriage {
  const out: SeekTriage = { repairable: 0, informational: 0, acked: 0 };
  for (const it of items) {
    const v = verdictOf(it);
    if (v === "repairable") out.repairable++;
    else if (v === "healthy") continue;
    else if (isAcked(it)) out.acked++;
    else out.informational++;
  }
  return out;
}

// What "Repair all" will actually act on. Kept for the mobile app, which
// still computes the button from the items; now the verdict, not a re-derived
// subset of flags, so web and mobile cannot disagree.
export function repairableCount(items: SeekVerdictInput[]): number {
  return items.filter((it) => verdictOf(it) === "repairable").length;
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

// --- Triage of the displayed rows: views, search, sort ------------------------
//
// The list is the same for web and mobile: one view per verdict plus the
// cross-cutting ones (acknowledged, backups, everything), a free-text search on
// title/path, and a sort whose default puts what needs doing first.

export type SeekView = "attention" | "repairable" | "informational" | "acked" | "backups" | "all";
export const SEEK_VIEWS: SeekView[] = ["attention", "repairable", "informational", "acked", "backups", "all"];

export type SeekTriageItem = SeekVerdictInput & Pick<V1SeekIndexItem, "ackedAtMs" | "hasBackup" | "title" | "path" | "sizeBytes">;

// matchesView: "attention" is what awaits an action or a look - every
// non-healthy, non-acknowledged verdict, plus the files carrying a backup
// (rollback/purge are live actions). The verdict views show pending AND
// acknowledged files of that verdict; "acked" gathers the acknowledged ones.
export function matchesView(it: SeekTriageItem, view: SeekView): boolean {
  const v = verdictOf(it);
  switch (view) {
    case "all":
      return true;
    case "attention":
      return Boolean(it.hasBackup) || (v !== "healthy" && !isAcked(it));
    case "acked":
      return isAcked(it);
    case "backups":
      return Boolean(it.hasBackup);
    default:
      return v === view;
  }
}

export function viewCounts(items: SeekTriageItem[]): Record<SeekView, number> {
  const out = { attention: 0, repairable: 0, informational: 0, acked: 0, backups: 0, all: 0 };
  for (const it of items) {
    for (const view of SEEK_VIEWS) {
      if (matchesView(it, view)) out[view]++;
    }
  }
  return out;
}

export function searchMatches(it: Pick<V1SeekIndexItem, "title" | "path">, query: string): boolean {
  const q = query.trim().toLowerCase();
  if (!q) return true;
  return (it.title ?? "").toLowerCase().includes(q) || (it.path ?? "").toLowerCase().includes(q);
}

export type SeekSort = "priority" | "size" | "title";

const VERDICT_RANK: Record<SeekVerdict, number> = { repairable: 0, informational: 1, healthy: 2 };

// sortSeekItems: "priority" ranks what Evey can fix first, then the remarks,
// then healthy; acknowledged files sink below
// their pending peers; ties go to the biggest file (the one that matters
// most to the shelf). Pure: returns a new array.
export function sortSeekItems<T extends SeekTriageItem>(items: T[], sort: SeekSort): T[] {
  const size = (it: T) => Number(it.sizeBytes ?? 0);
  const title = (it: T) => (it.title || it.path || "").toLowerCase();
  return [...items].sort((a, b) => {
    switch (sort) {
      case "size":
        return size(b) - size(a) || title(a).localeCompare(title(b));
      case "title":
        return title(a).localeCompare(title(b));
      default: {
        const rank = VERDICT_RANK[verdictOf(a)] - VERDICT_RANK[verdictOf(b)];
        if (rank !== 0) return rank;
        const ack = Number(isAcked(a)) - Number(isAcked(b));
        if (ack !== 0) return ack;
        return size(b) - size(a) || title(a).localeCompare(title(b));
      }
    }
  });
}

// triageRows is the one pipeline both apps render from: view, then search,
// then sort.
export function triageRows<T extends SeekTriageItem>(items: T[], view: SeekView, query: string, sort: SeekSort): T[] {
  return sortSeekItems(
    items.filter((it) => matchesView(it, view) && searchMatches(it, query)),
    sort,
  );
}


// The one choice the operator makes when starting a scan, mirroring the Go
// maintenance.ScanPace. It is a DISK budget, never a difference in what the
// scan finds: the same library comes back with the same verdicts either way.
//
//   gentle - one file at a time, then a stand-aside as long as that file took
//            to read. The disks are free about half the time, so playback
//            survives the scan. The default, because nobody watches a scan.
//   fast   - several files at once with no pause, so a library spread over
//            several disks uses all of them instead of one.
export type SeekScanPace = "gentle" | "fast";

export const SEEK_SCAN_PACE_DEFAULT: SeekScanPace = "gentle";

// scanRequestBody builds the start-scan payload both apps send, so a pace
// chosen on the phone means exactly what it means in the browser. The gentle
// pace is sent as ABSENCE, because that is what the server already does with a
// missing field: the safe pace is what you get for saying nothing.
export function scanRequestBody(libraryId: string, pace: SeekScanPace): { libraryId?: string; pace?: SeekScanPace } {
  return {
    ...(libraryId ? { libraryId } : {}),
    ...(pace === SEEK_SCAN_PACE_DEFAULT ? {} : { pace }),
  };
}
