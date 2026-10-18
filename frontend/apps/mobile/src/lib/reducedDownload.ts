// Size-reduced (720p) download: collect the server's Matroska slices one at a
// time and join them into the file the phone keeps.
//
// Why this is not just another URL passed to startDownload: there is no file
// on the server to fetch. A 4K remux does not fit on a phone, so the operator
// can offer a lighter copy - but producing it re-encodes VIDEO, which is close
// to realtime on the weak-NAS target (an hour for a feature film). The server
// therefore emits consecutive slices out of ONE ffmpeg and releases each as we
// collect it, so it never holds a second copy of the film either. Reassembly
// is ours (mkvgo's join, embedded in @evey/offline-audio).
//
// The unit of work is the slice, everywhere: we can start collecting minutes
// after asking rather than waiting an hour, a dropped connection costs one
// slice instead of the film, and the server's disk footprint is only the tail
// we have not picked up yet.

import * as FileSystem from "expo-file-system/legacy";
import { createDownloadTask, completeHandler, directories } from "@kesha-antonov/react-native-background-downloader";
import { eveyFetch } from "@evey/api";
import { moveMediaFile } from "@evey/media-caps";
import { joinDownloadSlices, deleteJoinedSlices, sliceFileName, OptimizeError } from "@evey/offline-audio";
import { getServerUrl } from "./server-config";
import { audioPassNotificationTitle } from "./downloadNotificationTexts";
import { claimReducedJob, releaseReducedJob, type ReducedJob } from "./reducedDownloadState";
import {
  clearDownloadInFlight,
  deleteDownloadedFileAnywhere,
  notifyDownloadProgress,
  recordDownloadedFile,
  registerDownloadInFlight,
  resolveDownloadDest,
  toFileUri,
  type DownloadAuthContext,
} from "./downloads";

/** What the server will hand over, and whether it is worth offering at all. */
export interface ReducedPlan {
  /** A lighter copy is possible AND smaller than the source. */
  available: boolean;
  /** The server's own opinion: the source is over the operator's threshold. */
  offer: boolean;
  height: number;
  width: number;
  estimatedBytes: number;
  sourceBytes: number;
  slices: number;
  sliceSec: number;
  durationMs: number;
}

interface ReducedStatus {
  status: "absent" | "preparing" | "ready" | "failed" | "interrupted" | "unnecessary";
  error?: string;
  permille?: number;
  slicesReady?: number;
  slices?: number;
  durationMs?: number;
}

const POLL_MS = 5000;
// A slice is ten minutes of film; the server encodes at roughly realtime on
// the weak-NAS target, so twenty-five minutes of waiting for ONE slice means
// something is wrong, not merely slow.
const SLICE_WAIT_TIMEOUT_MS = 25 * 60 * 1000;

/**
 * Ask the server whether a lighter copy of this item is worth offering.
 * Returns null for every "no" shape - route absent (old server, or the
 * operator left downloads.quality_choice off), refused, unreachable - because
 * the app's answer to all of them is the same: offer the original only.
 */
export async function fetchReducedPlan(mediaItemId: string): Promise<ReducedPlan | null> {
  try {
    const plan = await eveyFetch<ReducedPlan>({
      url: `/api/v1/media/${mediaItemId}/reduced-download/plan`,
      method: "GET",
    });
    return plan?.available ? plan : null;
  } catch {
    return null;
  }
}

// Re-exported so callers have one import for the whole feature; the state
// itself lives in a module with no native imports (see reducedDownloadState).
export {
  isReducedDownloadRunning,
  getReducedPreparePermille,
  getReducedSliceProgress,
  cancelReducedDownload,
} from "./reducedDownloadState";

const stagingRoot = (mediaItemId: string) => `${directories.documents}/evey-reduced-${mediaItemId}`;

/**
 * Collect and assemble the reduced variant. Resolves with the final file path.
 *
 * Everything survives being restarted: slices already in the staging dir are
 * not re-fetched, and the server resumes its production where it stopped. The
 * one thing that is NOT resumable is the join itself, which is why the slices
 * are only deleted after it succeeds.
 */
