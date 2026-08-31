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
export type SeekVerdict = "healthy" | "repairable" | "manual" | "informational";

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
>;

// verdictOf trusts the server's verdict and, for a server that predates it,
// derives the same answer from the flags with the same precedence: what only
// the operator can fix wins over what Evey could fix, which wins over a fact.
export function verdictOf(it: SeekVerdictInput): SeekVerdict {
  switch (it.verdict) {
    case "healthy":
    case "repairable":
    case "manual":
    case "informational":
      return it.verdict;
  }
  if (it.truncated || it.seekCoarse) return "manual";
  if (
    it.hasIndex === false ||
    it.hasIndex === undefined ||
    Number(it.damagedBytes ?? 0) > 0 ||
    audioDelayed(it) ||
    it.wrongContainer
  ) {
    return "repairable";
  }
  if ((it.audioShortfallSec ?? 0) > AUDIO_SHORTFALL_FLOOR_SEC || it.trailingJunk) return "informational";
  return "healthy";
}

export function isAcked(it: Pick<V1SeekIndexItem, "ackedAtMs">): boolean {
  return Number(it.ackedAtMs ?? 0) > 0;
}

// Only what nobody here can repair may be acknowledged.
export function isAckable(it: SeekVerdictInput): boolean {
  const v = verdictOf(it);
  return v === "manual" || v === "informational";
}

export interface SeekTriage {
  repairable: number;
  manual: number; // still awaiting the operator (acknowledged ones excluded)
  informational: number; // idem
  acked: number;
}

// triageOf counts the items exactly the way the Go rollups do, so the number
// on the "Repair all" button is the number of rows the job will process.
export function triageOf(items: (SeekVerdictInput & Pick<V1SeekIndexItem, "ackedAtMs">)[]): SeekTriage {
  const out: SeekTriage = { repairable: 0, manual: 0, informational: 0, acked: 0 };
  for (const it of items) {
    const v = verdictOf(it);
    if (v === "repairable") out.repairable++;
    else if (v === "healthy") continue;
    else if (isAcked(it)) out.acked++;
    else if (v === "manual") out.manual++;
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

export type SeekView = "attention" | "repairable" | "manual" | "informational" | "acked" | "backups" | "all";
export const SEEK_VIEWS: SeekView[] = ["attention", "repairable", "manual", "informational", "acked", "backups", "all"];

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
  const out = { attention: 0, repairable: 0, manual: 0, informational: 0, acked: 0, backups: 0, all: 0 };
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

const VERDICT_RANK: Record<SeekVerdict, number> = { repairable: 0, manual: 1, informational: 2, healthy: 3 };

// sortSeekItems: "priority" ranks what Evey can fix first, then what the
// operator must do, then facts, then healthy; acknowledged files sink below
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

