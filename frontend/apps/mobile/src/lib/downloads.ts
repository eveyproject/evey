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
import { deleteMediaFile, getPublicDownloadsDir } from "@evey/media-caps";
import { getServerUrl } from "./server-config";
import { getDeviceAudioCodecs } from "./deviceCodecs";
import { canPlayOffline, OfflineAccessError, type OfflineProfileInfo } from "./offlineAccess";
import { ensureNotificationPermission } from "./notificationPermission";
import { artworkUrl } from "./artwork";

// Persistent download index: same path as before so existing offline files
// remain visible after the background-downloader migration. The lib writes
// the actual media file under directories.documents (system-managed); we
// keep an app-level JSON index to map mediaItemId → filePath + metadata.
const docDir = FileSystem.documentDirectory ?? "";
const DOWNLOAD_DIR = `${docDir}downloads/`;

// Poster width cached for the Downloads list: the row thumbnail is ~48dp, so
// the server-side resize keeps these at a few KB each instead of full-size.
const POSTER_CACHE_WIDTH = 200;

/**
 * What the Downloads screen needs to NAME and PICTURE an item without the
 * server: an episode's own title ("The Spider and the Fly") names nothing on
 * its own, and the poster is served over HTTP - unreachable exactly when this
 * screen matters. Both are captured at download time; every field is optional
 * because entries that predate this simply render as they did before.
 */
export interface DownloadDisplay {
  seriesTitle?: string;
  seasonNumber?: number;
  episodeNumber?: number;
  /** Local file of the poster copied at download time (no network to render). */
  posterPath?: string;
}

export interface DownloadedMedia extends DownloadDisplay {
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
  /**
   * Set when the post-download audio-compat pass failed TERMINALLY for this
   * file (unsupported codec, unreadable container...). Terminal means
   * deterministic: retrying would fail identically, so the startup sweep
   * skips the entry instead of re-walking gigabytes every boot. Transient
   * failures (crash, cancel) never set it and are retried.
   */
  audioPassFailed?: string;
}

const INDEX_FILE = `${DOWNLOAD_DIR}index.json`;

// Sidecar record of every download BETWEEN start and settle. The native lib
// emits its completion event exactly once and then destroys the task state -
// if the OS had frozen the JS thread at that moment (ColorOS does, seen
// 08-01: a fully-written 1.85 GB file, a 99% display, an empty index), the
// event is gone and rehydrate finds nothing to re-attach. These records are
// what makes that loss recoverable: at startup, a record whose task no longer
// exists is reconciled against the bytes actually on disk.
const PENDING_FILE = `${DOWNLOAD_DIR}pending.json`;

interface PendingDownload {
  mediaItemId: string;
  title: string;
  destPath: string;
  /** Filled in at begin(); 0/absent means the transfer died before begin. */
  expectedBytes?: number;
  authContext?: DownloadAuthContext;
  audioCodecs?: string[];
  display?: DownloadDisplay;
  startedAt: string;
}

async function readPending(): Promise<PendingDownload[]> {
  try {
    const raw = await FileSystem.readAsStringAsync(PENDING_FILE);
    return JSON.parse(raw);
  } catch {
    return [];
  }
}

async function writePending(items: PendingDownload[]) {
  await ensureDir();
  await FileSystem.writeAsStringAsync(PENDING_FILE, JSON.stringify(items));
}

async function setPending(rec: PendingDownload) {
  const rest = (await readPending()).filter((p) => p.mediaItemId !== rec.mediaItemId);
  await writePending([...rest, rec]);
}

async function clearPending(mediaItemId: string) {
  const all = await readPending();
  const rest = all.filter((p) => p.mediaItemId !== mediaItemId);
  if (rest.length !== all.length) await writePending(rest);
}

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
export function toFileUri(path: string): string {
  return path.startsWith("file://") ? path : `file://${path}`;
}

/**
 * Delete a media file wherever it lives. expo-file-system's legacy API only
 * operates inside the app's own directories and REJECTS public-Downloads
 * paths (even for files this app created), so public files go through the
 * plain-File native helper first and the expo call covers the private shapes.
 * Never throws - deletion is cleanup, and the callers' own bookkeeping (index
 * writes, task teardown) must not die on a file that is already gone.
 */