export async function startReducedDownload(
  mediaItemId: string,
  title: string,
  durationMs: number,
  slices: number,
  opts?: { authContext?: DownloadAuthContext },
): Promise<string> {
  // Claimed BEFORE the first await, or a double tap starts two collections of
  // the same film into the same staging dir: the second caller would sail past
  // a check made against a map the first has not written to yet.
  const job = claimReducedJob(mediaItemId, slices);
  if (!job) throw new Error("Download already running");
  registerDownloadInFlight(mediaItemId, title);

  const dir = stagingRoot(mediaItemId);
  try {
    const serverUrl = await getServerUrl();
    if (!serverUrl) throw new Error("Not authenticated");
    await FileSystem.makeDirectoryAsync(toFileUri(dir), { intermediates: true });

    const started = await postStatus(mediaItemId);
    if (started.status === "unnecessary") {
      throw new Error("The server says this item does not need a reduced copy");
    }
    // The server knows the real slice count and length; trust it over what the
    // caller was told by a possibly stale plan.
    if (started.slices && started.slices > 0) job.slices = started.slices;
    if (started.durationMs && started.durationMs > 0) durationMs = started.durationMs;

    for (let i = 0; i < job.slices; i++) {
      throwIfCancelled(job);
      const slicePath = `${dir}/${sliceFileName(i)}`;
      if (!(await fileExists(slicePath))) {
        await waitForSlice(mediaItemId, i, job);
        throwIfCancelled(job);
        await fetchSlice(mediaItemId, i, slicePath, serverUrl, job, () => {
          // Whole-download progress: the slices are the transfer, the join is
          // the last tenth. An honest 0..1 beats a bar that jumps at the end.
          notifyDownloadProgress(mediaItemId, (job.collected + 0.5) / job.slices * 0.9);
        });
      }
      job.collected = i + 1;
      notifyDownloadProgress(mediaItemId, (job.collected / job.slices) * 0.9);
    }

    throwIfCancelled(job);
    const joined = `${dir}/joined.mkv`;
    await joinDownloadSlices(dir, job.slices, joined, durationMs, (p) => {
      if (p.stage === "join") notifyDownloadProgress(mediaItemId, 0.9 + (p.permille / 1000) * 0.1);
    }, title, audioPassNotificationTitle());

    // Publish where the user (and VLC) can reach it, then let the staging dir
    // go. The move is the last irreversible step on purpose.
    const dest = await resolveDownloadDest(title, mediaItemId, "mkv");
    if (!(await moveMediaFile(joined, dest))) {
      await FileSystem.moveAsync({ from: toFileUri(joined), to: toFileUri(dest) });
    }
    const size = await fileSize(dest);
    await recordDownloadedFile(mediaItemId, title, dest, size, opts?.authContext, ["aac"]);

    await deleteJoinedSlices(dir, job.slices);
    await cleanupStaging(dir);
    void discardServerJob(mediaItemId);
    notifyDownloadProgress(mediaItemId, 1);
    console.log(`evey-dl reduced done ${mediaItemId} -> ${dest} (${job.slices} slices)`);
    return dest;
  } catch (err) {
    // A cancelled or failed reduced download leaves nothing behind: the
    // staging dir is ours alone, and the server job is the server's copy of
    // the same abandoned work.
    await cleanupStaging(dir);
    void discardServerJob(mediaItemId);
    throw err;
  } finally {
    releaseReducedJob(mediaItemId);
    clearDownloadInFlight(mediaItemId);
  }
}

function throwIfCancelled(job: ReducedJob) {
  if (job.cancelled) throw new Error("Download cancelled");
}

async function postStatus(mediaItemId: string): Promise<ReducedStatus> {
  return eveyFetch<ReducedStatus>({
    url: `/api/v1/media/${mediaItemId}/reduced-download`,
    method: "POST",
  });
}

/** Poll until slice `index` exists server-side. A dropped poll is ignored:
 * one lost request must not abandon an hour of encoding that is still going. */
