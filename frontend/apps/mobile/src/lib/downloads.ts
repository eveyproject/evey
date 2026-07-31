// expo-file-system 55 kept getInfoAsync/makeDirectoryAsync/readAsStringAsync/
// writeAsStringAsync/deleteAsync/documentDirectory exported from the package
// root, but every one of them is a stub that THROWS at runtime ("Method X
// imported from expo-file-system is deprecated"). They live at
// expo-file-system/legacy now. Importing them from the root is not a
// deprecation warning, it is a hard failure on the first call - and it took
// out the whole offline download feature silently, because downloads.test.ts
// mocks "expo-file-system" wholesale and never touches the real module.
// Paths is the NEW api and only exists at the root, hence the two imports.
import { Paths } from "expo-file-system";
import * as FileSystem from "expo-file-system/legacy";
import {
  createDownloadTask,
  getExistingDownloadTasks,
  completeHandler,
  directories,
  setConfig,
  type DownloadTask,
} from "@kesha-antonov/react-native-background-downloader";
import { eveyFetch, libraryServiceGetMediaItem } from "@evey/api";
import { getPublicDownloadsDir } from "@evey/media-caps";
import { getServerUrl } from "./server-config";
import { getDeviceAudioCodecs } from "./deviceCodecs";
import { canPlayOffline, OfflineAccessError, type OfflineProfileInfo } from "./offlineAccess";

// Persistent download index: same path as before so existing offline files
// remain visible after the background-downloader migration. The lib writes
// the actual media file under directories.documents (system-managed); we
// keep an app-level JSON index to map mediaItemId → filePath + metadata.
const docDir = FileSystem.documentDirectory ?? "";
const DOWNLOAD_DIR = `${docDir}downloads/`;

export interface DownloadedMedia {
  mediaItemId: string;
  title: string;
  filePath: string;
  fileSize: number;
  downloadedAt: string;
  /**
   * Authorization context captured at download time - who downloaded it and
   * how restricted that content classification was. Absent on entries that
   * predate this field (see offlineAccess.ts's canPlayOffline, which never
   * retroactively blocks those on that basis alone).
   */
  downloadedByProfileId?: string;
  downloadedByIsKids?: boolean;
  downloadedByMaxAge?: number;
  officialRating?: string;
  certificationsByCountry?: Record<string, string>;
  /**
   * Audio codecs of the source file, captured at download time. The download
   * is the RAW remux - if none of these is device-decodable the file plays
   * SILENT, so the player only prefers the local copy over streaming when
   * hasRenderableAudio says so. Absent on entries that predate this field
   * (assumed playable - the historical behavior).
   */
  audioCodecs?: string[];
}

const INDEX_FILE = `${DOWNLOAD_DIR}index.json`;

// Track in-flight downloads in memory so the UI can subscribe to progress
// updates without re-binding callbacks on every render. Cleared on done/error.
// Byte counts ride along with the fraction: an integer percent of a multi-GB
// file reads "0%" for minutes even while the transfer is healthy, so the UI
// needs the raw bytes to show movement (and to derive a transfer rate).
export interface ProgressBytes {
  downloaded: number;
  total: number;
}
type ProgressListener = (pct: number, bytes?: ProgressBytes) => void;
const progressListeners = new Map<string, Set<ProgressListener>>();
const inFlight = new Map<string, { totalBytes: number; downloadedBytes: number }>();
// Title alongside inFlight, kept separately so a download-in-progress can be
// listed in the Downloads screen before it ever reaches the persisted index
// (an item only enters that index once .done() fires).
const inFlightTitles = new Map<string, string>();

// The task handle for a running download, kept so the Downloads screen can
// cancel it (task.stop() below). destPath rides along because stopTask()
// leaves a partial file on disk that .error()/.done() never fires for -
// cancelDownload has to clean it up itself.
interface ActiveDownloadHandle {
  task: DownloadTask;
  destPath: string;
  unsubscribe?: () => void;
}
const activeDownloads = new Map<string, ActiveDownloadHandle>();

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

// The full index entry for one media item - the player reads audioCodecs off
// it to decide whether the local copy can actually be heard on this device.
export async function getDownloadedMedia(mediaItemId: string): Promise<DownloadedMedia | null> {
  const index = await readIndex();
  return index.find((i) => i.mediaItemId === mediaItemId) ?? null;
}

