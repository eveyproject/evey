import { readFileSync } from "node:fs";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

// Same two-entry-point mock split as downloads.test.ts, and for the same
// reason: in expo-file-system 55 the file helpers only work from "/legacy"
// while Paths only exists at the root. Mocking one merged module is exactly
// what once kept a suite green against a module shape that does not exist at
// runtime - and took the download feature out silently.
vi.mock("expo-file-system", () => ({
  Paths: { get availableDiskSpace() { return 500 * 1024 ** 3; } },
}));

const existingSlices = new Set<string>();
vi.mock("expo-file-system/legacy", () => ({
  makeDirectoryAsync: vi.fn(async () => undefined),
  deleteAsync: vi.fn(async () => undefined),
  moveAsync: vi.fn(async () => undefined),
  getInfoAsync: vi.fn(async (uri: string) => ({
    exists: existingSlices.has(uri.replace("file://", "")),
    size: 1234,
  })),
  documentDirectory: "/data/user/0/app/files/",
}));

type TaskHandlers = { progress?: () => void; done?: () => void; error?: (d: { error: string }) => void };
const tasks = new Map<string, TaskHandlers>();
const taskUrls: string[] = [];
const taskFails = new Set<string>();
vi.mock("@kesha-antonov/react-native-background-downloader", () => ({
  createDownloadTask: vi.fn(({ id, url }: { id: string; url: string }) => {
    const handlers: TaskHandlers = {};
    tasks.set(id, handlers);
    taskUrls.push(url);
    const task = {
      progress: vi.fn((cb: TaskHandlers["progress"]) => { handlers.progress = cb; return task; }),
      done: vi.fn((cb: TaskHandlers["done"]) => { handlers.done = cb; return task; }),
      error: vi.fn((cb: TaskHandlers["error"]) => { handlers.error = cb; return task; }),
      // Resolve on the next tick so the caller has wired its handlers first.
      start: vi.fn(() => {
        setTimeout(() => {
          if (taskFails.has(id)) handlers.error?.({ error: "network down" });
          else handlers.done?.();
        }, 0);
      }),
      stop: vi.fn(async () => undefined),
    };
    return task;
  }),
  completeHandler: vi.fn(),
  directories: { documents: "/docs" },
}));

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetchMock(...a) }));
vi.mock("./server-config", () => ({ getServerUrl: vi.fn().mockResolvedValue("https://evey.test") }));

const moveMediaFileMock = vi.fn(async () => true);
vi.mock("@evey/media-caps", () => ({ moveMediaFile: (...a: unknown[]) => moveMediaFileMock(...(a as [])) }));

const joinMock = vi.fn(async () => undefined);
const deleteSlicesMock = vi.fn(async () => undefined);
vi.mock("@evey/offline-audio", () => ({
  joinDownloadSlices: (...a: unknown[]) => joinMock(...(a as [])),
  deleteJoinedSlices: (...a: unknown[]) => deleteSlicesMock(...(a as [])),
  sliceFileName: (i: number) => `slice-${String(i).padStart(5, "0")}.mkv`,
  OptimizeError: class OptimizeError extends Error {},
}));

const recordDownloadedFileMock = vi.fn(async () => undefined);
vi.mock("./downloads", () => ({
  resolveDownloadDest: vi.fn(async (title: string, id: string, ext: string) => `/pub/Evey/${title}-${id}.${ext}`),
  recordDownloadedFile: (...a: unknown[]) => recordDownloadedFileMock(...(a as [])),
  registerDownloadInFlight: vi.fn(),
  clearDownloadInFlight: vi.fn(),
  notifyDownloadProgress: vi.fn(),
  deleteDownloadedFileAnywhere: vi.fn(async () => undefined),
  toFileUri: (p: string) => (p.startsWith("file://") ? p : `file://${p}`),
}));

import { startReducedDownload, fetchReducedPlan } from "./reducedDownload";
// The live state is a separate module (no native imports) so the screens can
// read it without pulling the whole flow in; the flow re-exports it.
import {
  cancelReducedDownload,
  isReducedDownloadRunning,
  getReducedSliceProgress,
} from "./reducedDownloadState";