async function waitForSlice(mediaItemId: string, index: number, job: ReducedJob): Promise<void> {
  const deadline = Date.now() + SLICE_WAIT_TIMEOUT_MS;
  for (;;) {
    throwIfCancelled(job);
    let st: ReducedStatus | null = null;
    try {
      st = await eveyFetch<ReducedStatus>({
        url: `/api/v1/media/${mediaItemId}/reduced-download/status`,
        method: "GET",
      });
    } catch {
      /* transient - the deadline is the real bound */
    }
    if (st) {
      if (typeof st.permille === "number" && st.permille > job.preparePermille) {
        job.preparePermille = st.permille;
        // Wake the media screen's subscription for a re-read; NaN means "no
        // byte figure to show", same convention the compat path uses.
        notifyDownloadProgress(mediaItemId, NaN);
      }
      if ((st.slicesReady ?? 0) > index) return;
      if (st.status === "failed") throw new Error(st.error || "The server could not prepare the reduced copy");
      // A production the server forgot (restart, sweep) is restarted here
      // rather than waited on forever.
      if (st.status === "absent" || st.status === "interrupted") await postStatus(mediaItemId).catch(() => undefined);
    }
    if (Date.now() >= deadline) throw new Error("The server took too long to prepare this part of the film");
    await waitOrWake(POLL_MS, job);
  }
}

/** One slice, through the same background engine as a normal download - the
 * app WILL be backgrounded during an hour-long collection. */
async function fetchSlice(
  mediaItemId: string,
  index: number,
  destPath: string,
  serverUrl: string,
  job: ReducedJob,
  onProgress: () => void,
): Promise<void> {
  const { token } = await eveyFetch<{ token: string }>({
    url: `/api/v1/media/${mediaItemId}/stream-token`,
    method: "POST",
  });
  const url = `${serverUrl}/api/v1/media/${mediaItemId}/reduced-download/slice/${index}?token=${token}`;
  // The task id must be unique per slice or the native module reuses the
  // previous task's state and the second slice never starts.
  const taskId = `${mediaItemId}#s${index}`;

  await new Promise<void>((resolve, reject) => {
    const task = createDownloadTask({ id: taskId, url, destination: destPath, metadata: { mediaItemId, index } });
    job.stopTransfer = () => { void task.stop().catch(() => undefined); };
    task
      .progress(onProgress)
      .done(() => {
        completeHandler(taskId)?.catch(() => undefined);
        job.stopTransfer = undefined;
        resolve();
      })
      .error(({ error }) => {
        completeHandler(taskId)?.catch(() => undefined);
        job.stopTransfer = undefined;
        void deleteDownloadedFileAnywhere(destPath);
        reject(new Error(error || `slice ${index} failed`));
      });
    task.start();
  });
}

/** Tell the server we are done with it, so the tail of the film stops
 * occupying its disk until the idle sweep gets round to it. Best effort. */
async function discardServerJob(mediaItemId: string): Promise<void> {
  try {
    await eveyFetch({ url: `/api/v1/media/${mediaItemId}/reduced-download`, method: "DELETE" });
  } catch {
    /* the server's own idle sweep is the backstop */
  }
}

async function cleanupStaging(dir: string): Promise<void> {
  try {
    await FileSystem.deleteAsync(toFileUri(dir), { idempotent: true });
  } catch (err) {
    console.warn("evey-dl reduced staging cleanup", err);
  }
}

async function fileExists(path: string): Promise<boolean> {
  try {
    return (await FileSystem.getInfoAsync(toFileUri(path))).exists;
  } catch {
    return false;
  }
}

async function fileSize(path: string): Promise<number> {
  try {
    const info = await FileSystem.getInfoAsync(toFileUri(path));
    return info.exists && info.size ? info.size : 0;
  } catch {
    return 0;
  }
}

/** Sleep between polls, but come back immediately on a cancel: a user who
 * taps cancel must not watch the row sit there for another five seconds. */
function waitOrWake(ms: number, job: ReducedJob): Promise<void> {
  // Already cancelled before the wait even starts (the cancel landed between
  // two awaits): do not install a timer nobody will ring.
  if (job.cancelled) return Promise.resolve();
  return new Promise((resolve) => {
    const finish = () => {
      clearTimeout(timer);
      job.wake = undefined;
      resolve();
    };
    const timer = setTimeout(finish, ms);
    job.wake = finish;
  });
}

/** Human-facing reason a reduced download failed, for the error toast. */
export function reducedDownloadMessage(err: unknown): string {
  if (err instanceof OptimizeError) {
    switch (err.code) {
      case "slice-missing":
      case "no-slices":
        return "downloads.reduced.errorIncomplete";
      case "join-failed":
        return "downloads.reduced.errorJoin";
      case "unavailable":
        return "downloads.reduced.errorUnsupportedDevice";
      default:
        return "downloads.reduced.errorGeneric";
    }
  }
  return "downloads.reduced.errorGeneric";
}