async function deleteMediaFileAnywhere(path: string): Promise<void> {
  try {
    if (await deleteMediaFile(path)) return;
  } catch {
    /* fall through to the expo attempt */
  }
  try {
    await FileSystem.deleteAsync(toFileUri(path), { idempotent: true });
  } catch (err) {
    console.warn(`evey-dl delete failed ${path}`, err);
  }
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
export interface DownloadAuthContext {
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
  display?: DownloadDisplay,
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
    ...display,
  });
  await writeIndex(without);
}

/**
 * Copy the item's poster next to the download so the Downloads screen has a
 * picture with no network. Best-effort by design: a missing or failed poster
 * must never fail (or delay) the media download itself - the screen falls back
 * to its icon. The artwork route is public, so no token is involved.
 */
export async function cacheDownloadPoster(mediaItemId: string): Promise<string | undefined> {
  try {
    const serverUrl = await getServerUrl();
    if (!serverUrl) return undefined;
    await ensureDir();
    const dest = `${DOWNLOAD_DIR}poster-${encodeURIComponent(mediaItemId)}.jpg`;
    const res = await FileSystem.downloadAsync(artworkUrl(serverUrl, mediaItemId, "poster", POSTER_CACHE_WIDTH), dest);
    if (res.status !== 200) {
      await FileSystem.deleteAsync(dest, { idempotent: true }).catch(() => undefined);
      return undefined;
    }
    return dest;
  } catch {
    return undefined;
  }
}

// --- seam for the sliced reduced-quality download (reducedDownload.ts) ------
// That flow builds its file itself (N slices collected, then joined) instead
// of handing one URL to the background engine, but it must land in exactly the
// same index, the same destination convention and the same progress stream as
// every other download. These are the only doors it needs; the index format
// and the path rules stay this module's private property.

/** Where a finished download of this title should live: the public Downloads
 * collection when the platform offers one, the app-private path otherwise. */
export async function resolveDownloadDest(title: string, mediaItemId: string, ext: string): Promise<string> {
  const pubRoot = await publicMediaRoot();
  return pubRoot ? `${pubRoot}/${safeFilename(title, mediaItemId, ext)}` : destPathFor(mediaItemId, ext);
}

/** Add (or replace) the index entry for a file this module did not transfer. */
export async function recordDownloadedFile(
  mediaItemId: string,
  title: string,
  destPath: string,
  totalBytes: number,
  authContext?: DownloadAuthContext,
  audioCodecs?: string[],
): Promise<void> {
  await persistDownloaded(mediaItemId, title, destPath, totalBytes, authContext, audioCodecs);
}

/** List the item as in-flight so the Downloads screen shows it before it ever
 * reaches the index (an entry only lands there once the file is complete). */
export function registerDownloadInFlight(mediaItemId: string, title: string): void {
  inFlight.set(mediaItemId, { totalBytes: 0, downloadedBytes: 0 });
  inFlightTitles.set(mediaItemId, title);
}

export function clearDownloadInFlight(mediaItemId: string): void {
  inFlight.delete(mediaItemId);
  inFlightTitles.delete(mediaItemId);
}

/** Push a 0..1 completion to the screens subscribed to this item. */
export function notifyDownloadProgress(mediaItemId: string, pct: number, bytes?: ProgressBytes): void {
  notifyProgress(mediaItemId, pct, bytes);
}

export async function deleteDownloadedFileAnywhere(path: string): Promise<void> {
  await deleteMediaFileAnywhere(path);
}

/**
 * Post-pass index rewrite, in downloads.ts because the index format is this
 * module's private property. The compat pass hands back what changed: the
 * file shrank (AC-3 448k became AAC 160k) and one codec became "aac" - or,
 * on a terminal failure, the code to remember so the sweep stops retrying.
 */
export async function updateDownloadedAudio(
  mediaItemId: string,
  patch: Partial<Pick<DownloadedMedia, "fileSize" | "audioCodecs" | "audioPassFailed" | "filePath">>,
): Promise<void> {
  const index = await readIndex();
  const item = index.find((i) => i.mediaItemId === mediaItemId);
  if (!item) return;
  Object.assign(item, patch);
  await writeIndex(index);
}