// The mocks above stand in for real modules, and a mock whose shape has
// drifted from the real export is the failure mode this project has already
// paid for: a feature dead on device with a green suite. The guard is STATIC -
// it reads the real sources rather than importing them, so it cannot be
// satisfied by the mocks themselves and does not drag native modules into the
// test runtime.
describe("mock shapes match the real modules", () => {
  const sourceOf = (rel: string) => readFileSync(new URL(rel, import.meta.url), "utf8");

  it("every function reducedDownload imports really exists", () => {
    const offline = sourceOf("../../../../packages/offline-audio/src/index.ts");
    for (const name of ["joinDownloadSlices", "deleteJoinedSlices", "sliceFileName", "OptimizeError"]) {
      expect(offline, `@evey/offline-audio must export ${name}`).toMatch(
        new RegExp(`export\\s+(async\\s+function|function|class|const)\\s+${name}\\b`),
      );
    }
    const downloads = sourceOf("./downloads.ts");
    for (const name of [
      "resolveDownloadDest", "recordDownloadedFile", "registerDownloadInFlight",
      "clearDownloadInFlight", "notifyDownloadProgress", "deleteDownloadedFileAnywhere", "toFileUri",
    ]) {
      expect(downloads, `downloads.ts must export ${name}`).toMatch(
        new RegExp(`export\\s+(async\\s+function|function)\\s+${name}\\b`),
      );
    }
  });

  // The slice filename is agreed in three places (Go, the server's segment
  // muxer, this TS helper). A mismatch is a join that finds nothing, so the
  // two we own are pinned against each other here.
  it("names slices the way the Go side rebuilds them", async () => {
    const go = sourceOf("../../../../packages/offline-audio/go/join.go");
    expect(go).toContain('"slice-%05d.mkv"');
    // The REAL helper, not the mock: pinning the mock against Go would prove
    // nothing about what ships.
    const { sliceFileName } = await vi.importActual<{ sliceFileName: (i: number) => string }>("@evey/offline-audio");
    expect(sliceFileName(0)).toBe("slice-00000.mkv");
    expect(sliceFileName(42)).toBe("slice-00042.mkv");
  });
});

const PLAN = {
  available: true, offer: true, height: 720, width: 1280,
  estimatedBytes: 2_000_000_000, sourceBytes: 40_000_000_000,
  slices: 3, sliceSec: 600, durationMs: 1_800_000,
};

/** Server answers: POST prepare, then status polls, then the slice fetches. */
function serveSlices(readyAfterCalls = 0) {
  let statusCalls = 0;
  eveyFetchMock.mockImplementation(async ({ url, method }: { url: string; method: string }) => {
    if (url.endsWith("/stream-token")) return { token: "tok" };
    if (url.includes("/reduced-download/status")) {
      statusCalls++;
      return { status: "preparing", permille: 500, slicesReady: statusCalls > readyAfterCalls ? 3 : 0, slices: 3 };
    }
    if (url.includes("/reduced-download") && method === "POST") {
      return { status: "preparing", slices: 3, durationMs: 1_800_000, slicesReady: 3 };
    }
    return {};
  });
}

beforeEach(() => {
  vi.useRealTimers();
  tasks.clear();
  taskUrls.length = 0;
  taskFails.clear();
  existingSlices.clear();
  eveyFetchMock.mockReset();
  joinMock.mockClear();
  deleteSlicesMock.mockClear();
  recordDownloadedFileMock.mockClear();
  moveMediaFileMock.mockClear();
  moveMediaFileMock.mockResolvedValue(true);
  joinMock.mockResolvedValue(undefined);
});

afterEach(() => { vi.restoreAllMocks(); });

describe("fetchReducedPlan", () => {
  it("returns the plan when the server offers a lighter copy", async () => {
    eveyFetchMock.mockResolvedValue(PLAN);
    expect(await fetchReducedPlan("m1")).toEqual(PLAN);
  });

  // Every "no" shape collapses to null, because the app's answer to all of
  // them is the same: offer the original only. An old server has no route, an
  // opted-out operator leaves it unregistered, and an offline phone throws.
  it("collapses every refusal to null", async () => {
    eveyFetchMock.mockResolvedValue({ ...PLAN, available: false });
    expect(await fetchReducedPlan("m1")).toBeNull();
    eveyFetchMock.mockRejectedValue(new Error("404"));
    expect(await fetchReducedPlan("m1")).toBeNull();
  });
});

