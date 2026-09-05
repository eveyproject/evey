import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Two mocks because the module under test now imports from two entry points,
// and that split is the whole point: in expo-file-system 55 the file helpers
// only work when imported from "/legacy" (the root re-exports throwing stubs),
// while Paths only exists at the root. Mocking a single merged module is what
// hid the real failure - the suite was green against a module shape that does
// not exist at runtime.
// Free-space reading consulted by the pre-download guard. Behind a getter on a
// stable Paths object, because downloads.ts imports the binding by name
// (`import { Paths }`) rather than reading it off a namespace: reassigning the
// module's export from a test would not reach that binding, mutating this
// holder does. Generous by default so existing tests are unaffected.
const { diskSpace } = vi.hoisted(() => ({
  diskSpace: { available: 500 * 1024 * 1024 * 1024 },
}));

vi.mock("expo-file-system", () => ({
  Paths: {
    get availableDiskSpace() {
      return diskSpace.available;
    },
  },
}));

vi.mock("expo-file-system/legacy", () => ({
  getInfoAsync: vi.fn(),
  makeDirectoryAsync: vi.fn(),
  readAsStringAsync: vi.fn(),
  writeAsStringAsync: vi.fn(),
  deleteAsync: vi.fn(),
  readDirectoryAsync: vi.fn(),
  // moveAsync: the index is written to a .tmp then moved into place, so a
  // reader never parses a half file. downloadAsync: cacheDownloadPoster calls
  // it for every non-audio download and swallows its errors - leaving it out
  // of this mock meant every download test silently ran the failure branch.
  moveAsync: vi.fn(),
  downloadAsync: vi.fn(),
  documentDirectory: "/data/user/0/app/files/",
}));

// Mock the native background downloader. createDownloadTask returns a task
// with chainable begin/progress/done/error setters; the tests trigger them
// synchronously via the captured callbacks to assert the lifecycle.
type TaskHandlers = {
  begin?: (data: { expectedBytes: number; headers: Record<string, string> }) => void;
  progress?: (data: { bytesDownloaded: number; bytesTotal: number }) => void;
  done?: (data: { id: string }) => void;
  error?: (data: { id: string; error: string }) => void;
};
const taskInstances = new Map<string, TaskHandlers>();
const existingTasks: { id: string; metadata: Record<string, unknown> }[] = [];

const taskStops: string[] = [];

// Public Downloads collection root, null by default so the historical
// app-private path keeps covering the existing tests; individual tests flip it
// to assert the VLC-readable destination.
const publicDirMock = vi.fn(async (): Promise<string | null> => null);
// False by default: the native File helper "fails", so every existing test
// keeps exercising the expo deleteAsync path it was written against; the
// public-path tests flip it to true to assert the fallback order.
const deleteMediaFileMock = vi.fn(async (_path: string): Promise<boolean> => false);
vi.mock("@evey/media-caps", () => ({
  getPublicDownloadsDir: (...a: unknown[]) => publicDirMock(...(a as [])),
  deleteMediaFile: (...a: unknown[]) => deleteMediaFileMock(...(a as [string])),
}));

vi.mock("@kesha-antonov/react-native-background-downloader", () => ({
  setConfig: vi.fn(),
  createDownloadTask: vi.fn(({ id }: { id: string }) => {
    const handlers: TaskHandlers = {};
    taskInstances.set(id, handlers);
    const task = {
      begin: vi.fn((cb: TaskHandlers["begin"]) => { handlers.begin = cb; return task; }),
      progress: vi.fn((cb: TaskHandlers["progress"]) => { handlers.progress = cb; return task; }),
      done: vi.fn((cb: TaskHandlers["done"]) => { handlers.done = cb; return task; }),
      error: vi.fn((cb: TaskHandlers["error"]) => { handlers.error = cb; return task; }),
      start: vi.fn(),
      // Recorded by id so a test can assert the OS download was actually
      // stopped, not merely that the promise rejected.
      stop: vi.fn(() => { taskStops.push(id); }),
    };
    return task;
  }),
  getExistingDownloadTasks: vi.fn(async () => existingTasks),
  completeHandler: vi.fn(),
  directories: { documents: "/data/user/0/app/files/Documents" },
}));

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn().mockResolvedValue({ token: "stream-tok-123", mediaId: "m3" }),
  // Reused (not a new endpoint) by getDownloadPath's online revalidation - it
  // already 404s/403s server-side when library access or the parental gate
  // no longer authorizes this profile for the item.
  libraryServiceGetMediaItem: vi.fn().mockResolvedValue({ id: "m1" }),
}));

vi.mock("./server-config", () => ({
  getServerUrl: vi.fn().mockResolvedValue("https://evey.test"),
}));

// Device audio codecs consulted by the pre-download compat-pass decision.
// Mocked because the real module reaches for react-native's NativeModules;
// a mutable holder so tests can shape the device (no ac3 by default - the
// common Android reality the 2x guard exists for).
const { deviceCodecs } = vi.hoisted(() => ({
  deviceCodecs: { value: ["aac", "mp3", "opus", "flac"] },
}));
vi.mock("./deviceCodecs", () => ({
  getDeviceAudioCodecs: () => deviceCodecs.value,
}));

const ensureNotificationPermissionMock = vi.hoisted(() => vi.fn(async () => {}));
vi.mock("./notificationPermission", () => ({
  ensureNotificationPermission: ensureNotificationPermissionMock,
}));

import * as FileSystem from "expo-file-system/legacy";
const fsMock = FileSystem as unknown as Record<string, ReturnType<typeof vi.fn>>;

import { eveyFetch, libraryServiceGetMediaItem } from "@evey/api";
const eveyFetchMock = eveyFetch as ReturnType<typeof vi.fn>;
const getMediaItemMock = libraryServiceGetMediaItem as ReturnType<typeof vi.fn>;

import {
  createDownloadTask,
  getExistingDownloadTasks,
} from "@kesha-antonov/react-native-background-downloader";
const createDownloadTaskMock = createDownloadTask as unknown as ReturnType<typeof vi.fn>;
const getExistingDownloadTasksMock = getExistingDownloadTasks as unknown as ReturnType<typeof vi.fn>;

import {
  getDownloads,
  startDownload,
  deleteDownload,
  updateDownloadedAudio,
  cancelDownload,
  isDownloadCancelable,
  getDownloadPath,
  getTotalDownloadSize,
  rehydrateBackgroundDownloads,
  purgeOrphanedDownloads,
  getInFlightDownloads,
  getInFlightBytes,
  isPreparingCompat,
  getDownloadedMedia,
  hasRenderableAudio,
  needsAudioCompatPass,
  requiredDownloadBytes,
  downloadErrorMessageKey,
  ERR_NOT_ENOUGH_SPACE,
  ERR_NOT_ENOUGH_SPACE_AUDIO_PASS,
} from "./downloads";
import { buildDownloadAuthContext } from "./downloadContext";
import { canPlayOffline } from "./offlineAccess";

const sampleIndex = [
  { mediaItemId: "m1", title: "Movie 1", filePath: "/data/user/0/app/files/downloads/m1.mp4", fileSize: 1000, downloadedAt: "2025-01-01" },
  { mediaItemId: "m2", title: "Movie 2", filePath: "/data/user/0/app/files/downloads/m2.mp4", fileSize: 2000, downloadedAt: "2025-01-02" },
];

// The module persists TWO files now (index.json + the pending.json crash
// records), so "the last write" is ambiguous - assertions about persisted
// entries must filter on the index path instead of call order.
// The index lands via a .tmp file that is then moved into place (see
// writeIndex), so the CONTENT assertions below match either name.
function lastIndexWritten(): Array<Record<string, unknown>> {
  const calls = fsMock.writeAsStringAsync.mock.calls.filter((c) =>
    /index\.json(\.tmp)?$/.test(String(c[0])),
  );
  expect(calls.length).toBeGreaterThan(0);
  return JSON.parse(calls.at(-1)![1]);
}