// Audio codecs of the source file, from the one endpoint that hydrates
// streams. List-fed callers (episode rows) have none to give - ListMediaItems
// returns streams empty, and treating that gap as "renderable" is how an
// eac3-only episode reached the disk raw and played silent.
async function fetchSourceAudioCodecs(mediaItemId: string): Promise<string[] | undefined> {
  try {
    const item = (await libraryServiceGetMediaItem(mediaItemId)) as {
      streams?: { type?: string; codec?: string }[];
    };
    const codecs = (item?.streams ?? [])
      .filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO" && s.codec)
      .map((s) => s.codec as string);
    return codecs.length > 0 ? codecs : undefined;
  } catch {
    return undefined;
  }
}

/**
 * Same lookup for an entry already on disk (the boot sweep's self-heal), with
 * the answer persisted so the player and later sweeps stop asking. Undefined
 * when the server is unreachable or has no stream info - retryable, never
 * recorded.
 */
export async function refreshDownloadedAudioCodecs(mediaItemId: string): Promise<string[] | undefined> {
  const codecs = await fetchSourceAudioCodecs(mediaItemId);
  if (!codecs) return undefined;
  await updateDownloadedAudio(mediaItemId, { audioCodecs: codecs });
  return codecs;
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
  /**
   * Series/season/episode context so the Downloads screen can name an episode
   * offline. The poster is fetched here (not passed in): the caller has the
   * item, not the bytes.
   */
  display?: Omit<DownloadDisplay, "posterPath">;
}

// --- Server-prepared compat download ---------------------------------------
// When the source's audio is unrenderable here, the server can prepare (once
// per title, cached for everyone) a variant whose audio is stereo AAC. The
// phone then downloads a file that just plays, instead of re-encoding for
// minutes on battery. See prepareCompatDownload for the failure contract.

const COMPAT_POLL_MS = 4000;
// Bounded by the slowest realistic production (a 4h film at ~10x realtime on
// the weak-NAS target); past this the raw-download fallback is the better
// experience than an open-ended wait.
const COMPAT_PREPARE_TIMEOUT_MS = 30 * 60 * 1000;

const preparingCompat = new Map<string, { cancelled: boolean; permille: number }>();

// The Downloads screen shows "preparing on the server..." on the in-flight
// row while this is true (its poll loop picks the change up).
export function isPreparingCompat(mediaItemId: string): boolean {
  return preparingCompat.has(mediaItemId);
}

// Server-side production progress, 0..1000 (0 = not started/unknown). What
// turns a minutes-long "preparing" label from "it hung" into "it is working".
export function getCompatPreparePermille(mediaItemId: string): number {
  return preparingCompat.get(mediaItemId)?.permille ?? 0;
}

function sleep(ms: number): Promise<void> {
  return new Promise((r) => setTimeout(r, ms));
}

/**
 * Ask the server for the compat variant and wait until it is ready. Every
 * refusal shape - endpoint absent (old server), feature off, production
 * failure, timeout, network - collapses to "unavailable", which the caller
 * treats as "download the raw file like always" (the on-device pass remains
 * the net). "cancelled" is the user's own abort, surfaced separately so the
 * download rejects instead of silently continuing raw.
 */
