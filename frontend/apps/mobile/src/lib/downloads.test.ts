import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("expo-file-system", () => ({
  getInfoAsync: vi.fn(),
  makeDirectoryAsync: vi.fn(),
  readAsStringAsync: vi.fn(),
  writeAsStringAsync: vi.fn(),
  deleteAsync: vi.fn(),
  documentDirectory: "/data/user/0/app/files/",
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
      stop: vi.fn(),
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
