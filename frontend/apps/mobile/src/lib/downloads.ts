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
// Title alongside inFlight, kept separately so a download-in-progress can be
// listed in the Downloads screen before it ever reaches the persisted index
// (an item only enters that index once .done() fires).
const inFlightTitles = new Map<string, string>();

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

// A media id must never carry characters that let it escape the sandbox
// directory on write ("/", "..", backslash) or, via a deep link, redirect the
// authenticated request to an arbitrary API path ("?", "#", whitespace, control
// chars). Evey ids are opaque slugs, so rather than assume an exact grammar we
// reject the dangerous shapes and fail closed at the boundary.
const UNSAFE_MEDIA_ID = /[/\\?#\s]|\.\./;

function assertMediaId(id: string): void {
  if (id.length === 0 || id.length > 128 || UNSAFE_MEDIA_ID.test(id)) {
    throw new Error("invalid media id");
  }
}

// destPath under directories.documents (system-managed by the native module).
// The extension is kept real (not always .mp4) so Android's MediaScanner
// indexes the file correctly and the system player/file manager can open it.
function destPathFor(mediaItemId: string, ext: string): string {
  return `${directories.documents}/evey-${mediaItemId}.${ext}`;
}

// Container/mime aliases that don't map 1:1 to a file extension.
const EXT_ALIASES: Record<string, string> = {
  matroska: "mkv",
  "x-matroska": "mkv",
  quicktime: "mov",
  mpegts: "ts",
  "mp2t": "ts",
  "x-msvideo": "avi",
  "x-ms-wmv": "wmv",
  asf: "wmv",
  mpeg4: "mp4",
  "3gpp": "3gp",
  mpga: "mp3",
  mpeg: "mp3",
  "x-wav": "wav",
  wave: "wav",
  "x-m4a": "m4a",
  mp4a: "m4a",
  alac: "m4a",
  "x-flac": "flac",
  oga: "ogg",
  vorbis: "ogg",
};

const KNOWN_VIDEO_EXT = new Set(["mp4", "mkv", "mov", "webm", "avi", "ts", "wmv", "m4v", "3gp"]);
const KNOWN_AUDIO_EXT = new Set(["mp3", "flac", "ogg", "wav", "m4a", "aac", "opus", "wma"]);
const ALL_KNOWN_EXT = [...KNOWN_VIDEO_EXT, ...KNOWN_AUDIO_EXT];

// Accepts a container name ("mkv", "matroska") or a mime type
// ("video/x-matroska", "audio/mpeg") and returns a bare extension recognized
// for the given kind, or undefined if the hint isn't usable.
function extFromHint(hint: string | undefined, kind: "video" | "audio"): string | undefined {
  if (!hint) return undefined;
  const subtype = hint.includes("/") ? hint.split("/")[1] : hint;
  const clean = subtype.split(";")[0].trim().toLowerCase().replace(/^\./, "");
  const known = kind === "audio" ? KNOWN_AUDIO_EXT : KNOWN_VIDEO_EXT;
  if (known.has(clean)) return clean;
  const aliased = EXT_ALIASES[clean];
  if (aliased && known.has(aliased)) return aliased;
  return undefined;
}

// Derives the on-disk extension for a download: from the item's own
// container/mime hint when the caller has one (video containerFormat, or an
// audio mime type), else the historical default per media kind.
function resolveExtension(kind: "video" | "audio", hint?: string): string {
  return extFromHint(hint, kind) ?? (kind === "audio" ? "mp3" : "mp4");
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

export interface InFlightDownload {
  mediaItemId: string;
  title: string;
}

// Downloads currently running but not yet in the persisted index (an item
// only enters that index once .done() fires) - lets the Downloads screen show
// something in-progress instead of it being invisible until it finishes.
export function getInFlightDownloads(): InFlightDownload[] {
  return Array.from(inFlight.keys()).map((mediaItemId) => ({
    mediaItemId,
    title: inFlightTitles.get(mediaItemId) ?? mediaItemId,
  }));
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

/**
 * Free space kept in reserve beyond the file itself. A device driven to zero
 * bytes free fails in ways that have nothing to do with this app.
 */
const DISK_HEADROOM_BYTES = 200 * 1024 * 1024;

export interface StartDownloadOptions {
  /** Defaults to "video" - pass "audio" for music tracks. */
  kind?: "video" | "audio";
  /** Container name or mime type of the source file, e.g. item.containerFormat. */
  formatHint?: string;
  /**
   * Overrides the default POST /api/v1/media/{id}/stream-token mint endpoint.
   * A music track isn't a library media item - it mints through the generic
   * /api/v1/download-token/{id} endpoint instead (same one the web app's
   * mintDownloadToken uses for tracks/albums/collections/playlists).
   */
  tokenUrl?: string;
  /**
   * Overrides the default /api/v1/media/{id}/download path (server-relative,
   * no query string) - a music track downloads from /api/v1/music/tracks/{id}/download.
   */
  downloadPath?: string;
}

export async function startDownload(
  mediaItemId: string,
  title: string,
  onProgress?: (progress: number) => void,
  opts?: StartDownloadOptions,
): Promise<string> {
  assertMediaId(mediaItemId);
  await ensureDir();
  const serverUrl = await getServerUrl();
  if (!serverUrl) throw new Error("Not authenticated");

  const kind = opts?.kind ?? "video";
  const ext = resolveExtension(kind, opts?.formatHint);

  const tokenUrl = opts?.tokenUrl ?? `/api/v1/media/${mediaItemId}/stream-token`;
  const downloadPath = opts?.downloadPath ?? `/api/v1/media/${mediaItemId}/download`;

  const { token } = await eveyFetch<{ token: string }>({
    url: tokenUrl,
    method: "POST",
  });
  const streamUrl = `${serverUrl}${downloadPath}?token=${token}`;
  const destPath = destPathFor(mediaItemId, ext);

  // Keep the unsubscribe so the progress listener is removed when the download
  // settles - otherwise every download permanently leaks its callback into the
  // global progressListeners map (and keeps firing setState on a screen the user
  // has since left).
  const unsubscribeProgress = onProgress ? subscribeToProgress(mediaItemId, onProgress) : undefined;
  inFlight.set(mediaItemId, { totalBytes: 0, downloadedBytes: 0 });
  inFlightTitles.set(mediaItemId, title);

  return new Promise<string>((resolve, reject) => {
    const task = createDownloadTask({
      id: mediaItemId,
      url: streamUrl,
      destination: destPath,
      // ext rides along in the native module's own persisted metadata (same
      // round-trip title already relies on) so rehydrateBackgroundDownloads
      // can rebuild the exact same destPath after an app restart.
      metadata: { title, ext },
    });

    task
      .begin(({ expectedBytes }) => {
        const state = inFlight.get(mediaItemId);
        if (state) state.totalBytes = expectedBytes ?? 0;
        // The size is only knowable here, and this is the last moment before
        // gigabytes start landing on the device. Without this check a 40GB film
        // happily starts on a phone with 2GB free and dies deep into the
        // transfer, having filled the disk on the way.
        //
        // The headroom is not politeness: a device with literally zero bytes
        // free misbehaves well beyond this app.
        if (expectedBytes && expectedBytes > 0) {
          // Optional chaining on purpose: Paths is not present on every
          // platform build, and a missing free-space reading must skip the
          // check rather than crash the download.
          const free = FileSystem.Paths?.availableDiskSpace;
          if (typeof free === "number" && free > 0 && expectedBytes + DISK_HEADROOM_BYTES > free) {
            task.stop();
            inFlight.delete(mediaItemId);
            inFlightTitles.delete(mediaItemId);
            unsubscribeProgress?.();
            void FileSystem.deleteAsync(destPath, { idempotent: true }).catch(() => undefined);
            reject(new Error("NOT_ENOUGH_SPACE"));
          }
        }
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
        inFlightTitles.delete(mediaItemId);
        completeHandler(mediaItemId);
        unsubscribeProgress?.();
        resolve(destPath);
      })
      .error(async ({ error }) => {
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        completeHandler(mediaItemId);
        unsubscribeProgress?.();
        // A failed download leaves a partial file at destPath that never enters
        // the index, so deleteDownload can never reach it - clean it up here.
        // Deletion must never mask the original rejection.
        try { await FileSystem.deleteAsync(destPath, { idempotent: true }); } catch (err) { console.warn("caught", err); }
        reject(new Error(error || "Download failed"));
      });

    // createDownloadTask only registers the task with the native module -
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
    const meta = (task.metadata as { title?: string; ext?: string }) ?? {};
    const title = meta.title ?? mediaItemId;
    // ext falls back to "mp4" for downloads started before this field existed -
    // those were always .mp4, so the fallback reconstructs the exact same path.
    const destPath = destPathFor(mediaItemId, meta.ext ?? "mp4");
    inFlight.set(mediaItemId, { totalBytes: 0, downloadedBytes: 0 });
    inFlightTitles.set(mediaItemId, title);
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
        // The rehydrated task skips begin(), so totalBytes stays 0 if done fires
        // without any progress() call in between. Fall back to the real size on
        // disk rather than persisting a 0 that corrupts disk-usage accounting.
        let totalBytes = state?.totalBytes ?? 0;
        if (!totalBytes) {
          try {
            const info = await FileSystem.getInfoAsync(destPath);
            if (info.exists && info.size) totalBytes = info.size;
          } catch { /* silent */ }
        }
        try {
          await persistDownloaded(mediaItemId, title, destPath, totalBytes);
        } catch { /* silent */ }
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        completeHandler(mediaItemId);
      })
      .error(async () => {
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        completeHandler(mediaItemId);
        // Same leak as startDownload's error handler: the partial file never
        // entered the index, so it's otherwise unreachable by deleteDownload.
        try { await FileSystem.deleteAsync(destPath, { idempotent: true }); } catch (err) { console.warn("caught", err); }
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
  if (info.exists) return item.filePath;
  // The index stores the exact path used at download time, so this normally
  // never runs - kept as a tolerant fallback for an index entry that predates
  // the dynamic extension (or was hand-edited) and no longer matches disk.
  for (const ext of ALL_KNOWN_EXT) {
    const candidate = destPathFor(mediaItemId, ext);
    if (candidate === item.filePath) continue;
    const candidateInfo = await FileSystem.getInfoAsync(candidate);
    if (candidateInfo.exists) return candidate;
  }
  return null;
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
