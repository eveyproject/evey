import * as FileSystem from "expo-file-system";
import {
  createDownloadTask,
  getExistingDownloadTasks,
  completeHandler,
  directories,
} from "@kesha-antonov/react-native-background-downloader";
import { eveyFetch } from "@evey/api";
import { getServerUrl } from "./server-config";

// Persistent download index: same path as before so existing offline files
// remain visible after the background-downloader migration. The lib writes
// the actual media file under directories.documents (system-managed); we
// keep an app-level JSON index to map mediaItemId → filePath + metadata.
const docDir = (FileSystem as any).documentDirectory ?? (FileSystem as any).default?.documentDirectory ?? "";
const DOWNLOAD_DIR = `${docDir}downloads/`;

export interface DownloadedMedia {
  mediaItemId: string;
  title: string;
  filePath: string;
  fileSize: number;
  downloadedAt: string;
}

const INDEX_FILE = `${DOWNLOAD_DIR}index.json`;

// Track in-flight downloads in memory so the UI can subscribe to progress
// updates without re-binding callbacks on every render. Cleared on done/error.
type ProgressListener = (pct: number) => void;
const progressListeners = new Map<string, Set<ProgressListener>>();
const inFlight = new Map<string, { totalBytes: number; downloadedBytes: number }>();

async function ensureDir() {
  const info = await FileSystem.getInfoAsync(DOWNLOAD_DIR);
  if (!info.exists) {
    await FileSystem.makeDirectoryAsync(DOWNLOAD_DIR, { intermediates: true });
  }
}

async function readIndex(): Promise<DownloadedMedia[]> {
  try {
    const raw = await FileSystem.readAsStringAsync(INDEX_FILE);
    return JSON.parse(raw);
  } catch {
    return [];
  }
}

async function writeIndex(items: DownloadedMedia[]) {
  await ensureDir();
  await FileSystem.writeAsStringAsync(INDEX_FILE, JSON.stringify(items));
}

export async function getDownloads(): Promise<DownloadedMedia[]> {
  return readIndex();
}

// destPath under directories.documents (system-managed by the native module).
// Keep the .mp4 extension so Android's MediaScanner indexes the file and the
// system player can open it from the downloads folder if needed.
function destPathFor(mediaItemId: string): string {
  return `${directories.documents}/evey-${mediaItemId}.mp4`;
}

function notifyProgress(id: string, pct: number) {
  const listeners = progressListeners.get(id);
  if (!listeners) return;
  listeners.forEach((cb) => { try { cb(pct); } catch { /* silent */ } });
}

export function subscribeToProgress(mediaItemId: string, cb: ProgressListener): () => void {
  let set = progressListeners.get(mediaItemId);
  if (!set) {
    set = new Set();
    progressListeners.set(mediaItemId, set);
  }
  set.add(cb);
  return () => {
    set?.delete(cb);
    if (set && set.size === 0) progressListeners.delete(mediaItemId);
  };
}

export function getInFlightProgress(mediaItemId: string): number | null {
  const state = inFlight.get(mediaItemId);
  if (!state) return null;
  if (state.totalBytes <= 0) return NaN;
  return state.downloadedBytes / state.totalBytes;
}

async function persistDownloaded(mediaItemId: string, title: string, destPath: string, totalBytes: number) {
  const index = await readIndex();
  // Replace any existing entry with the same id so retry-after-failure keeps
  // a single row instead of accumulating duplicates.
  const without = index.filter((i) => i.mediaItemId !== mediaItemId);
  without.push({
    mediaItemId,
    title,
    filePath: destPath,
    fileSize: totalBytes,
    downloadedAt: new Date().toISOString(),
  });
  await writeIndex(without);
}

