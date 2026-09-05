// Where a downloaded title's playhead lives when the server cannot be told.
//
// useProgressReporter only runs once usePlaySession has a sessionId, and a
// session is opened by a REQUEST to the server. Offline that request fails, so
// no session ever exists, so nothing - not one tick, not the flush at unmount -
// records where the viewer got to. Watch 40 minutes of a downloaded film on a
// plane, close the app, and it resumes at the point you were at before takeoff.
// That is the exact case downloads exist for.
//
// Kept in its own file rather than in the download index: the playhead is
// written every few seconds, the index is not, and a hot write path has no
// business rewriting the metadata of every download on the device.
import * as FileSystem from "expo-file-system/legacy";

const docDir = FileSystem.documentDirectory ?? "";
const DIR = `${docDir}downloads/`;
const FILE = `${DIR}progress.json`;

export interface OfflineProgress {
  mediaItemId: string;
  positionMs: number;
  durationMs: number;
  /**
   * When this device recorded the position. Compared against the SERVER's own
   * progress.updatedAt to decide which is fresher - "the bigger position wins"
   * would be wrong for a viewer who restarted a title from the beginning
   * offline, and for one who watched further on the web in the meantime.
   */
  updatedAt: string;
}

// Same read-modify-write hazard as the download index, same answer: one chain.
// The player writes on a timer while a rehydrated download's own bookkeeping
// runs, and a lost update here is a lost playhead.
let chain: Promise<unknown> = Promise.resolve();

function locked<T>(fn: () => Promise<T>): Promise<T> {
  const run = chain.then(fn, fn);
  chain = run.catch(() => undefined);
  return run;
}

async function readAll(): Promise<OfflineProgress[]> {
  try {
    return JSON.parse(await FileSystem.readAsStringAsync(FILE));
  } catch {
    return [];
  }
}

async function writeAll(items: OfflineProgress[]): Promise<void> {
  const info = await FileSystem.getInfoAsync(DIR);
  if (!info.exists) await FileSystem.makeDirectoryAsync(DIR, { intermediates: true });
  // Write-then-move, so a reader never parses a truncated file and concludes
  // there is no saved position.
  const tmp = `${FILE}.tmp`;
  await FileSystem.writeAsStringAsync(tmp, JSON.stringify(items));
  await FileSystem.moveAsync({ from: tmp, to: FILE });
}

/** Records the playhead of a locally played title. Never throws. */
export async function saveOfflineProgress(
  mediaItemId: string,
  positionMs: number,
  durationMs: number,
): Promise<void> {
  if (!mediaItemId || !Number.isFinite(positionMs) || positionMs < 0) return;
  try {
    await locked(async () => {
      const all = await readAll();
      const rest = all.filter((p) => p.mediaItemId !== mediaItemId);
      rest.push({
        mediaItemId,
        positionMs: Math.round(positionMs),
        durationMs: Number.isFinite(durationMs) ? Math.round(durationMs) : 0,
        updatedAt: new Date().toISOString(),
      });
      await writeAll(rest);
    });
  } catch {
    // A playhead is worth exactly one best-effort write: never break playback
    // over it.
  }
}

export async function getOfflineProgress(mediaItemId: string): Promise<OfflineProgress | null> {
  try {
    const all = await locked(readAll);
    return all.find((p) => p.mediaItemId === mediaItemId) ?? null;
  } catch {
    return null;
  }
}

/** Drops the local record once the server has been told (or the file is gone). */
export async function clearOfflineProgress(mediaItemId: string): Promise<void> {
  try {
    await locked(async () => {
      const all = await readAll();
      const rest = all.filter((p) => p.mediaItemId !== mediaItemId);
      if (rest.length !== all.length) await writeAll(rest);
    });
  } catch {
    /* best effort */
  }
}

/**
 * Which of the two positions to resume from.
 *
 * Pure, so the rule is testable on its own: the local record wins only when it
 * is genuinely FRESHER than what the server holds. A server that has never
 * heard of this title loses by default; a local record with no timestamp is
 * treated as older than any dated server record, since it cannot prove
 * otherwise.
 */
export function preferLocalProgress(
  local: { positionMs: number; updatedAt?: string } | null | undefined,
  serverUpdatedAt: string | null | undefined,
): boolean {
  if (!local || local.positionMs <= 0) return false;
  if (!serverUpdatedAt) return true;
  const localMs = Date.parse(local.updatedAt ?? "");
  const serverMs = Date.parse(serverUpdatedAt);
  if (!Number.isFinite(localMs)) return false;
  if (!Number.isFinite(serverMs)) return true;
  return localMs > serverMs;
}
