import { describe, it, expect, vi, beforeEach } from "vitest";

// The pass orchestrator only touches three file-system verbs; all three are
// legacy-entry-point imports for the same reason as downloads.ts (the root
// re-exports are throwing stubs in expo-file-system 55).
const fsMock = vi.hoisted(() => ({
  deleteAsync: vi.fn(async (): Promise<void> => undefined),
  moveAsync: vi.fn(async (): Promise<void> => undefined),
  getInfoAsync: vi.fn(async () => ({ exists: true, size: 4_000_000 })),
}));
vi.mock("expo-file-system/legacy", () => fsMock);

// @evey/offline-audio is our own TS package; the mock mirrors its real export
// shape (checked against src/index.ts) including the error class, because the
// orchestrator routes on `err.code`.
const oa = vi.hoisted(() => {
  class OptimizeError extends Error {
    readonly code: string;
    constructor(code: string, message: string) {
      super(message);
      this.name = "OptimizeError";
      this.code = code;
    }
  }
  return {
    OptimizeError,
    isOfflineAudioAvailable: vi.fn(async () => true),
    optimizeAudio: vi.fn(async () => ({
      sampleRate: 48000,
      sourceCodec: "ac3",
      sourceChannels: 6,
      aacFrames: 100,
      skippedFrames: 0,
    })),
    cancelOptimize: vi.fn(async (): Promise<void> => undefined),
  };
});
vi.mock("@evey/offline-audio", () => oa);

const deviceCodecsMock = vi.hoisted(() => ({
  getDeviceAudioCodecs: vi.fn((): string[] => ["aac", "opus"]),
}));
vi.mock("./deviceCodecs", () => deviceCodecsMock);

// Native File helpers used FIRST for the swap (expo's legacy FS rejects
// public-Downloads paths outright). False by default so the bulk of the suite
// exercises the expo fallback; the public-path test flips them to true.
const mediaCaps = vi.hoisted(() => ({
  deleteMediaFile: vi.fn(async (_p: string): Promise<boolean> => false),
  moveMediaFile: vi.fn(async (_f: string, _t: string): Promise<boolean> => false),
}));
vi.mock("@evey/media-caps", () => mediaCaps);

const dl = vi.hoisted(() => ({
  getDownloadedMedia: vi.fn(),
  getDownloads: vi.fn(async (): Promise<unknown[]> => []),
  needsAudioCompatPass: vi.fn(() => true),
  toFileUri: (p: string) => (p.startsWith("file://") ? p : `file://${p}`),
  updateDownloadedAudio: vi.fn(async (): Promise<void> => undefined),
  refreshDownloadedAudioCodecs: vi.fn(async (): Promise<string[] | undefined> => undefined),
}));
vi.mock("./downloads", () => dl);


import {
  runAudioCompatPass,
  resumePendingAudioPasses,
  getAudioPassState,
  subscribeToAudioPass,
} from "./audioCompatPass";

const MKV_PATH = "/storage/emulated/0/Download/Evey/Mon Film-01234567.mkv";
const OPT_PATH = "/storage/emulated/0/Download/Evey/Mon Film-01234567.opt.mkv";

function entry(overrides: Record<string, unknown> = {}) {
  return {
    mediaItemId: "m1",
    title: "Mon Film",
    filePath: MKV_PATH,
    fileSize: 19_000_000,
    downloadedAt: "2026-08-01T00:00:00.000Z",
    audioCodecs: ["ac3"],
    ...overrides,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  mediaCaps.deleteMediaFile.mockResolvedValue(false);
  mediaCaps.moveMediaFile.mockResolvedValue(false);
  dl.getDownloadedMedia.mockResolvedValue(entry());
  dl.needsAudioCompatPass.mockReturnValue(true);
  dl.getDownloads.mockResolvedValue([]);
  dl.refreshDownloadedAudioCodecs.mockResolvedValue(undefined);
  oa.isOfflineAudioAvailable.mockResolvedValue(true);
  oa.optimizeAudio.mockResolvedValue({
    sampleRate: 48000,
    sourceCodec: "ac3",
    sourceChannels: 6,
    aacFrames: 100,
    skippedFrames: 0,
  });
  fsMock.getInfoAsync.mockResolvedValue({ exists: true, size: 4_000_000 });
});