/**
 * Whether the device can produce SOUND for this downloaded file: true when at
 * least one recorded audio track is decodable here (the native player then
 * selects that track), or when the entry predates codec capture (legacy -
 * assumed playable rather than retroactively degraded). Pure so it is
 * exhaustively testable; callers pass getDeviceAudioCodecs().
 */
export function hasRenderableAudio(
  entry: Pick<DownloadedMedia, "audioCodecs">,
  deviceCodecs: string[],
): boolean {
  const codecs = (entry.audioCodecs ?? []).map((c) => c.trim().toLowerCase()).filter(Boolean);
  if (codecs.length === 0) return true;
  const device = new Set(deviceCodecs.map((c) => c.trim().toLowerCase()));
  return codecs.some((c) => device.has(c));
}

// Codecs the on-device post-download audio pass can rewrite to AAC (ac3go's
// domain). DTS/TrueHD are deliberately absent: no on-device pass exists for
// them, so reserving 2x their size would hold storage hostage for a fix that
// never runs (their path is the server-side transcode-to-file, when it lands).
const AUDIO_COMPAT_PASS_CODECS = new Set(["ac3", "eac3"]);

/**
 * Whether this download will need the post-download audio-compat pass to be
 * hearable on this device: no recorded track is device-decodable AND at least
 * one is a codec the pass can rewrite. Pure, like hasRenderableAudio.
 */
export function needsAudioCompatPass(
  entry: Pick<DownloadedMedia, "audioCodecs">,
  deviceCodecs: string[],
): boolean {
  if (hasRenderableAudio(entry, deviceCodecs)) return false;
  return (entry.audioCodecs ?? []).some((c) => AUDIO_COMPAT_PASS_CODECS.has(c.trim().toLowerCase()));
}

/**
 * Free bytes a download must find before it starts. The compat pass remuxes
 * the file next to itself (source + rewritten copy coexist until the swap), so
 * a pass-needing download costs TWICE its size transiently - and that must be
 * refused BEFORE gigabytes land, not when the pass later finds the disk full:
 * paying 19 GB to discover an unfixable silent film is the exact failure the
 * pass exists to remove. Pure for the same testability reason as above.
 */
export function requiredDownloadBytes(expectedBytes: number, compatPass: boolean): number {
  return (compatPass ? expectedBytes * 2 : expectedBytes) + DISK_HEADROOM_BYTES;
}

// The two disk-space rejections are distinct on purpose: "not enough space"
// with 30 GB free reads as a bug unless the message can explain that THIS
// film transiently needs double its size (see requiredDownloadBytes).
export const ERR_NOT_ENOUGH_SPACE = "NOT_ENOUGH_SPACE";
export const ERR_NOT_ENOUGH_SPACE_AUDIO_PASS = "NOT_ENOUGH_SPACE_AUDIO_PASS";

/**
 * The i18n key for a failed download's toast. Every download call site used to
 * re-derive this mapping inline; the decision lives here once so the two space
 * rejections and the generic failure can't drift apart across screens.
 */