function lastPendingWritten(): Array<Record<string, unknown>> {
  const calls = fsMock.writeAsStringAsync.mock.calls.filter((c) => String(c[0]).endsWith("pending.json"));
  expect(calls.length).toBeGreaterThan(0);
  return JSON.parse(calls.at(-1)![1]);
}

beforeEach(() => {
  vi.clearAllMocks();
  taskInstances.clear();
  existingTasks.length = 0;
  fsMock.getInfoAsync.mockResolvedValue({ exists: true, size: 0 });
  fsMock.readAsStringAsync.mockResolvedValue(JSON.stringify(sampleIndex));
  fsMock.writeAsStringAsync.mockResolvedValue(undefined);
  fsMock.makeDirectoryAsync.mockResolvedValue(undefined);
  fsMock.deleteAsync.mockResolvedValue(undefined);
  fsMock.readDirectoryAsync.mockResolvedValue([]);
  fsMock.moveAsync.mockResolvedValue(undefined);
  fsMock.downloadAsync.mockResolvedValue({ status: 200 });
});

describe("getDownloads", () => {
  it("returns parsed index", async () => {
    const result = await getDownloads();
    expect(result).toEqual(sampleIndex);
  });

  it("returns empty array when index unreadable", async () => {
    fsMock.readAsStringAsync.mockRejectedValue(new Error("not found"));
    const result = await getDownloads();
    expect(result).toEqual([]);
  });
});

// Wait until the native createDownloadTask has been called with the given id.
// Avoids brittle "await Promise.resolve() N times" by polling the spy.
async function waitForTask(id: string, timeoutMs = 500): Promise<TaskHandlers> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const h = taskInstances.get(id);
    if (h) return h;
    await new Promise((r) => setTimeout(r, 5));
  }
  throw new Error(`timed out waiting for download task ${id}`);
}

describe("startDownload", () => {
  // The downloader's progress rides a foreground-service notification, which
  // Android 13+ suppresses until POST_NOTIFICATIONS is granted - and only the
  // cast flow ever asked for it, so a user who never cast downloaded blind.
  it("asks for the notification permission when a download starts", async () => {
    const pending = startDownload("m-notif", "Some Movie");
    const handlers = await waitForTask("m-notif");
    expect(ensureNotificationPermissionMock).toHaveBeenCalled();
    handlers.done?.({ id: "m-notif" });
    await pending;
  });

  it("creates a background download task with stream token + bearer URL", async () => {
    const pending = startDownload("m3", "Movie 3");
    const handlers = await waitForTask("m3");

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m3/stream-token",
      method: "POST",
    });
    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      id: "m3",
      url: "https://evey.test/api/v1/media/m3/download?token=stream-tok-123",
      destination: expect.stringContaining("evey-m3.mp4"),
      metadata: expect.objectContaining({ title: "Movie 3" }),
    }));
    // task.start() must be invoked after handlers are chained - without it
    // createDownloadTask only registers the task, it doesn't kick the OS
    // download (see DownloadTask.ts `start()` in the lib).
    const taskMock = createDownloadTaskMock.mock.results.at(-1)?.value;
    expect(taskMock?.start).toHaveBeenCalled();

    handlers.begin?.({ expectedBytes: 5000, headers: {} });
    handlers.done?.({ id: "m3" });

    const path = await pending;
    expect(path).toContain("evey-m3.mp4");
    const written = lastIndexWritten();
    expect(written).toHaveLength(3);
    expect(written[2].mediaItemId).toBe("m3");
    expect(written[2].fileSize).toBe(5000);
  });

  it("calls onProgress callback with fraction and byte counts", async () => {
    const onProgress = vi.fn();
    const pending = startDownload("m4", "Movie 4", onProgress);
    const handlers = await waitForTask("m4");
    handlers.progress?.({ bytesDownloaded: 50, bytesTotal: 100 });
    handlers.done?.({ id: "m4" });
    await pending;
    // Byte counts ride along with the fraction: an integer percent of a
    // multi-GB file sits at "0%" for minutes, so the UI needs the raw bytes
    // to show that the transfer is actually moving.
    expect(onProgress).toHaveBeenCalledWith(0.5, { downloaded: 50, total: 100 });
  });

  it("rejects when the native task errors", async () => {
    const pending = startDownload("m5", "Movie 5");
    const handlers = await waitForTask("m5");
    handlers.error?.({ id: "m5", error: "network broke" });
    await expect(pending).rejects.toThrow("network broke");
  });

  it("deletes the partial file on the disk when the native task errors", async () => {
    const pending = startDownload("m6", "Movie 6");
    const handlers = await waitForTask("m6");
    handlers.error?.({ id: "m6", error: "network broke" });
    await expect(pending).rejects.toThrow("network broke");
    expect(fsMock.deleteAsync).toHaveBeenCalledWith(
      expect.stringContaining("evey-m6.mp4"),
      { idempotent: true },
    );
  });
});

describe("cancelDownload", () => {
  it("reports a running download as cancelable, and not once settled", async () => {
    const pending = startDownload("m7", "Movie 7");
    const handlers = await waitForTask("m7");
    expect(isDownloadCancelable("m7")).toBe(true);

    handlers.done?.({ id: "m7" });
    await pending;
    expect(isDownloadCancelable("m7")).toBe(false);
  });

  it("stops the native task and cleans up in-flight state + the partial file", async () => {
    const pending = startDownload("m8", "Movie 8");
    await waitForTask("m8");
    expect(getInFlightDownloads().some((d) => d.mediaItemId === "m8")).toBe(true);

    await cancelDownload("m8");

    expect(taskStops).toContain("m8");
    expect(isDownloadCancelable("m8")).toBe(false);
    expect(getInFlightDownloads().some((d) => d.mediaItemId === "m8")).toBe(false);
    expect(fsMock.deleteAsync).toHaveBeenCalledWith(
      expect.stringContaining("evey-m8.mp4"),
      { idempotent: true },
    );

    // The now-orphaned promise from startDownload never settles once stopped
    // (stopTask fires neither done() nor error() on the native side) - this
    // is a fire-and-forget cancel, not something callers await.
    void pending;
  });

  it("is a no-op for a mediaId with nothing in flight", async () => {
    await expect(cancelDownload("unknown")).resolves.toBeUndefined();
    expect(taskStops).not.toContain("unknown");
  });
});

