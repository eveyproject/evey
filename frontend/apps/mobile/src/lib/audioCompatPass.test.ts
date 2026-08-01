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
}));
vi.mock("./downloads", () => dl);

// Real implementation of the pure helpers (conditionsMet/waitingReasonFor)
// so the gating logic under test is the actual one, not a re-derived mock;
// only the native-facing probe/wait functions are faked.
const pc = vi.hoisted(() => {
  function conditionsMet(c: { charging: boolean; unmeteredWifi: boolean }) {
    return c.charging && c.unmeteredWifi;
  }
  function waitingReasonFor(c: { charging: boolean; unmeteredWifi: boolean }) {
    if (conditionsMet(c)) return null;
    if (!c.charging && !c.unmeteredWifi) return "charge-and-wifi";
    return c.charging ? "wifi" : "charge";
  }
  return {
    conditionsMet,
    waitingReasonFor,
    getPassConditions: vi.fn(async () => ({ charging: true, unmeteredWifi: true })),
    waitForPassConditions: vi.fn(async () => undefined),
  };
});
vi.mock("./passConditions", () => pc);

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
  oa.isOfflineAudioAvailable.mockResolvedValue(true);
  pc.getPassConditions.mockResolvedValue({ charging: true, unmeteredWifi: true });
  pc.waitForPassConditions.mockResolvedValue(undefined);
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

    expect(oa.optimizeAudio).toHaveBeenCalledWith(MKV_PATH, OPT_PATH, expect.any(Function), "Mon Film");
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

describe("runAudioCompatPass conditions gating", () => {
  it("defers with a visible waiting state instead of running when unplugged/cellular", async () => {
    pc.getPassConditions.mockResolvedValue({ charging: false, unmeteredWifi: false });
    let releaseWait!: () => void;
    pc.waitForPassConditions.mockImplementation(
      (onUpdate?: (c: { charging: boolean; unmeteredWifi: boolean }) => void) =>
        new Promise<void>((resolve) => {
          releaseWait = resolve;
          onUpdate?.({ charging: false, unmeteredWifi: false });
        }),
    );

    const promise = runAudioCompatPass("m1");
    await vi.waitFor(() => expect(getAudioPassState("m1")).not.toBeNull());
    // A skip would be indistinguishable from "nothing to do" - the whole
    // point is that the state says WHY it hasn't started.
    expect(getAudioPassState("m1")).toEqual({ stage: "waiting", permille: 0, waitingReason: "charge-and-wifi" });
    expect(oa.optimizeAudio).not.toHaveBeenCalled();

    releaseWait();
    const result = await promise;
    expect(result).toBe("done");
    expect(oa.optimizeAudio).toHaveBeenCalled();
    expect(getAudioPassState("m1")).toBeNull();
  });

  it("runs immediately when charging on unmetered Wi-Fi", async () => {
    pc.getPassConditions.mockResolvedValue({ charging: true, unmeteredWifi: true });
    const result = await runAudioCompatPass("m1");
    expect(result).toBe("done");
    expect(pc.waitForPassConditions).not.toHaveBeenCalled();
    expect(oa.optimizeAudio).toHaveBeenCalled();
  });

  it("refreshes the waiting reason when only one condition is still missing", async () => {
    // Real waitForPassConditions only calls onUpdate while conditions remain
    // unmet (it resolves instead once both flip true) - this mock respects
    // that invariant: Wi-Fi arrived first, the charger is still missing.
    pc.getPassConditions.mockResolvedValue({ charging: false, unmeteredWifi: false });
    let seenDuringWait: unknown;
    pc.waitForPassConditions.mockImplementation(
      async (onUpdate?: (c: { charging: boolean; unmeteredWifi: boolean }) => void) => {
        onUpdate?.({ charging: false, unmeteredWifi: true });
        seenDuringWait = getAudioPassState("m1");
      },
    );
    const result = await runAudioCompatPass("m1");
    expect(result).toBe("done");
    expect(seenDuringWait).toEqual({ stage: "waiting", permille: 0, waitingReason: "charge" });
  });

  it("never starts two passes concurrently while one is waiting on conditions", async () => {
    pc.getPassConditions.mockResolvedValueOnce({ charging: false, unmeteredWifi: false }).mockResolvedValue({ charging: true, unmeteredWifi: true });
    let releaseWait!: () => void;
    pc.waitForPassConditions.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          releaseWait = resolve;
        }),
    );
    dl.getDownloadedMedia.mockImplementation(async (id: string) => entry({ mediaItemId: id, filePath: `/d/${id}.mkv` }));

    const p1 = runAudioCompatPass("m1");
    const p2 = runAudioCompatPass("m2");
    await vi.waitFor(() => expect(getAudioPassState("m1")).not.toBeNull());
    await new Promise((r) => setTimeout(r, 10));
    // m2 is queued behind m1 in the same serialization chain - its own
    // conditions probe must not have run yet.
    expect(pc.getPassConditions).toHaveBeenCalledTimes(1);
    expect(oa.optimizeAudio).not.toHaveBeenCalled();

    releaseWait();
    await Promise.all([p1, p2]);
    expect(oa.optimizeAudio).toHaveBeenCalledTimes(2);
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