async function prepareCompatDownload(mediaItemId: string, deviceAudio: string[]): Promise<"ready" | "unavailable" | "cancelled"> {
  const flag = { cancelled: false, permille: 0 };
  preparingCompat.set(mediaItemId, flag);
  try {
    let status: string;
    try {
      status = (
        await eveyFetch<{ status: string }>({
          // The declaration lets the server refuse a pointless production
          // ("unnecessary") if this device could in fact render a track -
          // the owner's invariant enforced on BOTH sides of the wire.
          url: `/api/v1/media/${mediaItemId}/compat-download?renderable=${encodeURIComponent(deviceAudio.join(","))}`,
          method: "POST",
        })
      ).status;
    } catch (err) {
      console.log(`evey-dl compat unavailable ${mediaItemId}: ${err instanceof Error ? err.message : err}`);
      return "unavailable";
    }
    console.log(`evey-dl compat ${mediaItemId} ${status}`);
    const deadline = Date.now() + COMPAT_PREPARE_TIMEOUT_MS;
    while (status === "preparing" && Date.now() < deadline && !flag.cancelled) {
      await sleep(COMPAT_POLL_MS);
      if (flag.cancelled) break;
      try {
        const st = await eveyFetch<{ status: string; error?: string; permille?: number }>({
          url: `/api/v1/media/${mediaItemId}/compat-download/status`,
          method: "GET",
        });
        status = st.status;
        if (typeof st.permille === "number" && st.permille > flag.permille) {
          flag.permille = st.permille;
          notifyProgress(mediaItemId, NaN); // wake the fiche's subscription for a re-read
        }
      } catch {
        // One dropped poll must not abandon a production that is still
        // running server-side; the deadline bounds the overall wait.
      }
    }
    if (flag.cancelled) return "cancelled";
    if (status === "ready") {
      console.log(`evey-dl compat ready ${mediaItemId}`);
      return "ready";
    }
    console.log(`evey-dl compat gave up ${mediaItemId} (status=${status})`);
    return "unavailable";
  } finally {
    preparingCompat.delete(mediaItemId);
  }
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
  // The progress rides the foreground service's notification, which Android
  // 13+ suppresses until POST_NOTIFICATIONS is granted - and only the cast
  // flow asked for it, so a user who never cast downloaded blind. The tap on
  // Download is the user-activation moment; a denial degrades to exactly the
  // old behavior (silent service), never blocks the download.
  await ensureNotificationPermission();
  await ensureDir();
  const serverUrl = await getServerUrl();
  if (!serverUrl) throw new Error("Not authenticated");

  const kind = opts?.kind ?? "video";
  let ext = resolveExtension(kind, opts?.formatHint);

  const tokenUrl = opts?.tokenUrl ?? `/api/v1/media/${mediaItemId}/stream-token`;
  let downloadPath = opts?.downloadPath ?? `/api/v1/media/${mediaItemId}/download`;

  const { token } = await eveyFetch<{ token: string }>({
    url: tokenUrl,
    method: "POST",
  });

  // Keep the unsubscribe so the progress listener is removed when the download
  // settles - otherwise every download permanently leaks its callback into the
  // global progressListeners map (and keeps firing setState on a screen the user
  // has since left).
  const unsubscribeProgress = onProgress ? subscribeToProgress(mediaItemId, onProgress) : undefined;
  inFlight.set(mediaItemId, { totalBytes: 0, downloadedBytes: 0 });
  inFlightTitles.set(mediaItemId, title);

  let audioCodecs = opts?.audioCodecs;
  if (kind === "video" && !audioCodecs?.length) {
    audioCodecs = await fetchSourceAudioCodecs(mediaItemId);
  }
  const deviceAudio = getDeviceAudioCodecs();
  // Decided at start (audioCodecs came with the item, the device probe is
  // sync): will this file need the post-download audio pass? Videos only -
  // the pass is a video-container remux, a music track never takes it.
  let compatPass = kind === "video" && needsAudioCompatPass({ audioCodecs }, deviceAudio);
  // The server path triggers on the OWNER'S invariant - the device says it
  // cannot render ANY recorded track - which is wider than the local pass's
  // scope: ffmpeg on the server also covers DTS/TrueHD, codecs the embedded
  // decoder must refuse. (Empty codec list = legacy entry, assumed playable.)
  const serverCompatWanted =
    kind === "video" && (audioCodecs?.length ?? 0) > 0 && !hasRenderableAudio({ audioCodecs }, deviceAudio);

  // The unrenderable case first tries the SERVER-prepared variant: the audio
  // is transcoded once per title on the server (seconds to minutes, cached
  // for every device) instead of minutes of CPU on this phone. Everything
  // about the failure side falls back to today's behavior - raw download
  // plus the on-device pass - so an old or opted-out server changes nothing.
  if (serverCompatWanted && !opts?.downloadPath) {
    const outcome = await prepareCompatDownload(mediaItemId, deviceAudio);
    if (outcome === "cancelled") {
      inFlight.delete(mediaItemId);
      inFlightTitles.delete(mediaItemId);
      unsubscribeProgress?.();
      throw new Error("Download cancelled");
    }
    if (outcome === "ready") {
      downloadPath = `/api/v1/media/${mediaItemId}/compat-download`;
      ext = "mkv"; // the compat variant is always Matroska
      audioCodecs = ["aac"]; // its truth, not the source's - the pass must not re-run
      compatPass = false; // and the 2x space reservation is no longer needed
    }
  }

  const streamUrl = `${serverUrl}${downloadPath}?token=${token}`;
  // Public, VLC-readable destination when the platform offers one; the
  // app-private id-based path otherwise (legacy shape, still what existing
  // index entries point at). One answer for every download shape, sliced
  // included - see resolveDownloadDest.
  const destPath = await resolveDownloadDest(title, mediaItemId, ext);

  const authContext: DownloadAuthContext = {
    downloadedByProfileId: opts?.downloadedByProfileId,
    downloadedByIsKids: opts?.downloadedByIsKids,
    downloadedByMaxAge: opts?.downloadedByMaxAge,
    officialRating: opts?.officialRating,
    certificationsByCountry: opts?.certificationsByCountry,
  };
  // Fetched before the transfer so the row has its picture as soon as it
  // lands; a failure here is silent and only costs the icon fallback.
  const display: DownloadDisplay = {
    ...opts?.display,
    posterPath: opts?.kind === "audio" ? undefined : await cacheDownloadPoster(mediaItemId),
  };

  // Release-build breadcrumbs (logcat tag ReactNativeJS): begin / first byte /
  // settle. Deliberately NOT one line per progress tick.
  let firstProgressLogged = false;
  console.log(`evey-dl start ${mediaItemId} -> ${destPath}`);

  // Written BEFORE the transfer starts so a completion whose event gets lost
  // (JS frozen at that instant) is recoverable at next startup - see
  // reconcilePendingDownloads. Failure to write it must not block the
  // download: the record is a safety net, not a precondition.
  const pendingRecord: PendingDownload = {
    mediaItemId,
    title,
    destPath,
    authContext,
    audioCodecs,
    display,
    startedAt: new Date().toISOString(),
  };
  try {
    await setPending(pendingRecord);
  } catch (err) {
    console.warn("evey-dl pending record", err);
  }

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
      metadata: { title, ext, authContext, audioCodecs, display, destPath },
    });

    activeDownloads.set(mediaItemId, { task, destPath, unsubscribe: unsubscribeProgress });

    task
      .begin(({ expectedBytes }) => {
        console.log(`evey-dl begin ${mediaItemId} expected=${expectedBytes}`);
        const state = inFlight.get(mediaItemId);
        if (state) state.totalBytes = expectedBytes ?? 0;
        // The reconciler can only call a file "complete" against this figure.
        if (expectedBytes && expectedBytes > 0) {
          pendingRecord.expectedBytes = expectedBytes;
          void setPending(pendingRecord).catch(() => undefined);
        }
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
            void deleteMediaFileAnywhere(destPath);
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
          await persistDownloaded(mediaItemId, title, destPath, totalBytes, authContext, audioCodecs, display);
        } catch (err) {
          if (process.env.NODE_ENV !== "production") console.warn("persist download index", err);
        }
        await clearPending(mediaItemId).catch(() => undefined);
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        activeDownloads.delete(mediaItemId);
        completeHandler(mediaItemId);
        unsubscribeProgress?.();
        resolve(destPath);
        // Fire-and-forget: the download IS complete (promise resolved above);
        // the compat pass is a separate best-effort stage that must never turn
        // a finished download into a rejection. Dynamic import so the download
        // path never pays for the pass machinery when no file needs it.
        void import("./audioCompatPass")
          .then((m) => m.runAudioCompatPass(mediaItemId))
          .catch((err) => console.warn("evey-audiopass kickoff", err));
      })
      .error(async ({ error }) => {
        console.warn(`evey-dl error ${mediaItemId}: ${error}`);
        await clearPending(mediaItemId).catch(() => undefined);
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        activeDownloads.delete(mediaItemId);
        completeHandler(mediaItemId);
        unsubscribeProgress?.();
        // A failed download leaves a partial file at destPath that never enters
        // the index, so deleteDownload can never reach it - clean it up here.
        // Deletion must never mask the original rejection.
        await deleteMediaFileAnywhere(destPath);
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
    const meta = (task.metadata as { title?: string; ext?: string; authContext?: DownloadAuthContext; audioCodecs?: string[]; display?: DownloadDisplay; destPath?: string }) ?? {};
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
    // Rides along in the native module's metadata so a download recovered
    // after an app kill keeps its poster and its series/episode labels.
    const display = meta.display;
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
          await persistDownloaded(mediaItemId, title, destPath, totalBytes, authContext, audioCodecs, display);
        } catch { /* silent */ }
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        activeDownloads.delete(mediaItemId);
        completeHandler(mediaItemId);
        void import("./audioCompatPass")
          .then((m) => m.runAudioCompatPass(mediaItemId))
          .catch((err) => console.warn("evey-audiopass kickoff", err));
      })
      .error(async () => {
        inFlight.delete(mediaItemId);
        inFlightTitles.delete(mediaItemId);
        activeDownloads.delete(mediaItemId);
        completeHandler(mediaItemId);
        // Same leak as startDownload's error handler: the partial file never
        // entered the index, so it's otherwise unreachable by deleteDownload.
        await deleteMediaFileAnywhere(destPath);
      });
  }

  // Runs after the live tasks are re-wired so their records are recognized as
  // live; must run BEFORE purgeOrphanedDownloads (a complete-but-unindexed
  // file is exactly what the purge would misread as an orphan).
  const liveIds = new Set(tasks.map((t) => t.id));
  await reconcilePendingDownloads(liveIds);
}