describe("rehydrateBackgroundDownloads", () => {
  it("re-attaches done/error callbacks to in-flight downloads", async () => {
    existingTasks.push({ id: "m9", metadata: { title: "Movie 9" } });
    const taskHandlers: TaskHandlers = {};
    const fakeTask = {
      id: "m9",
      metadata: { title: "Movie 9" },
      progress: vi.fn((cb: TaskHandlers["progress"]) => { taskHandlers.progress = cb; return fakeTask; }),
      done: vi.fn((cb: TaskHandlers["done"]) => { taskHandlers.done = cb; return fakeTask; }),
      error: vi.fn((cb: TaskHandlers["error"]) => { taskHandlers.error = cb; return fakeTask; }),
    };
    getExistingDownloadTasksMock.mockResolvedValueOnce([fakeTask]);

    await rehydrateBackgroundDownloads();

    expect(fakeTask.progress).toHaveBeenCalled();
    expect(fakeTask.done).toHaveBeenCalled();
    expect(fakeTask.error).toHaveBeenCalled();

    // Drive the rehydrated task to completion and assert the index updates.
    taskHandlers.progress?.({ bytesDownloaded: 100, bytesTotal: 1000 });
    taskHandlers.done?.({ id: "m9" });
    // Wait a tick so the async persist runs.
    await new Promise((resolve) => setTimeout(resolve, 0));
    const idx = lastIndexWritten();
    expect(idx.some((i: { mediaItemId: string }) => i.mediaItemId === "m9")).toBe(true);
  });

  it("returns quietly when the native lookup fails", async () => {
    getExistingDownloadTasksMock.mockRejectedValueOnce(new Error("not ready"));
    await expect(rehydrateBackgroundDownloads()).resolves.toBeUndefined();
  });

  it("deletes the partial file on disk when a rehydrated task errors", async () => {
    existingTasks.push({ id: "m10", metadata: { title: "Movie 10" } });
    const taskHandlers: TaskHandlers = {};
    const fakeTask = {
      id: "m10",
      metadata: { title: "Movie 10" },
      progress: vi.fn((cb: TaskHandlers["progress"]) => { taskHandlers.progress = cb; return fakeTask; }),
      done: vi.fn((cb: TaskHandlers["done"]) => { taskHandlers.done = cb; return fakeTask; }),
      error: vi.fn((cb: TaskHandlers["error"]) => { taskHandlers.error = cb; return fakeTask; }),
    };
    getExistingDownloadTasksMock.mockResolvedValueOnce([fakeTask]);

    await rehydrateBackgroundDownloads();
    taskHandlers.error?.({ id: "m10", error: "network broke" });
    await new Promise((resolve) => setTimeout(resolve, 0));

    expect(fsMock.deleteAsync).toHaveBeenCalledWith(
      expect.stringContaining("evey-m10.mp4"),
      { idempotent: true },
    );
  });

  it("reads the real file size from disk when done fires with no prior progress", async () => {
    existingTasks.push({ id: "m11", metadata: { title: "Movie 11" } });
    const taskHandlers: TaskHandlers = {};
    const fakeTask = {
      id: "m11",
      metadata: { title: "Movie 11" },
      progress: vi.fn((cb: TaskHandlers["progress"]) => { taskHandlers.progress = cb; return fakeTask; }),
      done: vi.fn((cb: TaskHandlers["done"]) => { taskHandlers.done = cb; return fakeTask; }),
      error: vi.fn((cb: TaskHandlers["error"]) => { taskHandlers.error = cb; return fakeTask; }),
    };
    getExistingDownloadTasksMock.mockResolvedValueOnce([fakeTask]);
    fsMock.getInfoAsync.mockResolvedValue({ exists: true, size: 42424 });

    await rehydrateBackgroundDownloads();
    // done fires with no progress() call at all - totalBytes was never set.
    taskHandlers.done?.({ id: "m11" });
    await new Promise((resolve) => setTimeout(resolve, 0));

    const idx = lastIndexWritten();
    const entry = idx.find((i: { mediaItemId: string }) => i.mediaItemId === "m11");
    expect(entry!.fileSize).toBe(42424);
  });
});

describe("deleteDownload", () => {
  it("removes file and updates index", async () => {
    await deleteDownload("m1");

    // MUST be a file:// URI: expo-file-system/legacy hands a bare absolute
    // path to Android with a null scheme, which probes it as an app RESOURCE -
    // deleteAsync then fails (silently, it's wrapped in try/catch) and the
    // gigabytes stay orphaned on the device forever.
    expect(fsMock.deleteAsync).toHaveBeenCalledWith(
      "file:///data/user/0/app/files/downloads/m1.mp4",
      { idempotent: true },
    );
    const written = lastIndexWritten();
    expect(written).toHaveLength(1);
    expect(written[0].mediaItemId).toBe("m2");
  });

  it("updates index even if item not found", async () => {
    await deleteDownload("nonexistent");

    expect(fsMock.deleteAsync).not.toHaveBeenCalled();
    const written = lastIndexWritten();
    expect(written).toHaveLength(2);
  });
});

// Every index write is a read-modify-write on one JSON file, and the callers
// genuinely overlap: rehydrateBackgroundDownloads re-attaches every background
// download that finished while the app was dead and their completion callbacks
// fire together, while the audio compat pass writes on its own schedule. Two
// interleaved cycles start from the same snapshot and the second write drops
// the first one's change - the bytes stay on disk with nothing referencing
// them, and the orphan sweep deletes them at the next boot.
describe("concurrent index writes", () => {
  // A real (if tiny) filesystem: the static mock in beforeEach always answers
  // the same snapshot, which is precisely the interleaving that must NOT be
  // silently tolerated here.
  function useEchoingIndex(initial: Array<Record<string, unknown>>) {
    const files = new Map<string, string>([
      ["/data/user/0/app/files/downloads/index.json", JSON.stringify(initial)],
    ]);
    fsMock.readAsStringAsync.mockImplementation(async (path: string) => {
      const v = files.get(String(path));
      if (v === undefined) throw new Error("not found");
      // Yield, so a competing cycle gets a chance to interleave here - exactly
      // where the lost update used to happen.
      await new Promise((r) => setTimeout(r, 0));
      return v;
    });
    fsMock.writeAsStringAsync.mockImplementation(async (path: string, data: string) => {
      files.set(String(path), data);
    });
    fsMock.moveAsync.mockImplementation(async ({ from, to }: { from: string; to: string }) => {
      const v = files.get(String(from));
      if (v !== undefined) {
        files.set(String(to), v);
        files.delete(String(from));
      }
    });
    return files;
  }

  it("keeps both changes when two writers run at once", async () => {
    const files = useEchoingIndex([
      { mediaItemId: "m1", title: "A", filePath: "/d/m1.mp4", fileSize: 1, downloadedAt: "2026-01-01T00:00:00Z" },
      { mediaItemId: "m2", title: "B", filePath: "/d/m2.mp4", fileSize: 2, downloadedAt: "2026-01-01T00:00:00Z" },
    ]);

    await Promise.all([
      updateDownloadedAudio("m1", { audioCodecs: ["aac"] }),
      updateDownloadedAudio("m2", { audioCodecs: ["opus"] }),
    ]);

    const final = JSON.parse(files.get("/data/user/0/app/files/downloads/index.json")!);
    expect(final.find((i: { mediaItemId: string }) => i.mediaItemId === "m1").audioCodecs).toEqual(["aac"]);
    expect(final.find((i: { mediaItemId: string }) => i.mediaItemId === "m2").audioCodecs).toEqual(["opus"]);
  });

  it("does not lose an entry when a delete and an update overlap", async () => {
    const files = useEchoingIndex([
      { mediaItemId: "m1", title: "A", filePath: "/d/m1.mp4", fileSize: 1, downloadedAt: "2026-01-01T00:00:00Z" },
      { mediaItemId: "m2", title: "B", filePath: "/d/m2.mp4", fileSize: 2, downloadedAt: "2026-01-01T00:00:00Z" },
    ]);

    await Promise.all([
      deleteDownload("m1"),
      updateDownloadedAudio("m2", { audioCodecs: ["aac"] }),
    ]);

    const final = JSON.parse(files.get("/data/user/0/app/files/downloads/index.json")!);
    expect(final).toHaveLength(1);
    expect(final[0].mediaItemId).toBe("m2");
    // The update must survive the delete's own read-modify-write.
    expect(final[0].audioCodecs).toEqual(["aac"]);
  });

  // readIndex answers [] on a parse failure, and purgeOrphanedDownloads
  // DELETES every private file the index does not reference. A reader landing
  // mid-write must therefore never see a truncated file.
  it("never leaves the index file half written", async () => {
    useEchoingIndex([]);
    await updateDownloadedAudio("m1", { audioCodecs: ["aac"] });

    const writes = fsMock.writeAsStringAsync.mock.calls.filter((c) => String(c[0]).includes("index.json"));
    for (const [path] of writes) {
      expect(String(path)).toMatch(/index\.json\.tmp$/);
    }
  });
});

describe("getDownloadPath", () => {
  it("returns path if file exists", async () => {
    fsMock.getInfoAsync.mockResolvedValue({ exists: true });
    const path = await getDownloadPath("m1");
    expect(path).toBe("/data/user/0/app/files/downloads/m1.mp4");
  });

  it("probes existence with a file:// URI, never a bare path", async () => {
    // Same null-scheme trap as deleteAsync above, with a worse symptom: a bare
    // path makes getInfoAsync answer exists:false for a file that IS on disk,
    // so offline playback reports the download as missing (the airplane-mode
    // "stream failed" bug) and online playback silently re-streams it.
    fsMock.getInfoAsync.mockResolvedValue({ exists: true });
    await getDownloadPath("m1");
    expect(fsMock.getInfoAsync).toHaveBeenCalledWith("file:///data/user/0/app/files/downloads/m1.mp4");
  });

  it("returns null if file missing on disk", async () => {
    fsMock.getInfoAsync.mockResolvedValue({ exists: false });
    const path = await getDownloadPath("m1");
    expect(path).toBeNull();
  });

  it("returns null if not in index", async () => {
    const path = await getDownloadPath("unknown");
    expect(path).toBeNull();
  });

  it("does not gate when no activeProfile is passed (legacy call sites not yet migrated)", async () => {
    fsMock.getInfoAsync.mockResolvedValue({ exists: true });
    const path = await getDownloadPath("m1");
    expect(path).toBe("/data/user/0/app/files/downloads/m1.mp4");
    expect(getMediaItemMock).not.toHaveBeenCalled();
  });
});