export function downloadErrorMessageKey(err: unknown): string {
  const msg = err instanceof Error ? err.message : "";
  switch (msg) {
    case ERR_NOT_ENOUGH_SPACE_AUDIO_PASS:
      return "media.detail.downloadNoSpaceAudioPass";
    case ERR_NOT_ENOUGH_SPACE:
      return "media.detail.downloadNoSpace";
    default:
      return "media.detail.downloadFailed";
  }
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
// This is the app-PRIVATE shape - the legacy location, and still the fallback
// when the platform can't offer a public one; new downloads prefer the public
// Downloads collection (see mediaRoot/safeFilename below).
function destPathFor(mediaItemId: string, ext: string): string {
  return `${directories.documents}/evey-${mediaItemId}.${ext}`;
}

// Where new downloads land: the PUBLIC Downloads collection ("Download/Evey"),
// so the user can open the file with VLC or a file manager - owner decision
// (2026-07-31): the parental gate only guards in-app playback and must not
// dictate storage. Null when the platform can't offer a writable public dir
// (Android < 11, iOS, module absent) - callers then keep the private path.
async function publicMediaRoot(): Promise<string | null> {
  const pub = await getPublicDownloadsDir();
  return pub ? `${pub}/Evey` : null;
}

// Filenames the user will actually see in VLC/file managers: the media title
// cleaned of filesystem-hostile characters, plus a short id suffix so two
// same-titled items never collide. Private-path downloads keep the id-only
// shape (nobody browses that directory).
function safeFilename(title: string, mediaItemId: string, ext: string): string {
  const clean = title
    .replace(/[\\/:*?"<>|]/g, " ")
    .replace(/\s+/g, " ")
    .trim()
    .slice(0, 80)
    .trim();
  return `${clean || "evey"}-${mediaItemId.slice(0, 8)}.${ext}`;
}

// expo-file-system's legacy API only accepts real URIs. directories.documents
// (and therefore every stored filePath) is a BARE absolute path - handed as-is
// to Android it parses with a null scheme and gets probed as an app RESOURCE:
// getInfoAsync answers exists:false for a file that IS on disk (offline
// playback then reported the download missing), and deleteAsync fails inside
// its try/catch (the gigabytes stayed orphaned after "delete"). Every FS call
// on a media file path goes through here. The downloader itself is unaffected:
// createDownloadTask strips a file:// prefix from its destination.
function toFileUri(path: string): string {
  return path.startsWith("file://") ? path : `file://${path}`;
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

function notifyProgress(id: string, pct: number, bytes?: ProgressBytes) {
  const listeners = progressListeners.get(id);
  if (!listeners) return;
  listeners.forEach((cb) => { try { cb(pct, bytes); } catch { /* silent */ } });
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

// Live byte counters of a running download, for first render before any
// progress event has reached the subscriber. null when nothing is in flight.
export function getInFlightBytes(mediaItemId: string): ProgressBytes | null {
  const state = inFlight.get(mediaItemId);
  if (!state) return null;
  return { downloaded: state.downloadedBytes, total: state.totalBytes };
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

// Authorization context captured at the moment a download starts - see
// offlineAccess.ts's canPlayOffline, which consumes these fields later to
// decide whether a DIFFERENT (more restricted) profile may replay this file.
interface DownloadAuthContext {
  downloadedByProfileId?: string;
  downloadedByIsKids?: boolean;
  downloadedByMaxAge?: number;
  officialRating?: string;
  certificationsByCountry?: Record<string, string>;
}

async function persistDownloaded(
  mediaItemId: string,
  title: string,
  destPath: string,
  totalBytes: number,
  authContext?: DownloadAuthContext,
  audioCodecs?: string[],
) {
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
    ...authContext,
    ...(audioCodecs && audioCodecs.length > 0 ? { audioCodecs } : {}),
  });
  await writeIndex(without);
}

/**
 * Free space kept in reserve beyond the file itself. A device driven to zero
 * bytes free fails in ways that have nothing to do with this app.
 */
const DISK_HEADROOM_BYTES = 200 * 1024 * 1024;

export interface StartDownloadOptions extends DownloadAuthContext {
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
  /**
   * Audio codecs of the source file (from the media item's streams) - stored
   * in the index so the player can tell a hearable local copy from one that
   * would play silent on this device (see hasRenderableAudio).
   */
  audioCodecs?: string[];
}

// Native-side logging of the download engine, on once per process. Field
// diagnosis of a stalled download is impossible without it: the library is
// silent by default and a device stuck at 0 bytes leaves no trace in logcat.
let downloaderLogsEnabled = false;
function enableDownloaderLogs() {
  if (downloaderLogsEnabled) return;
  downloaderLogsEnabled = true;
  try {
    setConfig({ isLogsEnabled: true });
  } catch (err) {
    console.warn("downloader setConfig", err);
  }
}

export async function startDownload(
  mediaItemId: string,
  title: string,
  onProgress?: (progress: number, bytes?: ProgressBytes) => void,
  opts?: StartDownloadOptions,
): Promise<string> {
  enableDownloaderLogs();
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
  // Public, VLC-readable destination when the platform offers one; the
  // app-private id-based path otherwise (legacy shape, still what existing
  // index entries point at).
  const pubRoot = await publicMediaRoot();
  const destPath = pubRoot
    ? `${pubRoot}/${safeFilename(title, mediaItemId, ext)}`
    : destPathFor(mediaItemId, ext);

  // Keep the unsubscribe so the progress listener is removed when the download
  // settles - otherwise every download permanently leaks its callback into the
  // global progressListeners map (and keeps firing setState on a screen the user
  // has since left).
  const unsubscribeProgress = onProgress ? subscribeToProgress(mediaItemId, onProgress) : undefined;
  inFlight.set(mediaItemId, { totalBytes: 0, downloadedBytes: 0 });
  inFlightTitles.set(mediaItemId, title);

  const audioCodecs = opts?.audioCodecs;
  // Decided at start (audioCodecs came with the item, the device probe is
  // sync): will this file need the post-download audio pass? Videos only -
  // the pass is a video-container remux, a music track never takes it.
  const compatPass = kind === "video" && needsAudioCompatPass({ audioCodecs }, getDeviceAudioCodecs());
  const authContext: DownloadAuthContext = {
    downloadedByProfileId: opts?.downloadedByProfileId,
    downloadedByIsKids: opts?.downloadedByIsKids,
    downloadedByMaxAge: opts?.downloadedByMaxAge,
    officialRating: opts?.officialRating,
    certificationsByCountry: opts?.certificationsByCountry,
  };

  // Release-build breadcrumbs (logcat tag ReactNativeJS): begin / first byte /
  // settle. Deliberately NOT one line per progress tick.
  let firstProgressLogged = false;
  console.log(`evey-dl start ${mediaItemId} -> ${destPath}`);

  return new Promise<string>((resolve, reject) => {
    const task = createDownloadTask({
      id: mediaItemId,
      url: streamUrl,
      destination: destPath,
      // ext + authContext ride along in the native module's own persisted
      // metadata (same round-trip title already relies on) so
      // rehydrateBackgroundDownloads can rebuild the exact same destPath and
      // offline-access record after an app restart. destPath itself is
      // included since the public/private root choice is no longer derivable
      // from the id alone.
      metadata: { title, ext, authContext, audioCodecs, destPath },
    });

    activeDownloads.set(mediaItemId, { task, destPath, unsubscribe: unsubscribeProgress });

    task
      .begin(({ expectedBytes }) => {
        console.log(`evey-dl begin ${mediaItemId} expected=${expectedBytes}`);
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
          const free = Paths?.availableDiskSpace;
          if (typeof free === "number" && free > 0 && requiredDownloadBytes(expectedBytes, compatPass) > free) {
            console.log(`evey-dl no-space ${mediaItemId} expected=${expectedBytes} free=${free} compatPass=${compatPass}`);
            task.stop();
            inFlight.delete(mediaItemId);
            inFlightTitles.delete(mediaItemId);
            activeDownloads.delete(mediaItemId);
            unsubscribeProgress?.();
            void FileSystem.deleteAsync(toFileUri(destPath), { idempotent: true }).catch(() => undefined);
            reject(new Error(compatPass ? ERR_NOT_ENOUGH_SPACE_AUDIO_PASS : ERR_NOT_ENOUGH_SPACE));
          }
        }
      })
      .progress(({ bytesDownloaded, bytesTotal }) => {
        if (!firstProgressLogged && bytesDownloaded > 0) {
          firstProgressLogged = true;
          console.log(`evey-dl first-bytes ${mediaItemId} ${bytesDownloaded}/${bytesTotal}`);
        }
        const state = inFlight.get(mediaItemId);
        if (state) {
          state.downloadedBytes = bytesDownloaded;
          if (bytesTotal > 0) state.totalBytes = bytesTotal;
        }
        const pct = bytesTotal > 0 ? bytesDownloaded / bytesTotal : NaN;
        notifyProgress(mediaItemId, pct, { downloaded: bytesDownloaded, total: bytesTotal });
      })
      .done(async () => {
        console.log(`evey-dl done ${mediaItemId}`);
        const state = inFlight.get(mediaItemId);
        const totalBytes = state?.totalBytes ?? 0;
        try {
          await persistDownloaded(mediaItemId, title, destPath, totalBytes, authContext, audioCodecs);
        } catch (err) {
          if (process.env.NODE_ENV !== "production") console.warn("persist download index", err);
        }
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        activeDownloads.delete(mediaItemId);
        completeHandler(mediaItemId);
        unsubscribeProgress?.();
        resolve(destPath);
      })
      .error(async ({ error }) => {
        console.warn(`evey-dl error ${mediaItemId}: ${error}`);
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        activeDownloads.delete(mediaItemId);
        completeHandler(mediaItemId);
        unsubscribeProgress?.();
        // A failed download leaves a partial file at destPath that never enters
        // the index, so deleteDownload can never reach it - clean it up here.
        // Deletion must never mask the original rejection.
        try { await FileSystem.deleteAsync(toFileUri(destPath), { idempotent: true }); } catch (err) { console.warn("caught", err); }
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
  enableDownloaderLogs();
  let tasks: Awaited<ReturnType<typeof getExistingDownloadTasks>>;
  try {
    tasks = await getExistingDownloadTasks();
  } catch (err) {
    if (process.env.NODE_ENV !== "production") console.warn("rehydrate downloads", err);
    return;
  }
  for (const task of tasks) {
    const mediaItemId = task.id;
    const meta = (task.metadata as { title?: string; ext?: string; authContext?: DownloadAuthContext; audioCodecs?: string[]; destPath?: string }) ?? {};
    const title = meta.title ?? mediaItemId;
    // The task's own recorded destination is authoritative (public/private
    // root is not derivable from the id). The id-based reconstruction only
    // covers tasks started before destPath rode along in the metadata - those
    // were always private .mp4-or-recorded-ext paths.
    const destPath = meta.destPath ?? destPathFor(mediaItemId, meta.ext ?? "mp4");
    // authContext is absent for downloads started before this field existed -
    // persistDownloaded then simply omits it (legacy entry, see offlineAccess.ts).
    const authContext = meta.authContext;
    const audioCodecs = meta.audioCodecs;
    inFlight.set(mediaItemId, { totalBytes: 0, downloadedBytes: 0 });
    inFlightTitles.set(mediaItemId, title);
    activeDownloads.set(mediaItemId, { task, destPath });
    task
      .progress(({ bytesDownloaded, bytesTotal }) => {
        const state = inFlight.get(mediaItemId);
        if (state) {
          state.downloadedBytes = bytesDownloaded;
          if (bytesTotal > 0) state.totalBytes = bytesTotal;
        }
        const pct = bytesTotal > 0 ? bytesDownloaded / bytesTotal : NaN;
        notifyProgress(mediaItemId, pct, { downloaded: bytesDownloaded, total: bytesTotal });
      })
      .done(async () => {
        const state = inFlight.get(mediaItemId);
        // The rehydrated task skips begin(), so totalBytes stays 0 if done fires
        // without any progress() call in between. Fall back to the real size on
        // disk rather than persisting a 0 that corrupts disk-usage accounting.
        let totalBytes = state?.totalBytes ?? 0;
        if (!totalBytes) {
          try {
            const info = await FileSystem.getInfoAsync(toFileUri(destPath));
            if (info.exists && info.size) totalBytes = info.size;
          } catch { /* silent */ }
        }
        try {
          await persistDownloaded(mediaItemId, title, destPath, totalBytes, authContext, audioCodecs);
        } catch { /* silent */ }
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        activeDownloads.delete(mediaItemId);
        completeHandler(mediaItemId);
      })
      .error(async () => {
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        activeDownloads.delete(mediaItemId);
        completeHandler(mediaItemId);
        // Same leak as startDownload's error handler: the partial file never
        // entered the index, so it's otherwise unreachable by deleteDownload.
        try { await FileSystem.deleteAsync(toFileUri(destPath), { idempotent: true }); } catch (err) { console.warn("caught", err); }
      });
  }
}

// Whether mediaItemId has a download currently running that can be stopped -
// the Downloads screen uses this to decide whether to render a cancel action
// on an in-flight row.
export function isDownloadCancelable(mediaItemId: string): boolean {
  return activeDownloads.has(mediaItemId);
}

/**
 * Cancels a running download. Reuses the same background-downloader task
 * handle startDownload/rehydrateBackgroundDownloads register - stopTask()
 * cancels the native transfer but (unlike done()/error()) never fires a JS
 * event, so the in-flight bookkeeping and the partial file it leaves behind
 * have to be cleaned up here rather than relying on the task's own handlers.
 */
export async function cancelDownload(mediaItemId: string): Promise<void> {
  const handle = activeDownloads.get(mediaItemId);
  if (!handle) return;
  activeDownloads.delete(mediaItemId);
  inFlight.delete(mediaItemId);
  inFlightTitles.delete(mediaItemId);
  handle.unsubscribe?.();
  try { await handle.task.stop(); } catch (err) { console.warn("caught", err); }
  completeHandler(mediaItemId);
  try { await FileSystem.deleteAsync(toFileUri(handle.destPath), { idempotent: true }); } catch (err) { console.warn("caught", err); }
}

export async function deleteDownload(mediaItemId: string): Promise<void> {
  const index = await readIndex();
  const item = index.find((i) => i.mediaItemId === mediaItemId);
  if (item) {
    try { await FileSystem.deleteAsync(toFileUri(item.filePath), { idempotent: true }); } catch (err) { console.warn("caught", err); }
  }
  await writeIndex(index.filter((i) => i.mediaItemId !== mediaItemId));
}

async function resolveExistingPath(item: DownloadedMedia, mediaItemId: string): Promise<string | null> {
  const info = await FileSystem.getInfoAsync(toFileUri(item.filePath));
  if (info.exists) return item.filePath;
  // The index stores the exact path used at download time, so this normally
  // never runs - kept as a tolerant fallback for an index entry that predates
  // the dynamic extension (or was hand-edited) and no longer matches disk.
  for (const ext of ALL_KNOWN_EXT) {
    const candidate = destPathFor(mediaItemId, ext);
    if (candidate === item.filePath) continue;
    const candidateInfo = await FileSystem.getInfoAsync(toFileUri(candidate));
    if (candidateInfo.exists) return candidate;
  }
  return null;
}

// Developer-actionable messages for OfflineAccessError - not shown to end
// users verbatim; the UI maps `.reason` to a translated string instead (see
// downloads.tsx).
function offlineDenialMessage(reason: OfflineAccessError["reason"]): string {
  switch (reason) {
    case "expired":
      return "Download is past its offline TTL and must be re-downloaded.";
    case "age-gated":
      return "Download exceeds the active profile's parental age cap.";
    case "more-restricted-profile":
      return "Download belongs to a less restricted profile than the one active now.";
    case "unknown-profile":
      return "No active profile is available to authorize this download.";
    case "revoked":
      return "Server revoked access to this title (library access or parental settings changed).";
  }
}

export interface GetDownloadPathOptions {
  /** The profile currently active on the device. Omitted = legacy behavior
   * (existence check only, no offline-access gate) for call sites not yet
   * migrated to pass it. */
  activeProfile?: OfflineProfileInfo;
  /** Injectable clock for tests; defaults to Date.now(). */
  now?: number;
}

/**
 * Resolves the on-disk path for a downloaded media item, or throws
 * OfflineAccessError when the ACTIVE profile is not authorized to replay it
 * (expired, age-gated, or downloaded by a less restricted profile - see
 * offlineAccess.ts). When online, also revalidates against the server (an
 * existing generated call, not a new endpoint) since library access or
 * parental settings may have been revoked since the download; only an
 * explicit 401/403/404 is treated as revocation so a genuinely offline
 * device (e.g. on a flight) keeps playing from the local decision alone.
 */
export async function getDownloadPath(
  mediaItemId: string,
  options?: GetDownloadPathOptions,
): Promise<string | null> {
  const index = await readIndex();
  const item = index.find((i) => i.mediaItemId === mediaItemId);
  if (!item) return null;

  const filePath = await resolveExistingPath(item, mediaItemId);
  if (!filePath) return null;

  if (options?.activeProfile) {
    const decision = canPlayOffline(item, options.activeProfile, options.now ?? Date.now());
    if (!decision.allowed) {
      throw new OfflineAccessError(decision.reason!, offlineDenialMessage(decision.reason!));
    }

    try {
      await libraryServiceGetMediaItem(mediaItemId);
    } catch (err) {
      const status = (err as { httpStatus?: number } | undefined)?.httpStatus;
      if (status === 401 || status === 403 || status === 404) {
        throw new OfflineAccessError("revoked", offlineDenialMessage("revoked"));
      }
      // Any other failure (offline, timeout, 5xx) - fall back to the local
      // decision computed above; don't let a flaky/absent network block
      // legitimate offline playback.
    }
  }

  return filePath;
}

export async function getTotalDownloadSize(): Promise<number> {
  const index = await readIndex();
  let total = 0;
  for (const item of index) {
    try {
      const info = await FileSystem.getInfoAsync(toFileUri(item.filePath));
      if (info.exists && info.size) total += info.size;
    } catch (err) { console.warn("caught", err); }
  }
  return total;
}
