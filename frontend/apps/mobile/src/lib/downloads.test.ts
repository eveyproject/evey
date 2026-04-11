import { describe, it, expect, vi, beforeEach } from "vitest";

vi.mock("expo-file-system", () => ({
  getInfoAsync: vi.fn(),
  makeDirectoryAsync: vi.fn(),
  readAsStringAsync: vi.fn(),
  writeAsStringAsync: vi.fn(),
  deleteAsync: vi.fn(),
  createDownloadResumable: vi.fn(),
  documentDirectory: "/data/user/0/app/files/",
  default: {
    getInfoAsync: vi.fn(),
    makeDirectoryAsync: vi.fn(),
    readAsStringAsync: vi.fn(),
    writeAsStringAsync: vi.fn(),
    deleteAsync: vi.fn(),
    createDownloadResumable: vi.fn(),
    documentDirectory: "/data/user/0/app/files/",
  },
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

import { getDownloads, startDownload, deleteDownload, getDownloadPath, getTotalDownloadSize } from "./downloads";

const sampleIndex = [
  { mediaItemId: "m1", title: "Movie 1", filePath: "/data/user/0/app/files/downloads/m1.mp4", fileSize: 1000, downloadedAt: "2025-01-01" },
  { mediaItemId: "m2", title: "Movie 2", filePath: "/data/user/0/app/files/downloads/m2.mp4", fileSize: 2000, downloadedAt: "2025-01-02" },
];

beforeEach(() => {
  vi.clearAllMocks();
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

describe("startDownload", () => {
  it("downloads file and appends to index", async () => {
    const downloadAsync = vi.fn().mockResolvedValue({ headers: { "content-length": "5000" } });
    fsMock.createDownloadResumable.mockReturnValue({ downloadAsync });

    const path = await startDownload("m3", "Movie 3");

    expect(eveyFetchMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m3/stream-token",
      method: "POST",
    });
    expect(fsMock.createDownloadResumable).toHaveBeenCalledWith(
      "https://evey.test/api/v1/media/m3/download?token=stream-tok-123",
      expect.stringContaining("m3.mp4"),
      {},
      expect.any(Function),
    );
    expect(path).toContain("m3.mp4");
    expect(fsMock.writeAsStringAsync).toHaveBeenCalled();
    const written = JSON.parse(fsMock.writeAsStringAsync.mock.calls[0][1]);
    expect(written).toHaveLength(3);
    expect(written[2].mediaItemId).toBe("m3");
  });

  it("calls onProgress callback", async () => {
    let progressCb: ((p: any) => void) | undefined;
    fsMock.createDownloadResumable.mockImplementation((_url: string, _dest: string, _opts: any, cb: any) => {
      progressCb = cb;
      return { downloadAsync: vi.fn().mockResolvedValue({ headers: {} }) };
    });
    const onProgress = vi.fn();

    await startDownload("m4", "Movie 4", onProgress);
    progressCb?.({ totalBytesWritten: 50, totalBytesExpectedToWrite: 100 });

    expect(onProgress).toHaveBeenCalledWith(0.5);
  });

  it("throws when download fails", async () => {
    fsMock.createDownloadResumable.mockReturnValue({
      downloadAsync: vi.fn().mockResolvedValue(null),
    });

    await expect(startDownload("m5", "Movie 5")).rejects.toThrow("Download failed");
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