describe("getDownloadPath offline-access gate + online revalidation", () => {
  // Relative, not a fixed date: these cases assert the PROFILE decision, and a
  // hard-coded downloadedAt silently turns them into TTL tests once
  // OFFLINE_DOWNLOAD_TTL_MS (30 days) elapses. The literal "2026-07-01" that
  // used to be here started failing on 2026-07-31 - the suite went red on a
  // calendar boundary, with nothing in the code having changed.
  const recentlyDownloadedAt = new Date(Date.now() - 24 * 60 * 60 * 1000).toISOString();
  const restrictedIndex = [
    {
      mediaItemId: "m1",
      title: "Movie 1",
      filePath: "/data/user/0/app/files/downloads/m1.mp4",
      fileSize: 1000,
      downloadedAt: recentlyDownloadedAt,
      downloadedByProfileId: "adult-1",
      downloadedByIsKids: false,
    },
  ];

  beforeEach(() => {
    fsMock.readAsStringAsync.mockResolvedValue(JSON.stringify(restrictedIndex));
    fsMock.getInfoAsync.mockResolvedValue({ exists: true });
  });

  it("throws OfflineAccessError when the active profile is more restricted than the downloader", async () => {
    await expect(
      getDownloadPath("m1", { activeProfile: { id: "kid-1", isKids: true, maxAge: 10 } }),
    ).rejects.toMatchObject({ name: "OfflineAccessError", reason: "more-restricted-profile" });
  });

  it("throws OfflineAccessError when the entry is expired", async () => {
    // Anchored on the fixture, not on a literal date: this is the one case
    // that IS about the TTL, so "40 days after the download" has to follow the
    // entry wherever it sits in time.
    const farFuture = Date.parse(recentlyDownloadedAt) + 40 * 24 * 60 * 60 * 1000;
    await expect(
      getDownloadPath("m1", { activeProfile: { id: "adult-1" }, now: farFuture }),
    ).rejects.toMatchObject({ name: "OfflineAccessError", reason: "expired" });
  });

  it("returns the path when the active profile is authorized and the server confirms access", async () => {
    getMediaItemMock.mockResolvedValueOnce({ id: "m1" });
    const path = await getDownloadPath("m1", { activeProfile: { id: "adult-1" } });
    expect(path).toBe("/data/user/0/app/files/downloads/m1.mp4");
    expect(getMediaItemMock).toHaveBeenCalledWith("m1");
  });

  it("throws OfflineAccessError('revoked') when the server explicitly refuses access (403/404)", async () => {
    getMediaItemMock.mockRejectedValueOnce(Object.assign(new Error("Forbidden"), { httpStatus: 403 }));
    await expect(
      getDownloadPath("m1", { activeProfile: { id: "adult-1" } }),
    ).rejects.toMatchObject({ name: "OfflineAccessError", reason: "revoked" });
  });

  it("falls back to the local decision when revalidation merely fails to reach the server (offline)", async () => {
    getMediaItemMock.mockRejectedValueOnce(new Error("network request failed"));
    const path = await getDownloadPath("m1", { activeProfile: { id: "adult-1" } });
    expect(path).toBe("/data/user/0/app/files/downloads/m1.mp4");
  });
});

describe("getTotalDownloadSize", () => {
  it("sums sizes of existing files", async () => {
    fsMock.getInfoAsync
      .mockResolvedValueOnce({ exists: true, size: 1500 })
      .mockResolvedValueOnce({ exists: true, size: 2500 });

    const total = await getTotalDownloadSize();
    expect(total).toBe(4000);
  });

  it("skips non-existing files", async () => {
    fsMock.getInfoAsync
      .mockResolvedValueOnce({ exists: false })
      .mockResolvedValueOnce({ exists: true, size: 3000 });

    const total = await getTotalDownloadSize();
    expect(total).toBe(3000);
  });
});

describe("startDownload media-id validation", () => {
  it("rejects a media id that could escape the sandbox path or redirect the API request", async () => {
    for (const bad of ["mi-1/../../etc", "mi-1/../admin", "../secret", "mi-1?x=y", "mi 1", ""]) {
      await expect(startDownload(bad, "x")).rejects.toThrow(/invalid media id/);
    }
  });

  it("accepts a legitimate media id (Evey ids are opaque UUID/slug strings, not a fixed grammar)", async () => {
    const pending = startDownload("550e8400-e29b-41d4-a716-446655440000", "Legit");
    const handlers = await waitForTask("550e8400-e29b-41d4-a716-446655440000");
    handlers.begin?.({ expectedBytes: 100, headers: {} });
    handlers.done?.({ id: "550e8400-e29b-41d4-a716-446655440000" });
    await expect(pending).resolves.toContain("550e8400-e29b-41d4-a716-446655440000");
  });
});

// Downloading a 40GB film onto a phone with 2GB free used to start happily and
// die deep into the transfer, having filled the disk on the way. The size is
// only knowable in the begin handler, so that is the last moment to refuse.
describe("startDownload disk-space guard", () => {
  it("refuses a download that would not fit, and leaves nothing behind", async () => {
    diskSpace.available = 2 * 1024 * 1024 * 1024;

    const pending = startDownload("big", "Huge Film");
    const handlers = await waitForTask("big");

    // 40GB against 2GB free.
    handlers.begin?.({ expectedBytes: 40 * 1024 * 1024 * 1024, headers: {} });

    await expect(pending).rejects.toThrow("NOT_ENOUGH_SPACE");
    expect(taskStops).toContain("big");
    expect(vi.mocked(FileSystem.deleteAsync)).toHaveBeenCalledWith(
      expect.stringContaining("evey-big.mp4"),
      { idempotent: true },
    );
  });

  it("allows a download that fits", async () => {
    diskSpace.available = 50 * 1024 * 1024 * 1024;

    const pending = startDownload("small", "Small Film");
    const handlers = await waitForTask("small");

    handlers.begin?.({ expectedBytes: 1024 * 1024 * 1024, headers: {} });
    handlers.done?.({ id: "small" });

    await expect(pending).resolves.toContain("evey-small.mp4");
  });
});

// A movie stored as MKV or a music track must not be forced onto a hardcoded
// .mp4 name - MediaScanner/system players rely on the real extension.
describe("startDownload extension resolution", () => {
  it("defaults to .mp4 for a video download with no format hint", async () => {
    const pending = startDownload("v1", "Movie", undefined, { kind: "video" });
    const handlers = await waitForTask("v1");
    handlers.done?.({ id: "v1" });
    await expect(pending).resolves.toContain("evey-v1.mp4");
  });

  it("uses the item's container format for a video download", async () => {
    const pending = startDownload("v2", "Movie MKV", undefined, { kind: "video", formatHint: "matroska" });
    const handlers = await waitForTask("v2");
    handlers.done?.({ id: "v2" });
    await expect(pending).resolves.toContain("evey-v2.mkv");
  });

  it("defaults to .mp3 for an audio download with no format hint", async () => {
    const pending = startDownload("a1", "Track", undefined, { kind: "audio" });
    const handlers = await waitForTask("a1");
    handlers.done?.({ id: "a1" });
    await expect(pending).resolves.toContain("evey-a1.mp3");
  });

  it("derives the extension from a mime type hint", async () => {
    const pending = startDownload("a2", "Lossless Track", undefined, { kind: "audio", formatHint: "audio/flac" });
    const handlers = await waitForTask("a2");
    handlers.done?.({ id: "a2" });
    await expect(pending).resolves.toContain("evey-a2.flac");
  });

  it("falls back to the default when the hint isn't a recognized container/mime", async () => {
    const pending = startDownload("a3", "Unknown Track", undefined, { kind: "audio", formatHint: "application/octet-stream" });
    const handlers = await waitForTask("a3");
    handlers.done?.({ id: "a3" });
    await expect(pending).resolves.toContain("evey-a3.mp3");
  });
});

