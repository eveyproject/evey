// The live state of the sliced reduced-quality downloads, on its own.
//
// Split out of reducedDownload.ts for one concrete reason: the screens need to
// ASK what a download is doing (downloadStageLabel), and reducedDownload.ts
// pulls in expo-file-system, the native background downloader and the embedded
// Go module at import time. Reading a counter must not drag any of that in -
// it took two screen test suites down when the label helper first imported the
// whole flow, and on device it would be paid on every render path that only
// wants a string.

export interface ReducedJob {
  cancelled: boolean;
  /** Wakes the poll loop early so a cancel is felt at once rather than at the
   * end of the current five-second wait. */
  wake?: () => void;
  /** Server-side production progress, 0..1000. */
  preparePermille: number;
  /** How many slices are collected in the staging dir. */
  collected: number;
  slices: number;
  /** Stops the in-flight slice transfer; set while one is running. */
  stopTransfer?: () => void;
}

const jobs = new Map<string, ReducedJob>();

/** Claim the item. False when one is already running - the caller must not
 * start a second collection into the same staging directory. */
export function claimReducedJob(mediaItemId: string, slices: number): ReducedJob | null {
  if (jobs.has(mediaItemId)) return null;
  const job: ReducedJob = { cancelled: false, preparePermille: 0, collected: 0, slices };
  jobs.set(mediaItemId, job);
  return job;
}

export function releaseReducedJob(mediaItemId: string): void {
  jobs.delete(mediaItemId);
}

/** True while a reduced download is being prepared, collected or joined. */
export function isReducedDownloadRunning(mediaItemId: string): boolean {
  return jobs.has(mediaItemId);
}

/** Server-side production progress (0..1000) - what stops a long wait from
 * reading as a hang, the exact failure the compat path already hit. */
export function getReducedPreparePermille(mediaItemId: string): number {
  return jobs.get(mediaItemId)?.preparePermille ?? 0;
}

/** Slices collected so far, and how many there are in total. */
export function getReducedSliceProgress(mediaItemId: string): { collected: number; slices: number } {
  const job = jobs.get(mediaItemId);
  return { collected: job?.collected ?? 0, slices: job?.slices ?? 0 };
}

/** Ask a running reduced download to stop at the next slice boundary. */
export function cancelReducedDownload(mediaItemId: string): boolean {
  const job = jobs.get(mediaItemId);
  if (!job) return false;
  job.cancelled = true;
  job.wake?.();
  job.stopTransfer?.();
  return true;
}