/**
 * Settle every pending record whose native task VANISHED without ever firing
 * done/error - the lib emits completion exactly once and then destroys the
 * task state, so a JS thread frozen at that moment (app backgrounded on an
 * aggressive OEM) loses the event forever. The bytes on disk are the ground
 * truth this falls back on: a file at least as large as the transfer's
 * expected size IS the completed download, and is indexed as such; anything
 * short of that is an unresumable partial and is dropped.
 */
async function reconcilePendingDownloads(liveIds: Set<string>): Promise<void> {
  let pending: PendingDownload[];
  try {
    pending = await readPending();
  } catch {
    return;
  }
  for (const rec of pending) {
    if (liveIds.has(rec.mediaItemId)) continue;
    try {
      const already = await getDownloadedMedia(rec.mediaItemId);
      if (already) {
        await clearPending(rec.mediaItemId);
        continue;
      }
      const info = await FileSystem.getInfoAsync(toFileUri(rec.destPath));
      if (info.exists && rec.expectedBytes && rec.expectedBytes > 0 && (info.size ?? 0) >= rec.expectedBytes) {
        console.log(`evey-dl reconcile ${rec.mediaItemId}: complete on disk (${info.size}), indexing lost completion`);
        await persistDownloaded(rec.mediaItemId, rec.title, rec.destPath, info.size ?? rec.expectedBytes, rec.authContext, rec.audioCodecs);
        await clearPending(rec.mediaItemId);
        // Same follow-up a live completion gets. The import cannot be
        // hoisted: audioCompatPass imports back into this module.
        void import("./audioCompatPass")
          .then((m) => m.runAudioCompatPass(rec.mediaItemId))
          .catch((err) => console.warn("evey-audiopass kickoff", err));
        continue;
      }
      if (info.exists && rec.expectedBytes && rec.expectedBytes > 0) {
        console.log(`evey-dl reconcile ${rec.mediaItemId}: partial without a task (${info.size ?? 0}/${rec.expectedBytes}), dropping`);
        await deleteMediaFileAnywhere(rec.destPath);
      } else if (info.exists) {
        // Died before begin() recorded the expected size: the file can't be
        // told complete from truncated. Keep the bytes, forget the record - a
        // re-download overwrites the same path anyway.
        console.log(`evey-dl reconcile ${rec.mediaItemId}: unverifiable file (no expected size), keeping bytes`);
      } else {
        console.log(`evey-dl reconcile ${rec.mediaItemId}: nothing on disk, forgetting`);
      }
      await clearPending(rec.mediaItemId);
    } catch (err) {
      console.warn(`evey-dl reconcile ${rec.mediaItemId}`, err);
    }
  }
}