// A music track is not a library media item: it mints its download token
// through the generic /api/v1/download-token/{id} endpoint and downloads from
// /api/v1/music/tracks/{id}/download, not the movie/episode-only
// /api/v1/media/{id}/... routes - a naive reuse of the defaults 404s.
describe("startDownload endpoint overrides (non-media resources, e.g. music tracks)", () => {
  it("mints through tokenUrl and downloads through downloadPath when provided", async () => {
    const pending = startDownload("track-1", "Some Song", undefined, {
      kind: "audio",
      tokenUrl: "/api/v1/download-token/track-1",
      downloadPath: "/api/v1/music/tracks/track-1/download",
    });
    const handlers = await waitForTask("track-1");

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/download-token/track-1",
      method: "POST",
    });
    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      url: "https://evey.test/api/v1/music/tracks/track-1/download?token=stream-tok-123",
      destination: expect.stringContaining("evey-track-1.mp3"),
    }));

    handlers.done?.({ id: "track-1" });
    await pending;
  });

  it("defaults to the media stream-token/download routes when no override is given", async () => {
    const pending = startDownload("m20", "Movie 20");
    const handlers = await waitForTask("m20");

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m20/stream-token",
      method: "POST",
    });
    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      url: "https://evey.test/api/v1/media/m20/download?token=stream-tok-123",
    }));

    handlers.done?.({ id: "m20" });
    await pending;
  });
});

describe("getInFlightDownloads", () => {
  it("lists downloads that have not reached the persisted index yet", async () => {
    const pending = startDownload("if1", "In Flight Movie");
    await waitForTask("if1");

    const inFlightList = getInFlightDownloads();
    expect(inFlightList).toEqual(expect.arrayContaining([
      { mediaItemId: "if1", title: "In Flight Movie" },
    ]));

    const handlers = await waitForTask("if1");
    handlers.done?.({ id: "if1" });
    await pending;

    expect(getInFlightDownloads().some((d) => d.mediaItemId === "if1")).toBe(false);
  });
});

describe("offline audio renderability", () => {
  it("persists the source audio codecs captured at download time", async () => {
    const pending = startDownload("ac1", "AC3 Movie", undefined, { audioCodecs: ["ac3", "aac"] });
    const handlers = await waitForTask("ac1");
    handlers.done?.({ id: "ac1" });
    await pending;
    const written = lastIndexWritten();
    const entry = written.find((i) => i.mediaItemId === "ac1");
    expect(entry!.audioCodecs).toEqual(["ac3", "aac"]);
  });

  it("getDownloadedMedia returns the stored index entry", async () => {
    const entry = await getDownloadedMedia("m1");
    expect(entry?.filePath).toBe("/data/user/0/app/files/downloads/m1.mp4");
    expect(await getDownloadedMedia("nope")).toBeNull();
  });

  it("hasRenderableAudio: legacy entries without codecs are assumed playable", () => {
    expect(hasRenderableAudio({}, ["aac", "mp3"])).toBe(true);
    expect(hasRenderableAudio({ audioCodecs: [] }, ["aac"])).toBe(true);
  });

  it("hasRenderableAudio: false when NO recorded track is device-decodable", () => {
    expect(hasRenderableAudio({ audioCodecs: ["ac3"] }, ["aac", "mp3", "opus"])).toBe(false);
    expect(hasRenderableAudio({ audioCodecs: ["eac3", "dts"] }, ["aac", "mp3"])).toBe(false);
  });

  it("hasRenderableAudio: true when at least one track is decodable (player can pick it)", () => {
    expect(hasRenderableAudio({ audioCodecs: ["ac3", "aac"] }, ["aac", "mp3"])).toBe(true);
    expect(hasRenderableAudio({ audioCodecs: ["AC3"] }, ["ac3"])).toBe(true);
  });
});

describe("public VLC-readable destination", () => {
  it("writes new downloads under <publicDownloads>/Evey with a human-readable name", async () => {
    publicDirMock.mockResolvedValueOnce("/storage/emulated/0/Download");
    const pending = startDownload("0123456789abcdef", "Mon Film: L'Été", undefined, { formatHint: "mkv" });
    const handlers = await waitForTask("0123456789abcdef");
    handlers.done?.({ id: "0123456789abcdef" });
    await pending;
    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      // ":" is filesystem-hostile and must be cleaned; the short id suffix
      // keeps two same-titled items apart; the extension stays real for VLC.
      destination: "/storage/emulated/0/Download/Evey/Mon Film L'Été-01234567.mkv",
    }));
  });

  it("falls back to the app-private directory when no public dir exists", async () => {
    publicDirMock.mockResolvedValueOnce(null);
    const pending = startDownload("pv1", "Private Movie");
    const handlers = await waitForTask("pv1");
    handlers.done?.({ id: "pv1" });
    await pending;
    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      destination: "/data/user/0/app/files/Documents/evey-pv1.mp4",
    }));
  });
});

describe("getInFlightBytes", () => {
  it("exposes the live byte counters of a running download", async () => {
    const pending = startDownload("if2", "In Flight Bytes");
    const handlers = await waitForTask("if2");

    handlers.progress?.({ bytesDownloaded: 250, bytesTotal: 4000 });
    expect(getInFlightBytes("if2")).toEqual({ downloaded: 250, total: 4000 });

    handlers.done?.({ id: "if2" });
    await pending;
    expect(getInFlightBytes("if2")).toBeNull();
  });
});

describe("getDownloadPath extension fallback", () => {
  it("finds the file on disk under a different known extension than the index records", async () => {
    fsMock.readAsStringAsync.mockResolvedValue(JSON.stringify([
      { mediaItemId: "old1", title: "Old", filePath: "/data/user/0/app/files/Documents/evey-old1.mp4", fileSize: 100, downloadedAt: "2025-01-01" },
    ]));
    // The probe arrives as a file:// URI (see the toFileUri contract tests);
    // the resolved path handed back to callers stays in stored (bare) form.
    fsMock.getInfoAsync.mockImplementation(async (path: string) => ({
      exists: path === "file:///data/user/0/app/files/Documents/evey-old1.mkv",
    }));

    const path = await getDownloadPath("old1");
    expect(path).toBe("/data/user/0/app/files/Documents/evey-old1.mkv");
  });
});