describe("runAudioCompatPass success", () => {
  it("optimizes next to the source, swaps, and patches the index", async () => {
    const result = await runAudioCompatPass("m1");
    expect(result).toBe("done");

    expect(oa.optimizeAudio).toHaveBeenCalledWith(MKV_PATH, OPT_PATH, expect.any(Function), "Mon Film", "Optimizing audio");
    // Swap order matters: the original goes first, then the rewrite takes its
    // exact path (same name, so VLC/file-manager visibility is unchanged).
    expect(fsMock.deleteAsync).toHaveBeenCalledWith(`file://${MKV_PATH}`, { idempotent: true });
    expect(fsMock.moveAsync).toHaveBeenCalledWith({ from: `file://${OPT_PATH}`, to: `file://${MKV_PATH}` });
    const deleteOrder = fsMock.deleteAsync.mock.invocationCallOrder[0];
    const moveOrder = fsMock.moveAsync.mock.invocationCallOrder[0];
    expect(deleteOrder).toBeLessThan(moveOrder);

    expect(dl.updateDownloadedAudio).toHaveBeenCalledWith("m1", {
      fileSize: 4_000_000,
      audioCodecs: ["aac"],
      filePath: MKV_PATH,
    });
  });

  it("only rewrites the converted codec, other tracks keep theirs", async () => {
    dl.getDownloadedMedia.mockResolvedValue(entry({ audioCodecs: ["dts", "ac3", "ac3"] }));
    await runAudioCompatPass("m1");
    expect(dl.updateDownloadedAudio).toHaveBeenCalledWith("m1", {
      fileSize: 4_000_000,
      audioCodecs: ["dts", "aac", "ac3"],
      filePath: MKV_PATH,
    });
  });

  it("runs on a misnamed .mp4 and renames the rewrite to .mkv", async () => {
    // The movie screen used to omit its formatHint, so Matroska bytes were
    // saved under a ".mp4" name - the pass must trust the sniffer, not the
    // extension, and the swapped file takes the honest name.
    const mp4Path = "/storage/emulated/0/Download/Evey/Film-01234567.mp4";
    const fixedPath = "/storage/emulated/0/Download/Evey/Film-01234567.mkv";
    dl.getDownloadedMedia.mockResolvedValue(entry({ filePath: mp4Path }));

    const result = await runAudioCompatPass("m1");
    expect(result).toBe("done");
    expect(oa.optimizeAudio).toHaveBeenCalledWith(
      mp4Path,
      "/storage/emulated/0/Download/Evey/Film-01234567.opt.mkv",
      expect.any(Function),
      "Mon Film",
      "Optimizing audio",
    );
    expect(fsMock.moveAsync).toHaveBeenCalledWith({
      from: "file:///storage/emulated/0/Download/Evey/Film-01234567.opt.mkv",
      to: `file://${fixedPath}`,
    });
    expect(dl.updateDownloadedAudio).toHaveBeenCalledWith("m1", expect.objectContaining({ filePath: fixedPath }));
  });

  it("swaps via the native File helpers when they work (public paths)", async () => {
    // The field failure this pins: expo's legacy FS rejected the swap of a
    // finished 1.7 GB rewrite because the file lives in public Downloads.
    mediaCaps.deleteMediaFile.mockResolvedValue(true);
    mediaCaps.moveMediaFile.mockResolvedValue(true);

    const result = await runAudioCompatPass("m1");
    expect(result).toBe("done");
    expect(mediaCaps.deleteMediaFile).toHaveBeenCalledWith(MKV_PATH);
    expect(mediaCaps.moveMediaFile).toHaveBeenCalledWith(OPT_PATH, MKV_PATH);
    // The expo path must not even be attempted once the native one succeeded.
    expect(fsMock.deleteAsync).not.toHaveBeenCalled();
    expect(fsMock.moveAsync).not.toHaveBeenCalled();
  });

  it("exposes weighted progress while running, cleared after", async () => {
    const seen: Array<ReturnType<typeof getAudioPassState>> = [];
    oa.optimizeAudio.mockImplementation(async (_in: string, _out: string, onProgress: (p: { stage: string; permille: number }) => void) => {
      onProgress({ stage: "transcode", permille: 500 });
      seen.push(getAudioPassState("m1"));
      onProgress({ stage: "remux", permille: 500 });
      seen.push(getAudioPassState("m1"));
      return { sampleRate: 48000, sourceCodec: "ac3", sourceChannels: 6, aacFrames: 1, skippedFrames: 0 };
    });
    const bumps = vi.fn();
    const unsub = subscribeToAudioPass(bumps);
    await runAudioCompatPass("m1");
    unsub();

    // transcode is weighted 0..800, remux 800..1000.
    expect(seen[0]).toEqual({ stage: "transcode", permille: 400 });
    expect(seen[1]).toEqual({ stage: "remux", permille: 900 });
    expect(getAudioPassState("m1")).toBeNull();
    expect(bumps).toHaveBeenCalled();
  });
});

