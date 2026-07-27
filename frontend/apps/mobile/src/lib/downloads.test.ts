import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("expo-file-system", () => ({
  getInfoAsync: vi.fn(),
  makeDirectoryAsync: vi.fn(),
  readAsStringAsync: vi.fn(),
  writeAsStringAsync: vi.fn(),
  deleteAsync: vi.fn(),
  documentDirectory: "/data/user/0/app/files/",
  // Free-space reading consulted by the pre-download guard. Generous by
  // default so existing tests are unaffected; the guard's own tests override it.
  Paths: { availableDiskSpace: 500 * 1024 * 1024 * 1024 },
  default: {
    getInfoAsync: vi.fn(),
    makeDirectoryAsync: vi.fn(),
    readAsStringAsync: vi.fn(),
    writeAsStringAsync: vi.fn(),
    deleteAsync: vi.fn(),
    documentDirectory: "/data/user/0/app/files/",
  },
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

vi.mock("@kesha-antonov/react-native-background-downloader", () => ({
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
}));

vi.mock("./server-config", () => ({
  getServerUrl: vi.fn().mockResolvedValue("https://evey.test"),
}));

import * as FileSystem from "expo-file-system";
const fsMock = FileSystem as unknown as Record<string, ReturnType<typeof vi.fn>>;

import { eveyFetch } from "@evey/api";
const eveyFetchMock = eveyFetch as ReturnType<typeof vi.fn>;

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
  getDownloadPath,
  getTotalDownloadSize,
  rehydrateBackgroundDownloads,
  getInFlightDownloads,
} from "./downloads";

const sampleIndex = [
  { mediaItemId: "m1", title: "Movie 1", filePath: "/data/user/0/app/files/downloads/m1.mp4", fileSize: 1000, downloadedAt: "2025-01-01" },
  { mediaItemId: "m2", title: "Movie 2", filePath: "/data/user/0/app/files/downloads/m2.mp4", fileSize: 2000, downloadedAt: "2025-01-02" },
];

beforeEach(() => {
  vi.clearAllMocks();
  taskInstances.clear();
  existingTasks.length = 0;
  fsMock.getInfoAsync.mockResolvedValue({ exists: true, size: 0 });
  fsMock.readAsStringAsync.mockResolvedValue(JSON.stringify(sampleIndex));
  fsMock.writeAsStringAsync.mockResolvedValue(undefined);
  fsMock.makeDirectoryAsync.mockResolvedValue(undefined);
  fsMock.deleteAsync.mockResolvedValue(undefined);
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
      metadata: { title: "Movie 3" },
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
    expect(fsMock.writeAsStringAsync).toHaveBeenCalled();
    const written = JSON.parse(fsMock.writeAsStringAsync.mock.calls[0][1]);
    expect(written).toHaveLength(3);
    expect(written[2].mediaItemId).toBe("m3");
    expect(written[2].fileSize).toBe(5000);
  });

  it("calls onProgress callback with fraction", async () => {
    const onProgress = vi.fn();
    const pending = startDownload("m4", "Movie 4", onProgress);
    const handlers = await waitForTask("m4");
    handlers.progress?.({ bytesDownloaded: 50, bytesTotal: 100 });
    handlers.done?.({ id: "m4" });
    await pending;
    expect(onProgress).toHaveBeenCalledWith(0.5);
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
    const written = fsMock.writeAsStringAsync.mock.calls.at(-1);
    expect(written).toBeTruthy();
    const idx = JSON.parse(written![1]);
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

    const written = fsMock.writeAsStringAsync.mock.calls.at(-1);
    expect(written).toBeTruthy();
    const idx = JSON.parse(written![1]);
    const entry = idx.find((i: { mediaItemId: string }) => i.mediaItemId === "m11");
    expect(entry.fileSize).toBe(42424);
  });
});

describe("deleteDownload", () => {
  it("removes file and updates index", async () => {
    await deleteDownload("m1");

    expect(fsMock.deleteAsync).toHaveBeenCalledWith(
      "/data/user/0/app/files/downloads/m1.mp4",
      { idempotent: true },
    );
    const written = JSON.parse(fsMock.writeAsStringAsync.mock.calls[0][1]);
    expect(written).toHaveLength(1);
    expect(written[0].mediaItemId).toBe("m2");
  });

  it("updates index even if item not found", async () => {
    await deleteDownload("nonexistent");

    expect(fsMock.deleteAsync).not.toHaveBeenCalled();
    const written = JSON.parse(fsMock.writeAsStringAsync.mock.calls[0][1]);
    expect(written).toHaveLength(2);
  });
});

describe("getDownloadPath", () => {
  it("returns path if file exists", async () => {
    fsMock.getInfoAsync.mockResolvedValue({ exists: true });
    const path = await getDownloadPath("m1");
    expect(path).toBe("/data/user/0/app/files/downloads/m1.mp4");
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
    (FileSystem as unknown as { Paths: { availableDiskSpace: number } }).Paths = {
      availableDiskSpace: 2 * 1024 * 1024 * 1024,
    };

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
    (FileSystem as unknown as { Paths: { availableDiskSpace: number } }).Paths = {
      availableDiskSpace: 50 * 1024 * 1024 * 1024,
    };

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

describe("getDownloadPath extension fallback", () => {
  it("finds the file on disk under a different known extension than the index records", async () => {
    fsMock.readAsStringAsync.mockResolvedValue(JSON.stringify([
      { mediaItemId: "old1", title: "Old", filePath: "/data/user/0/app/files/Documents/evey-old1.mp4", fileSize: 100, downloadedAt: "2025-01-01" },
    ]));
    fsMock.getInfoAsync.mockImplementation(async (path: string) => ({
      exists: path === "/data/user/0/app/files/Documents/evey-old1.mkv",
    }));

    const path = await getDownloadPath("old1");
    expect(path).toBe("/data/user/0/app/files/Documents/evey-old1.mkv");
  });
});