// End-to-end proof that the offline-access guard is no longer inert: a
// screen wires buildDownloadAuthContext into startDownload's opts exactly as
// media/[id].tsx and music.tsx now do, and the resulting persisted entry is
// what actually gets a kids profile refused - not just canPlayOffline in
// isolation with a hand-built entry.
describe("startDownload + buildDownloadAuthContext wiring closes the offline-access gap", () => {
  const adultProfile = { id: "adult-1", isKids: false };
  const kidProfile = { id: "kid-1", isKids: true, maxAge: 10 };

  // fsMock.readAsStringAsync is a static mock (see beforeEach) - it doesn't
  // echo back what writeAsStringAsync just persisted, so getDownloads() alone
  // can't be used to read the entry back. Instead, read the exact JSON the
  // write call received, matching the existing "creates a background..." test
  // above, then feed it back to readAsStringAsync so a subsequent
  // getDownloadPath() call reads that same persisted entry.
  function lastWrittenIndex(): Array<Record<string, unknown>> {
    return lastIndexWritten();
  }

  it("persists the downloader's identity on the entry startDownload writes", async () => {
    const pending = startDownload(
      "m30",
      "Adult Movie",
      undefined,
      buildDownloadAuthContext(adultProfile, { officialRating: "R" }),
    );
    const handlers = await waitForTask("m30");
    handlers.begin?.({ expectedBytes: 1000, headers: {} });
    handlers.done?.({ id: "m30" });
    await pending;

    const entry = lastWrittenIndex().find((e) => e.mediaItemId === "m30")!;
    expect(entry.downloadedByProfileId).toBe("adult-1");
    expect(entry.downloadedByIsKids).toBe(false);
    expect(entry.officialRating).toBe("R");
  });

  it("refuses a kids profile replaying a title downloaded by an adult, via the real persisted entry", async () => {
    const pending = startDownload(
      "m31",
      "Another Adult Movie",
      undefined,
      buildDownloadAuthContext(adultProfile, {}),
    );
    const handlers = await waitForTask("m31");
    handlers.begin?.({ expectedBytes: 1000, headers: {} });
    handlers.done?.({ id: "m31" });
    await pending;

    const written = lastWrittenIndex();
    const entry = written.find((e) => e.mediaItemId === "m31") as unknown as import("./downloads").DownloadedMedia;

    const decision = canPlayOffline(entry, kidProfile, Date.now());
    expect(decision).toEqual({ allowed: false, reason: "more-restricted-profile" });

    // getDownloadPath is what the real callers (player, media detail) invoke -
    // proves the whole chain, not just the pure decision function. Feed back
    // the exact JSON just persisted so readIndex() sees this download.
    fsMock.readAsStringAsync.mockResolvedValueOnce(JSON.stringify(written));
    fsMock.getInfoAsync.mockResolvedValue({ exists: true });
    await expect(
      getDownloadPath("m31", { activeProfile: kidProfile }),
    ).rejects.toMatchObject({ name: "OfflineAccessError", reason: "more-restricted-profile" });
  });

  it("still allows the downloading adult profile itself to replay it", async () => {
    const pending = startDownload(
      "m32",
      "Adult Movie Three",
      undefined,
      buildDownloadAuthContext(adultProfile, {}),
    );
    const handlers = await waitForTask("m32");
    handlers.begin?.({ expectedBytes: 1000, headers: {} });
    handlers.done?.({ id: "m32" });
    await pending;

    const written = lastWrittenIndex();
    const entry = written.find((e) => e.mediaItemId === "m32") as unknown as import("./downloads").DownloadedMedia;
    expect(canPlayOffline(entry, adultProfile, Date.now())).toEqual({ allowed: true });
  });
});

// The compat pass rewrites the file next to itself, so a pass-needing download
// transiently costs TWICE its size. That must be refused BEFORE gigabytes
// land: paying 19 GB to discover an unfixable silent film is the exact failure
// the pass exists to remove.
describe("audio compat pass disk guard", () => {
  it("needsAudioCompatPass: only for non-renderable audio the pass can rewrite", () => {
    const noAc3Device = ["aac", "mp3"];
    expect(needsAudioCompatPass({ audioCodecs: ["ac3"] }, noAc3Device)).toBe(true);
    expect(needsAudioCompatPass({ audioCodecs: ["EAC3"] }, noAc3Device)).toBe(true);
    // Renderable somewhere: the local file already plays with sound.
    expect(needsAudioCompatPass({ audioCodecs: ["ac3", "aac"] }, noAc3Device)).toBe(false);
    expect(needsAudioCompatPass({ audioCodecs: ["ac3"] }, ["ac3"])).toBe(false);
    // DTS/TrueHD: silent AND not fixable on device - reserving 2x would hold
    // storage hostage for a pass that never runs.
    expect(needsAudioCompatPass({ audioCodecs: ["dts"] }, noAc3Device)).toBe(false);
    expect(needsAudioCompatPass({ audioCodecs: ["truehd"] }, noAc3Device)).toBe(false);
    // Legacy entry without codec capture: assumed playable, never gated.
    expect(needsAudioCompatPass({ audioCodecs: [] }, noAc3Device)).toBe(false);
    expect(needsAudioCompatPass({}, noAc3Device)).toBe(false);
  });

  it("requiredDownloadBytes doubles the file only when the pass will run", () => {
    const headroom = 200 * 1024 * 1024;
    expect(requiredDownloadBytes(1000, false)).toBe(1000 + headroom);
    expect(requiredDownloadBytes(1000, true)).toBe(2000 + headroom);
  });

  it("refuses an ac3-only film that fits once but not twice, with the dedicated error", async () => {
    // 10 GB file, 15 GB free: the download alone fits, download + rewritten
    // copy does not.
    diskSpace.available = 15 * 1024 * 1024 * 1024;
    const pending = startDownload("ac3film", "Dolby Film", undefined, { audioCodecs: ["ac3"] });
    const handlers = await waitForTask("ac3film");
    handlers.begin?.({ expectedBytes: 10 * 1024 * 1024 * 1024, headers: {} });

    await expect(pending).rejects.toThrow(ERR_NOT_ENOUGH_SPACE_AUDIO_PASS);
    expect(taskStops).toContain("ac3film");
  });

  it("allows the same film when free space covers twice its size", async () => {
    diskSpace.available = 25 * 1024 * 1024 * 1024;
    const pending = startDownload("ac3film2", "Dolby Film 2", undefined, { audioCodecs: ["ac3"] });
    const handlers = await waitForTask("ac3film2");
    handlers.begin?.({ expectedBytes: 10 * 1024 * 1024 * 1024, headers: {} });
    handlers.done?.({ id: "ac3film2" });
    await expect(pending).resolves.toBeTruthy();
  });

  it("keeps the 1x guard for renderable audio (device decodes a track)", async () => {
    deviceCodecs.value = ["aac", "ac3"];
    try {
      diskSpace.available = 15 * 1024 * 1024 * 1024;
      const pending = startDownload("ac3ok", "Dolby On Capable Device", undefined, { audioCodecs: ["ac3"] });
      const handlers = await waitForTask("ac3ok");
      handlers.begin?.({ expectedBytes: 10 * 1024 * 1024 * 1024, headers: {} });
      handlers.done?.({ id: "ac3ok" });
      await expect(pending).resolves.toBeTruthy();
    } finally {
      deviceCodecs.value = ["aac", "mp3", "opus", "flac"];
    }
  });

  it("never doubles a music download (the pass is a video-container remux)", async () => {
    diskSpace.available = 15 * 1024 * 1024 * 1024;
    const pending = startDownload("track1", "Some Track", undefined, {
      kind: "audio",
      audioCodecs: ["ac3"],
    });
    const handlers = await waitForTask("track1");
    handlers.begin?.({ expectedBytes: 10 * 1024 * 1024 * 1024, headers: {} });
    handlers.done?.({ id: "track1" });
    await expect(pending).resolves.toBeTruthy();
  });

  it("downloadErrorMessageKey maps each failure to its one message", () => {
    expect(downloadErrorMessageKey(new Error(ERR_NOT_ENOUGH_SPACE_AUDIO_PASS)))
      .toBe("media.detail.downloadNoSpaceAudioPass");
    expect(downloadErrorMessageKey(new Error(ERR_NOT_ENOUGH_SPACE)))
      .toBe("media.detail.downloadNoSpace");
    expect(downloadErrorMessageKey(new Error("boom"))).toBe("media.detail.downloadFailed");
    expect(downloadErrorMessageKey("not-an-error")).toBe("media.detail.downloadFailed");
  });
});

