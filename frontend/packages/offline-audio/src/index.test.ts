import { describe, it, expect, vi, beforeEach } from "vitest";

// The Android module is stood in by a recorder; `present` flips between an
// app build (module linked) and iOS/tests/Expo Go (module absent).
const native = vi.hoisted(() => ({
  present: true,
  optimize: vi.fn(),
  joinSlices: vi.fn(),
  deleteSlices: vi.fn(),
  cancel: vi.fn(),
  isRunning: vi.fn(() => false),
  removed: 0,
  listeners: [] as Array<(p: unknown) => void>,
  addListener(_e: string, l: (p: unknown) => void) {
    native.listeners.push(l);
    return { remove: () => { native.removed++; } };
  },
}));
vi.mock("expo-modules-core", () => ({
  requireOptionalNativeModule: () => (native.present ? native : null),
}));

import {
  OptimizeError, optimizeAudio, cancelOptimize, sliceFileName, joinDownloadSlices, deleteJoinedSlices, isOfflineAudioAvailable,
} from "./index";

beforeEach(() => {
  native.present = true;
  native.removed = 0;
  native.listeners.length = 0;
  native.optimize.mockReset().mockResolvedValue({ sampleRate: 48000, sourceCodec: "eac3", sourceChannels: 6, aacFrames: 10, skippedFrames: 0 });
  native.joinSlices.mockReset().mockResolvedValue({ slices: 3, path: "/out.mkv" });
  native.deleteSlices.mockReset();
  native.cancel.mockReset();
});

// Without the module every entry point is either a no-op or an
// "unavailable" error the caller can route on; with it, a native failure
// keeps its machine-readable code (unknown ones collapse to "failed"), the
// progress subscription never outlives the call, and slice names match the
// server's muxer to the digit.
describe("offline-audio bridge", () => {
  it("reports availability and refuses work without the module", async () => {
    native.present = false;
    expect(await isOfflineAudioAvailable()).toBe(false);
    await expect(optimizeAudio("/in.mkv", "/out.mkv")).rejects.toMatchObject({ name: "OptimizeError", code: "unavailable" });
    await expect(joinDownloadSlices("/d", 3, "/out.mkv", 1000)).rejects.toMatchObject({ code: "unavailable" });
    await expect(cancelOptimize()).resolves.toBeUndefined();
    await expect(deleteJoinedSlices("/d", 3)).resolves.toBeUndefined();
    native.present = true;
    expect(await isOfflineAudioAvailable()).toBe(true);
  });

  it("runs the pass with a progress listener that is removed afterwards", async () => {
    const onProgress = vi.fn();
    const summary = await optimizeAudio("/in.mkv", "/out.mkv", onProgress, "Heat");
    expect(summary.sourceCodec).toBe("eac3");
    expect(native.optimize).toHaveBeenCalledWith("/in.mkv", "/out.mkv", "Heat");
    expect(native.listeners).toHaveLength(1);
    expect(native.removed).toBe(1);
    // No listener is registered when nobody asked for progress.
    await optimizeAudio("/in.mkv", "/out.mkv");
    expect(native.listeners).toHaveLength(1);
    expect(native.optimize).toHaveBeenLastCalledWith("/in.mkv", "/out.mkv", "");
  });

  it("keeps a known native code, collapses an unknown one to failed, and still removes the listener", async () => {
    const codes: string[] = [];
    for (const code of ["busy", "cancelled", "not-matroska", "unsupported-codec", "something-else", undefined]) {
      native.optimize.mockRejectedValueOnce(Object.assign(new Error(`native: ${code}`), { code }));
      try {
        await optimizeAudio("/in.mkv", "/out.mkv", () => {});
      } catch (e) {
        expect(e).toBeInstanceOf(OptimizeError);
        codes.push((e as OptimizeError).code);
        expect((e as Error).message).toBe(`native: ${code}`);
      }
    }
    expect(codes).toEqual(["busy", "cancelled", "not-matroska", "unsupported-codec", "failed", "failed"]);
    expect(native.removed).toBe(6);
  });

  it("joins slices with the server's true duration and names slices like the muxer", async () => {
    await joinDownloadSlices("/d", 3, "/out.mkv", 5_400_000, undefined, "Heat");
    expect(native.joinSlices).toHaveBeenCalledWith("/d", 3, "/out.mkv", 5_400_000, "Heat");
    native.joinSlices.mockRejectedValueOnce(Object.assign(new Error("hole"), { code: "slice-missing" }));
    await expect(joinDownloadSlices("/d", 3, "/out.mkv", 1)).rejects.toMatchObject({ code: "slice-missing" });
    await deleteJoinedSlices("/d", 3);
    expect(native.deleteSlices).toHaveBeenCalledWith("/d", 3);
    await cancelOptimize();
    expect(native.cancel).toHaveBeenCalledTimes(1);
    expect(sliceFileName(0)).toBe("slice-00000.mkv");
    expect(sliceFileName(42)).toBe("slice-00042.mkv");
    expect(sliceFileName(123456)).toBe("slice-123456.mkv");
  });
});