// Whether mediaItemId has a download currently running that can be stopped.
// No production caller: the Downloads screen renders its cancel action off the
// index's inProgress flag. This is the observation seam the cancel tests need -
// it is the only view that also covers the compat-preparation phase, which
// getInFlightDownloads (tasks only) cannot see.
export function isDownloadCancelable(mediaItemId: string): boolean {
  return activeDownloads.has(mediaItemId) || preparingCompat.has(mediaItemId);
}

/**
 * Cancels a running download. Reuses the same background-downloader task
 * handle startDownload/rehydrateBackgroundDownloads register - stopTask()
 * cancels the native transfer but (unlike done()/error()) never fires a JS
 * event, so the in-flight bookkeeping and the partial file it leaves behind
 * have to be cleaned up here rather than relying on the task's own handlers.
 */
export async function cancelDownload(mediaItemId: string): Promise<void> {
  // Still in the server-preparation phase: no task exists yet. Raising the
  // flag makes startDownload's own flow clean up and reject.
  const prep = preparingCompat.get(mediaItemId);
  if (prep) {
    prep.cancelled = true;
    return;
  }
  // A sliced reduced download owns its own transfer loop (and may be waiting
  // on the server rather than downloading), so it cancels itself.
  const reduced = await import("./reducedDownload");
  if (reduced.cancelReducedDownload(mediaItemId)) return;
  const handle = activeDownloads.get(mediaItemId);
  if (!handle) return;
  await clearPending(mediaItemId).catch(() => undefined);
  activeDownloads.delete(mediaItemId);
  inFlight.delete(mediaItemId);
  inFlightTitles.delete(mediaItemId);
  handle.unsubscribe?.();
  try { await handle.task.stop(); } catch (err) { console.warn("caught", err); }
  completeHandler(mediaItemId);
  await deleteMediaFileAnywhere(handle.destPath);
}