// Betas <= 25 deleted downloads with a bare path the legacy FS API silently
// rejected: the index row vanished, the file stayed - unreachable forever by
// deleteDownload. The startup sweep is the only way those gigabytes come back.
describe("purgeOrphanedDownloads", () => {
  const docs = "/data/user/0/app/files/Documents";

  it("deletes unreferenced evey files, keeps indexed ones, ignores foreign names", async () => {
    // sampleIndex references m1 and m2.
    fsMock.readDirectoryAsync.mockResolvedValue([
      "evey-m1.mp4",      // indexed - kept
      "evey-ghost.mkv",   // orphan - deleted
      "evey-ghost2.mp4",  // orphan - deleted
      "index.json",       // not ours - ignored
      "somethingelse.mp4", // not ours - ignored
    ]);

    const purged = await purgeOrphanedDownloads();

    expect(purged).toBe(2);
    expect(fsMock.deleteAsync).toHaveBeenCalledWith(`file://${docs}/evey-ghost.mkv`, { idempotent: true });
    expect(fsMock.deleteAsync).toHaveBeenCalledWith(`file://${docs}/evey-ghost2.mp4`, { idempotent: true });
    expect(fsMock.deleteAsync).not.toHaveBeenCalledWith(`file://${docs}/evey-m1.mp4`, { idempotent: true });
    expect(fsMock.deleteAsync).not.toHaveBeenCalledWith(`file://${docs}/index.json`, { idempotent: true });
  });

  it("spares the partial file of an in-flight download (on disk, not yet in the index)", async () => {
    const pending = startDownload("inflight1", "Running");
    const handlers = await waitForTask("inflight1");
    handlers.begin?.({ expectedBytes: 1000, headers: {} });

    fsMock.readDirectoryAsync.mockResolvedValue(["evey-inflight1.mp4"]);
    const purged = await purgeOrphanedDownloads();

    expect(purged).toBe(0);
    expect(fsMock.deleteAsync).not.toHaveBeenCalledWith(
      `file://${docs}/evey-inflight1.mp4`,
      { idempotent: true },
    );

    handlers.done?.({ id: "inflight1" });
    await pending;
  });

  it("returns 0 when the directory cannot be read", async () => {
    fsMock.readDirectoryAsync.mockRejectedValue(new Error("nope"));
    await expect(purgeOrphanedDownloads()).resolves.toBe(0);
  });
});

// The native lib emits its completion event exactly once, then destroys the
// task state. A JS thread frozen at that instant (aggressive OEM background
// policy - seen in the field: a fully-written file, a 99% display, an empty
// index) loses the event with nothing left to rehydrate. These tests pin the
// recovery contract: the pending.json record plus the bytes on disk.
describe("lost-completion reconciliation", () => {
  const DEST = "/storage/emulated/0/Download/Evey/Lost Movie-lost1234.mkv";

  function pendingRecord(overrides: Record<string, unknown> = {}) {
    return {
      mediaItemId: "lost1234",
      title: "Lost Movie",
      destPath: DEST,
      expectedBytes: 5000,
      audioCodecs: ["ac3"],
      startedAt: "2026-08-01T08:27:00.000Z",
      ...overrides,
    };
  }

  function pathAwareRead(index: unknown[], pending: unknown[]) {
    fsMock.readAsStringAsync.mockImplementation(async (p: string) =>
      String(p).endsWith("pending.json") ? JSON.stringify(pending) : JSON.stringify(index),
    );
  }

  it("indexes a finished file whose completion event was lost", async () => {
    pathAwareRead([], [pendingRecord()]);
    fsMock.getInfoAsync.mockResolvedValue({ exists: true, size: 5000 });

    await rehydrateBackgroundDownloads();

    const idx = lastIndexWritten();
    const entry = idx.find((i) => i.mediaItemId === "lost1234");
    expect(entry).toBeTruthy();
    expect(entry!.filePath).toBe(DEST);
    expect(entry!.fileSize).toBe(5000);
    // The codecs ride along so the audio compat pass can still decide.
    expect(entry!.audioCodecs).toEqual(["ac3"]);
    // The record is settled - next boot must not re-index.
    expect(lastPendingWritten()).toEqual([]);
    expect(fsMock.deleteAsync).not.toHaveBeenCalled();
  });

  it("drops a partial whose task vanished (unresumable)", async () => {
    pathAwareRead([], [pendingRecord()]);
    fsMock.getInfoAsync.mockResolvedValue({ exists: true, size: 100 });

    await rehydrateBackgroundDownloads();

    expect(fsMock.deleteAsync).toHaveBeenCalledWith(`file://${DEST}`, { idempotent: true });
    const indexWrites = fsMock.writeAsStringAsync.mock.calls.filter((c) => /index\.json(\.tmp)?$/.test(String(c[0])));
    expect(indexWrites).toHaveLength(0);
    expect(lastPendingWritten()).toEqual([]);
  });

  it("keeps unverifiable bytes when the transfer died before begin()", async () => {
    pathAwareRead([], [pendingRecord({ expectedBytes: undefined })]);
    fsMock.getInfoAsync.mockResolvedValue({ exists: true, size: 900 });

    await rehydrateBackgroundDownloads();

    // Can't prove complete, can't prove truncated: never delete on a guess.
    expect(fsMock.deleteAsync).not.toHaveBeenCalled();
    expect(lastPendingWritten()).toEqual([]);
  });

  it("leaves the record of a still-live task alone", async () => {
    pathAwareRead([], [pendingRecord()]);
    const live = {
      id: "lost1234",
      metadata: { title: "Lost Movie" },
      progress: vi.fn(() => live),
      done: vi.fn(() => live),
      error: vi.fn(() => live),
    };
    getExistingDownloadTasksMock.mockResolvedValueOnce([live]);

    await rehydrateBackgroundDownloads();

    // Its own done/error will settle it; reconciliation must not interfere.
    expect(fsMock.deleteAsync).not.toHaveBeenCalled();
    const pendingWrites = fsMock.writeAsStringAsync.mock.calls.filter((c) => String(c[0]).endsWith("pending.json"));
    expect(pendingWrites).toHaveLength(0);
  });

  it("only clears the record when the entry already made it to the index", async () => {
    pathAwareRead(
      [{ mediaItemId: "lost1234", title: "Lost Movie", filePath: DEST, fileSize: 5000, downloadedAt: "2026-08-01" }],
      [pendingRecord()],
    );

    await rehydrateBackgroundDownloads();

    expect(fsMock.deleteAsync).not.toHaveBeenCalled();
    const indexWrites = fsMock.writeAsStringAsync.mock.calls.filter((c) => /index\.json(\.tmp)?$/.test(String(c[0])));
    expect(indexWrites).toHaveLength(0);
    expect(lastPendingWritten()).toEqual([]);
  });
});