describe("runAudioCompatPass failures", () => {
  it("records a terminal failure so the sweep never retries it", async () => {
    oa.optimizeAudio.mockRejectedValue(new oa.OptimizeError("unsupported-codec", "unsupported-codec:dts"));
    const result = await runAudioCompatPass("m1");
    expect(result).toBe("failed");
    expect(dl.updateDownloadedAudio).toHaveBeenCalledWith("m1", { audioPassFailed: "unsupported-codec" });
    // Belt: the partial output is cleaned even though the native side also does.
    expect(fsMock.deleteAsync).toHaveBeenCalledWith(`file://${OPT_PATH}`, { idempotent: true });
    // The original was never touched.
    expect(fsMock.moveAsync).not.toHaveBeenCalled();
    expect(fsMock.deleteAsync).not.toHaveBeenCalledWith(`file://${MKV_PATH}`, { idempotent: true });
  });

  it("leaves a transient failure retryable (no index marker)", async () => {
    oa.optimizeAudio.mockRejectedValue(new oa.OptimizeError("cancelled", "optimization cancelled"));
    const result = await runAudioCompatPass("m1");
    expect(result).toBe("failed");
    expect(dl.updateDownloadedAudio).not.toHaveBeenCalled();
  });

  it("clears the visible state even on failure", async () => {
    oa.optimizeAudio.mockRejectedValue(new Error("boom"));
    await runAudioCompatPass("m1");
    expect(getAudioPassState("m1")).toBeNull();
  });
});

describe("runAudioCompatPass skips", () => {
  it.each([
    ["unknown entry", () => dl.getDownloadedMedia.mockResolvedValue(null)],
    ["renderable audio", () => dl.needsAudioCompatPass.mockReturnValue(false)],
    ["recorded terminal failure", () => dl.getDownloadedMedia.mockResolvedValue(entry({ audioPassFailed: "unsupported-codec" }))],
    ["native module absent", () => oa.isOfflineAudioAvailable.mockResolvedValue(false)],
  ])("skips on %s without calling the native pass", async (_name, arrange) => {
    arrange();
    const result = await runAudioCompatPass("m1");
    expect(result).toBe("skipped");
    expect(oa.optimizeAudio).not.toHaveBeenCalled();
    expect(fsMock.deleteAsync).not.toHaveBeenCalled();
  });
});