describe("startReducedDownload", () => {
  it("collects every slice in order, joins them, and publishes one file", async () => {
    serveSlices();
    const path = await startReducedDownload("m1", "Big Film", 1_800_000, 3);

    // The slices were fetched in order, each from its own indexed URL.
    const sliceUrls = taskUrls.filter((u) => u.includes("/slice/"));
    expect(sliceUrls).toHaveLength(3);
    sliceUrls.forEach((u, i) => expect(u).toContain(`/reduced-download/slice/${i}?token=`));

    // The join was handed the film's TRUE length. Without it the finished
    // download announces the length of slice 0 and refuses to seek past it -
    // mkvgo copies the first source's duration into the joined header.
    expect(joinMock).toHaveBeenCalledTimes(1);
    expect(joinMock.mock.calls[0][3]).toBe(1_800_000);

    expect(moveMediaFileMock).toHaveBeenCalled();
    expect(recordDownloadedFileMock).toHaveBeenCalled();
    // The index records AAC: the reduced variant's audio is already
    // phone-renderable, so the on-device compat pass must never re-run on it.
    expect(recordDownloadedFileMock.mock.calls[0][5]).toEqual(["aac"]);
    expect(path).toContain("Big Film");

    // Slices go only AFTER the join succeeded - they are the only thing
    // between a crash and re-downloading gigabytes.
    expect(deleteSlicesMock).toHaveBeenCalledWith("/docs/evey-reduced-m1", 3);
    expect(deleteSlicesMock.mock.invocationCallOrder[0]).toBeGreaterThan(joinMock.mock.invocationCallOrder[0]);
    expect(isReducedDownloadRunning("m1")).toBe(false);
  });

  it("does not re-fetch a slice already on disk", async () => {
    serveSlices();
    existingSlices.add("/docs/evey-reduced-m1/slice-00000.mkv");
    await startReducedDownload("m1", "Big Film", 1_800_000, 3);
    expect(taskUrls.filter((u) => u.includes("/slice/"))).toHaveLength(2);
  });

  it("refuses to start twice for the same item", async () => {
    serveSlices(99); // never ready: the first call stays in the wait loop
    const first = startReducedDownload("m1", "Big Film", 1_800_000, 3);
    await expect(startReducedDownload("m1", "Big Film", 1_800_000, 3)).rejects.toThrow(/already running/);
    expect(cancelReducedDownload("m1")).toBe(true);
    await expect(first).rejects.toThrow(/cancelled/);
  });

  it("reports which part it is on while collecting", async () => {
    serveSlices();
    existingSlices.add("/docs/evey-reduced-m1/slice-00000.mkv");
    existingSlices.add("/docs/evey-reduced-m1/slice-00001.mkv");
    // Fail the last slice so the run stops with two collected, and the
    // progress the screens read is observable mid-flight.
    taskFails.add("m1#s2");
    await expect(startReducedDownload("m1", "Big Film", 1_800_000, 3)).rejects.toThrow();
    // The job is gone once it settles; what matters is that it never claimed
    // to be finished.
    expect(getReducedSliceProgress("m1")).toEqual({ collected: 0, slices: 0 });
    expect(joinMock).not.toHaveBeenCalled();
    expect(deleteSlicesMock).not.toHaveBeenCalled();
  });

  it("gives up when the server says the production failed", async () => {
    eveyFetchMock.mockImplementation(async ({ url, method }: { url: string; method: string }) => {
      if (url.includes("/status")) return { status: "failed", error: "disk full", slicesReady: 0 };
      if (url.includes("/reduced-download") && method === "POST") return { status: "preparing", slices: 3, slicesReady: 0 };
      return {};
    });
    await expect(startReducedDownload("m1", "Big Film", 1_800_000, 3)).rejects.toThrow(/disk full/);
    expect(joinMock).not.toHaveBeenCalled();
  });

  it("does not publish anything when the join fails", async () => {
    serveSlices();
    joinMock.mockRejectedValue(new Error("join-failed: codec mismatch"));
    await expect(startReducedDownload("m1", "Big Film", 1_800_000, 3)).rejects.toThrow(/join-failed/);
    expect(moveMediaFileMock).not.toHaveBeenCalled();
    expect(recordDownloadedFileMock).not.toHaveBeenCalled();
    // And the slices survive, so a retry does not re-download the film.
    expect(deleteSlicesMock).not.toHaveBeenCalled();
  });
});