export async function startDownload(
  mediaItemId: string,
  title: string,
  onProgress?: (progress: number) => void,
): Promise<string> {
  await ensureDir();
  const serverUrl = await getServerUrl();
  if (!serverUrl) throw new Error("Not authenticated");

  const { token } = await eveyFetch<{ token: string; mediaId: string }>({
    url: `/api/v1/media/${mediaItemId}/stream-token`,
    method: "POST",
  });
  const streamUrl = `${serverUrl}/api/v1/media/${mediaItemId}/download?token=${token}`;
  const destPath = destPathFor(mediaItemId);

  // Keep the unsubscribe so the progress listener is removed when the download
  // settles — otherwise every download permanently leaks its callback into the
  // global progressListeners map (and keeps firing setState on a screen the user
  // has since left).
  const unsubscribeProgress = onProgress ? subscribeToProgress(mediaItemId, onProgress) : undefined;
  inFlight.set(mediaItemId, { totalBytes: 0, downloadedBytes: 0 });

  return new Promise<string>((resolve, reject) => {
    const task = createDownloadTask({
      id: mediaItemId,
      url: streamUrl,
      destination: destPath,
      metadata: { title },
    });

    task
      .begin(({ expectedBytes }) => {
        const state = inFlight.get(mediaItemId);
        if (state) state.totalBytes = expectedBytes ?? 0;
      })
      .progress(({ bytesDownloaded, bytesTotal }) => {
        const state = inFlight.get(mediaItemId);
        if (state) {
          state.downloadedBytes = bytesDownloaded;
          if (bytesTotal > 0) state.totalBytes = bytesTotal;
        }
        const pct = bytesTotal > 0 ? bytesDownloaded / bytesTotal : NaN;
        notifyProgress(mediaItemId, pct);
      })
      .done(async () => {
        const state = inFlight.get(mediaItemId);
        const totalBytes = state?.totalBytes ?? 0;
        try {
          await persistDownloaded(mediaItemId, title, destPath, totalBytes);
        } catch (err) {
          if (process.env.NODE_ENV !== "production") console.warn("persist download index", err);
        }
        inFlight.delete(mediaItemId);
        completeHandler(mediaItemId);
        unsubscribeProgress?.();
        resolve(destPath);
      })
      .error(({ error }) => {
        inFlight.delete(mediaItemId);
        completeHandler(mediaItemId);
        unsubscribeProgress?.();
        reject(new Error(error || "Download failed"));
      });

    // createDownloadTask only registers the task with the native module —
    // start() is what actually triggers the OS download (see DownloadTask.ts
    // `start()`: dispatches `download()` to native). Without this call the
    // task sits in PENDING forever and none of the handlers fire.
    task.start();
  });
}

// rehydrateBackgroundDownloads must be called once at app startup. It picks
// up downloads that were running when the app was killed (foreground service
// kept them alive at the OS level) and re-attaches our JS callbacks so the
// UI can track them again. Without this, an in-progress download finishes
// silently and the index never gets updated.
export async function rehydrateBackgroundDownloads(): Promise<void> {
  let tasks: Awaited<ReturnType<typeof getExistingDownloadTasks>>;
  try {
    tasks = await getExistingDownloadTasks();
  } catch (err) {
    if (process.env.NODE_ENV !== "production") console.warn("rehydrate downloads", err);
    return;
  }
  for (const task of tasks) {
    const mediaItemId = task.id;
    const title = (task.metadata as { title?: string })?.title ?? mediaItemId;
    const destPath = destPathFor(mediaItemId);
    inFlight.set(mediaItemId, { totalBytes: 0, downloadedBytes: 0 });
    task
      .progress(({ bytesDownloaded, bytesTotal }) => {
        const state = inFlight.get(mediaItemId);
        if (state) {
          state.downloadedBytes = bytesDownloaded;
          if (bytesTotal > 0) state.totalBytes = bytesTotal;
        }
        const pct = bytesTotal > 0 ? bytesDownloaded / bytesTotal : NaN;
        notifyProgress(mediaItemId, pct);
      })
      .done(async () => {
        const state = inFlight.get(mediaItemId);
        try {
          await persistDownloaded(mediaItemId, title, destPath, state?.totalBytes ?? 0);
        } catch { /* silent */ }
        inFlight.delete(mediaItemId);
        completeHandler(mediaItemId);
      })
      .error(() => {
        inFlight.delete(mediaItemId);
        completeHandler(mediaItemId);
      });
  }
}

export async function deleteDownload(mediaItemId: string): Promise<void> {
  const index = await readIndex();
  const item = index.find((i) => i.mediaItemId === mediaItemId);
  if (item) {
    try { await FileSystem.deleteAsync(item.filePath, { idempotent: true }); } catch (err) { console.warn("caught", err); }
  }
  await writeIndex(index.filter((i) => i.mediaItemId !== mediaItemId));
}

export async function getDownloadPath(mediaItemId: string): Promise<string | null> {
  const index = await readIndex();
  const item = index.find((i) => i.mediaItemId === mediaItemId);
  if (!item) return null;
  const info = await FileSystem.getInfoAsync(item.filePath);
  return info.exists ? item.filePath : null;
}

export async function getTotalDownloadSize(): Promise<number> {
  const index = await readIndex();
  let total = 0;
  for (const item of index) {
    try {
      const info = await FileSystem.getInfoAsync(item.filePath);
      if (info.exists && info.size) total += info.size;
    } catch (err) { console.warn("caught", err); }
  }
  return total;
}