export async function deleteDownload(mediaItemId: string): Promise<void> {
  const index = await readIndex();
  const item = index.find((i) => i.mediaItemId === mediaItemId);
  if (item) {
    await deleteMediaFileAnywhere(item.filePath);
    // The cached poster is ours (app-private), so it goes with the media -
    // otherwise every deleted download leaves a thumbnail behind forever.
    if (item.posterPath) {
      await FileSystem.deleteAsync(item.posterPath, { idempotent: true }).catch(() => undefined);
    }
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

// Matches the app-private download shape (destPathFor): evey-<mediaId>.<ext>.
// The sweep recognizes OUR files by this exact contract and never touches
// anything else living in the same system-managed directory.
const PRIVATE_DOWNLOAD_RE = /^evey-(.+)\.([a-z0-9]+)$/;

/**
 * Deletes app-private download files that no index entry references anymore.
 * Betas <= 25 deleted downloads with a bare path the legacy FS API silently
 * rejected: the index row vanished, the gigabytes stayed. Those files are now
 * unreachable by deleteDownload (no index entry to start from), so a startup
 * sweep is the only way back to a truthful disk. Runs AFTER
 * rehydrateBackgroundDownloads so an in-flight task's partial file - present
 * on disk, absent from the index by design - is recognized and spared.
 * The public Download/Evey files are deliberately NOT swept: they are
 * user-visible documents the user may keep on purpose, and their deletes have
 * worked correctly since the file:// fix.
 * Returns the number of files removed.
 */
export async function purgeOrphanedDownloads(): Promise<number> {
  let names: string[];
  try {
    names = await FileSystem.readDirectoryAsync(toFileUri(directories.documents));
  } catch {
    return 0; // directory unreadable/absent: nothing to sweep
  }
  const index = await readIndex();
  const referenced = new Set(index.map((i) => i.mediaItemId));
  let purged = 0;
  for (const name of names) {
    const m = PRIVATE_DOWNLOAD_RE.exec(name);
    if (!m) continue;
    const mediaItemId = m[1];
    if (referenced.has(mediaItemId) || activeDownloads.has(mediaItemId) || inFlight.has(mediaItemId)) {
      continue;
    }
    try {
      await FileSystem.deleteAsync(toFileUri(`${directories.documents}/${name}`), { idempotent: true });
      purged += 1;
    } catch (err) {
      console.warn("caught", err);
    }
  }
  if (purged > 0) console.log(`evey-dl purge removed=${purged}`);
  return purged;
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