// The server-prepared compat variant: a pass-needing download first asks the
// server for the AAC-audio version (seconds of wait beat minutes of on-device
// re-encode), and EVERY failure shape falls back to today's raw download so
// an old or opted-out server changes nothing.
describe("server compat download", () => {
  function fetchByUrl(routes: Record<string, () => unknown>) {
    eveyFetchMock.mockImplementation(async ({ url }: { url: string }) => {
      // Longest-match on path fragments: the POST now carries a ?renderable=
      // declaration, so "/compat-download/status" must win over its prefix.
      const hit = Object.keys(routes)
        .filter((s) => url.includes(s))
        .sort((a, b) => b.length - a.length)[0];
      if (hit) return routes[hit]();
      return { token: "stream-tok-123" };
    });
  }

  afterEach(() => {
    vi.useRealTimers();
    eveyFetchMock.mockResolvedValue({ token: "stream-tok-123", mediaId: "m3" });
  });

  it("downloads the compat variant when the server has it ready", async () => {
    fetchByUrl({ "/compat-download": () => ({ status: "ready" }) });
    const pending = startDownload("cp1", "AC3 Film", undefined, { audioCodecs: ["ac3"] });
    const handlers = await waitForTask("cp1");

    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      url: "https://evey.test/api/v1/media/cp1/compat-download?token=stream-tok-123",
      destination: expect.stringContaining("evey-cp1.mkv"),
    }));

    handlers.begin?.({ expectedBytes: 5000, headers: {} });
    handlers.done?.({ id: "cp1" });
    await pending;
    // The persisted codecs are the VARIANT's, so the on-device pass never
    // re-runs on a file that is already renderable.
    const entry = lastIndexWritten().find((i) => i.mediaItemId === "cp1");
    expect(entry!.audioCodecs).toEqual(["aac"]);
  });

  it("polls a preparing production until it is ready", async () => {
    let polls = 0;
    fetchByUrl({
      "/compat-download": () => ({ status: "preparing" }),
      "/compat-download/status": () => (++polls < 2 ? { status: "preparing" } : { status: "ready" }),
    });
    vi.useFakeTimers();
    const pending = startDownload("cp2", "AC3 Film", undefined, { audioCodecs: ["ac3"] });
    await vi.advanceTimersByTimeAsync(10);
    expect(isPreparingCompat("cp2")).toBe(true);
    await vi.advanceTimersByTimeAsync(4000);
    await vi.advanceTimersByTimeAsync(4000);
    vi.useRealTimers();

    const handlers = await waitForTask("cp2");
    expect(polls).toBe(2);
    expect(isPreparingCompat("cp2")).toBe(false);
    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      url: expect.stringContaining("/compat-download?token="),
    }));
    handlers.done?.({ id: "cp2" });
    await pending;
  });

  it.each([
    ["endpoint absent (old server)", () => { throw new Error("404"); }],
    ["production failed", () => ({ status: "failed", error: "disk full" })],
  ])("falls back to the raw download on %s", async (_name, compatResponse) => {
    fetchByUrl({ "/compat-download": compatResponse as () => unknown });
    const pending = startDownload("cp3", "AC3 Film", undefined, { audioCodecs: ["ac3"] });
    const handlers = await waitForTask("cp3");
    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      url: "https://evey.test/api/v1/media/cp3/download?token=stream-tok-123",
    }));
    handlers.done?.({ id: "cp3" });
    await pending;
    // Raw file: the SOURCE codecs persist, the local pass stays in play.
    const entry = lastIndexWritten().find((i) => i.mediaItemId === "cp3");
    expect(entry!.audioCodecs).toEqual(["ac3"]);
  });

  it("cancel during preparation rejects without ever starting a task", async () => {
    fetchByUrl({
      "/compat-download": () => ({ status: "preparing" }),
      "/compat-download/status": () => ({ status: "preparing" }),
    });
    vi.useFakeTimers();
    const pending = startDownload("cp4", "AC3 Film", undefined, { audioCodecs: ["ac3"] });
    // Attach the expectation BEFORE the rejection can fire: the abort lands
    // during a timer advance, and an unobserved rejection fails the run.
    const rejected = expect(pending).rejects.toThrow("Download cancelled");
    await vi.advanceTimersByTimeAsync(10);
    expect(isDownloadCancelable("cp4")).toBe(true);
    await cancelDownload("cp4");
    await vi.advanceTimersByTimeAsync(4000);
    vi.useRealTimers();

    await rejected;
    expect(createDownloadTaskMock).not.toHaveBeenCalledWith(expect.objectContaining({ id: "cp4" }));
    expect(getInFlightDownloads().some((d) => d.mediaItemId === "cp4")).toBe(false);
  });

  it("declares the device codecs so the server can refuse a pointless job", async () => {
    fetchByUrl({ "/compat-download": () => ({ status: "ready" }) });
    const pending = startDownload("cp6", "AC3 Film", undefined, { audioCodecs: ["ac3"] });
    const handlers = await waitForTask("cp6");
    const postUrl = eveyFetchMock.mock.calls.map((c) => (c[0] as { url: string }).url).find((u) => u.includes("compat-download"));
    expect(postUrl).toContain("renderable=aac%2Cmp3%2Copus%2Cflac");
    handlers.done?.({ id: "cp6" });
    await pending;
  });

  it("a DTS-only film takes the server path too (the local pass never could)", async () => {
    fetchByUrl({ "/compat-download": () => ({ status: "ready" }) });
    const pending = startDownload("cp7", "DTS Film", undefined, { audioCodecs: ["dts"] });
    const handlers = await waitForTask("cp7");
    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      url: expect.stringContaining("/compat-download?token="),
      destination: expect.stringContaining("evey-cp7.mkv"),
    }));
    handlers.done?.({ id: "cp7" });
    await pending;
    const entry = lastIndexWritten().find((i) => i.mediaItemId === "cp7");
    expect(entry!.audioCodecs).toEqual(["aac"]);
  });

  it("a renderable-audio download never touches the compat endpoint", async () => {
    const pending = startDownload("cp5", "AAC Film", undefined, { audioCodecs: ["aac"] });
    const handlers = await waitForTask("cp5");
    const urls = eveyFetchMock.mock.calls.map((c) => (c[0] as { url: string }).url);
    expect(urls.some((u) => u.includes("compat-download"))).toBe(false);
    handlers.done?.({ id: "cp5" });
    await pending;
  });
});

// A list-fed caller (episode rows) has no codecs to hand over: ListMediaItems
// returns streams empty, and "no codecs" used to read as "renderable" - so an
// eac3-only episode downloaded raw, skipped every net and played silent
// (2026-08-03, Special ops lioness S03E01). The codecs now come from the item
// detail whenever a video caller has none.
describe("source codec fallback", () => {
  function fetchByUrl(routes: Record<string, () => unknown>) {
    eveyFetchMock.mockImplementation(async ({ url }: { url: string }) => {
      const hit = Object.keys(routes)
        .filter((s) => url.includes(s))
        .sort((a, b) => b.length - a.length)[0];
      if (hit) return routes[hit]();
      return { token: "stream-tok-123" };
    });
  }

  afterEach(() => {
    eveyFetchMock.mockResolvedValue({ token: "stream-tok-123", mediaId: "m3" });
    getMediaItemMock.mockResolvedValue({ id: "m1" });
  });

  it("fetches the detail's codecs when the caller has none, and takes the compat path", async () => {
    getMediaItemMock.mockResolvedValue({
      id: "ep1",
      streams: [
        { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264" },
        { type: "MEDIA_STREAM_TYPE_AUDIO", codec: "eac3" },
      ],
    });
    fetchByUrl({ "/compat-download": () => ({ status: "ready" }) });
    const pending = startDownload("ep1", "Episode 1", undefined, { formatHint: "matroska" });
    const handlers = await waitForTask("ep1");
    expect(getMediaItemMock).toHaveBeenCalledWith("ep1");
    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      url: expect.stringContaining("/compat-download?token="),
    }));
    handlers.begin?.({ expectedBytes: 5000, headers: {} });
    handlers.done?.({ id: "ep1" });
    await pending;
    const entry = lastIndexWritten().find((i) => i.mediaItemId === "ep1");
    expect(entry!.audioCodecs).toEqual(["aac"]);
  });

  it("keeps the raw path when the detail fetch fails", async () => {
    getMediaItemMock.mockRejectedValue(new Error("offline"));
    const pending = startDownload("ep2", "Episode 2", undefined, { formatHint: "matroska" });
    const handlers = await waitForTask("ep2");
    expect(createDownloadTaskMock).toHaveBeenCalledWith(expect.objectContaining({
      url: "https://evey.test/api/v1/media/ep2/download?token=stream-tok-123",
    }));
    handlers.done?.({ id: "ep2" });
    await pending;
  });

  it("records the detail's codecs even when they are renderable (raw path)", async () => {
    getMediaItemMock.mockResolvedValue({
      id: "ep3",
      streams: [{ type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac" }],
    });
    const pending = startDownload("ep3", "Episode 3", undefined, { formatHint: "matroska" });
    const handlers = await waitForTask("ep3");
    const urls = eveyFetchMock.mock.calls.map((c) => (c[0] as { url: string }).url);
    expect(urls.some((u) => u.includes("compat-download"))).toBe(false);
    handlers.done?.({ id: "ep3" });
    await pending;
    const entry = lastIndexWritten().find((i) => i.mediaItemId === "ep3");
    expect(entry!.audioCodecs).toEqual(["aac"]);
  });

  it("explicit caller codecs never trigger the detail fetch", async () => {
    const pending = startDownload("cp8", "AAC Film", undefined, { audioCodecs: ["aac"] });
    const handlers = await waitForTask("cp8");
    expect(getMediaItemMock).not.toHaveBeenCalled();
    handlers.done?.({ id: "cp8" });
    await pending;
  });

  it("a music download never fetches the detail", async () => {
    const pending = startDownload("tr1", "Track", undefined, { kind: "audio" });
    const handlers = await waitForTask("tr1");
    expect(getMediaItemMock).not.toHaveBeenCalled();
    handlers.done?.({ id: "tr1" });
    await pending;
  });
});