// An entry without recorded codecs (list-fed episode, legacy download) used to
// read as "renderable" and skip forever - the exact silent-file case the pass
// exists to remove. The truth now comes from the server once, then the normal
// policy applies; unreachable stays retryable at the next sweep.
describe("unknown codecs", () => {
  it("asks the server for the codecs, then runs the pass on their answer", async () => {
    dl.getDownloadedMedia.mockResolvedValue(entry({ audioCodecs: undefined }));
    dl.refreshDownloadedAudioCodecs.mockResolvedValue(["eac3"]);
    oa.optimizeAudio.mockResolvedValue({
      sampleRate: 48000,
      sourceCodec: "eac3",
      sourceChannels: 6,
      aacFrames: 100,
      skippedFrames: 0,
    });
    const result = await runAudioCompatPass("m1");
    expect(result).toBe("done");
    expect(dl.refreshDownloadedAudioCodecs).toHaveBeenCalledWith("m1");
    expect(dl.needsAudioCompatPass).toHaveBeenCalledWith(
      expect.objectContaining({ audioCodecs: ["eac3"] }),
      expect.anything(),
    );
    expect(dl.updateDownloadedAudio).toHaveBeenCalledWith("m1", expect.objectContaining({ audioCodecs: ["aac"] }));
  });

  it("stays retryable when the server has no answer", async () => {
    dl.getDownloadedMedia.mockResolvedValue(entry({ audioCodecs: undefined }));
    dl.refreshDownloadedAudioCodecs.mockResolvedValue(undefined);
    const result = await runAudioCompatPass("m1");
    expect(result).toBe("skipped");
    expect(oa.optimizeAudio).not.toHaveBeenCalled();
    expect(dl.updateDownloadedAudio).not.toHaveBeenCalled();
  });

  it("known codecs never hit the network", async () => {
    await runAudioCompatPass("m1");
    expect(dl.refreshDownloadedAudioCodecs).not.toHaveBeenCalled();
  });
});

describe("serialization", () => {
  it("runs passes one at a time, in call order", async () => {
    const order: string[] = [];
    let releaseFirst!: () => void;
    const gate = new Promise<void>((r) => { releaseFirst = r; });
    oa.optimizeAudio.mockImplementation(async (inPath: string) => {
      order.push(`start ${inPath}`);
      if (order.length === 1) await gate;
      order.push(`end ${inPath}`);
      return { sampleRate: 48000, sourceCodec: "ac3", sourceChannels: 6, aacFrames: 1, skippedFrames: 0 };
    });
    dl.getDownloadedMedia.mockImplementation(async (id: string) =>
      entry({ mediaItemId: id, filePath: `/d/${id}.mkv` }),
    );

    const p1 = runAudioCompatPass("m1");
    const p2 = runAudioCompatPass("m2");
    // Let the first pass cross its several awaits and reach the gate; the
    // second must still not have started (it is queued behind the chain).
    await vi.waitFor(() => expect(order).toEqual(["start /d/m1.mkv"]));
    await new Promise((r) => setTimeout(r, 10));
    expect(order).toEqual(["start /d/m1.mkv"]);
    releaseFirst();
    await Promise.all([p1, p2]);
    expect(order).toEqual(["start /d/m1.mkv", "end /d/m1.mkv", "start /d/m2.mkv", "end /d/m2.mkv"]);
  });
});

describe("resumePendingAudioPasses", () => {
  it("attempts every indexed entry, serialized", async () => {
    dl.getDownloads.mockResolvedValue([entry({ mediaItemId: "a" }), entry({ mediaItemId: "b" })]);
    dl.getDownloadedMedia.mockImplementation(async (id: string) => entry({ mediaItemId: id, filePath: `/d/${id}.mkv` }));
    await resumePendingAudioPasses();
    expect(oa.optimizeAudio).toHaveBeenCalledTimes(2);
    expect(oa.optimizeAudio.mock.calls[0][0]).toBe("/d/a.mkv");
    expect(oa.optimizeAudio.mock.calls[1][0]).toBe("/d/b.mkv");
  });

  it("survives an unreadable index", async () => {
    dl.getDownloads.mockRejectedValue(new Error("fs gone"));
    await expect(resumePendingAudioPasses()).resolves.toBeUndefined();
  });
});
