// Orchestration of the post-download audio compatibility pass.
//
// A downloaded film whose audio the device can't render (AC-3/E-AC-3 on most
// phones) is rewritten ONCE, right after the download settles: the native
// pass (@evey/offline-audio: embedded ac3go+mkvgo plus the hardware AAC
// encoder) replaces the silent track with stereo AAC, then this module swaps
// the rewritten file over the original and patches the index so
// hasRenderableAudio starts answering true. The player needs no new logic:
// it already prefers the local copy exactly when the index says it is
// hearable.
//
// This module owns the policy; the native side owns the mechanics. Policy:
// - one pass at a time, downloads keep priority over passes (the pass is
//   minutes of CPU and a full re-write of the file's bytes);
// - a TERMINAL failure (deterministic: DTS file, unreadable container) is
//   remembered in the index so the startup sweep never re-walks gigabytes
//   toward the same refusal; transient failures retry on next launch;
// - only .mkv today - the Go side reads Matroska; an .mp4 with AC-3 keeps
//   the streaming fallback until the mp4 path lands.

import * as FileSystem from "expo-file-system/legacy";
import { deleteMediaFile, moveMediaFile } from "@evey/media-caps";
import {
  isOfflineAudioAvailable,
  optimizeAudio,
  OptimizeError,
  type OptimizeProgress,
} from "@evey/offline-audio";
import { getDeviceAudioCodecs } from "./deviceCodecs";
import { audioPassNotificationTitle } from "./downloadNotificationTexts";
import {
  getDownloadedMedia,
  needsAudioCompatPass,
  refreshDownloadedAudioCodecs,
  toFileUri,
  updateDownloadedAudio,
  getDownloads,
} from "./downloads";

export type AudioPassStage = OptimizeProgress["stage"];

export interface AudioPassState {
  stage: AudioPassStage;
  /** 0..1000 across the WHOLE pass (stages weighted, see combine below). */
  permille: number;
}

export type AudioPassResult = "done" | "skipped" | "failed";

// The decode+encode walk reads the source once; the remux reads it again and
// writes the output. Byte-wise the remux moves more data, but the first stage
// carries the CPU work - 80/20 tracks wall-clock on the measured pipeline.
function combinedPermille(p: OptimizeProgress): number {
  return p.stage === "transcode"
    ? Math.round(p.permille * 0.8)
    : 800 + Math.round(p.permille * 0.2);
}

// In-memory pass state for the Downloads screen, same shape of contract as
// downloads.ts's in-flight map: a poll-free subscribe plus a sync getter.
const passStates = new Map<string, AudioPassState>();
const passListeners = new Set<() => void>();

export function getAudioPassState(mediaItemId: string): AudioPassState | null {
  return passStates.get(mediaItemId) ?? null;
}

export function subscribeToAudioPass(cb: () => void): () => void {
  passListeners.add(cb);
  return () => passListeners.delete(cb);
}

function setPassState(mediaItemId: string, state: AudioPassState | null) {
  if (state) passStates.set(mediaItemId, state);
  else passStates.delete(mediaItemId);
  for (const cb of passListeners) cb();
}

// Failure codes that are properties of the FILE, not of the moment. Anything
// else (crash, cancel, "busy") may succeed on retry and is left retryable.
const TERMINAL_CODES = new Set([
  "not-matroska",
  "no-audio",
  "unsupported-codec",
  "no-convertible-track",
  "no-decodable-audio",
]);

// Serialize passes here in JS: the native module refuses concurrency ("busy")
// but refusing loses the work - a second finished download must WAIT for the
// running pass, not skip its own forever.
let chain: Promise<unknown> = Promise.resolve();

/**
 * Run the pass for one downloaded item if it needs one. Resolves "skipped"
 * when there is nothing to do (audio already renderable, not an mkv, module
 * absent, terminal failure recorded), "done" on success, "failed" otherwise.
 * Never rejects: the caller is a fire-and-forget download handler.
 */
export function runAudioCompatPass(mediaItemId: string): Promise<AudioPassResult> {
  const next = chain.then(() => runPassNow(mediaItemId)).catch((err) => {
    console.warn(`evey-audiopass ${mediaItemId} unexpected`, err);
    return "failed" as const;
  });
  chain = next;
  return next;
}

// A skip must say WHY in logcat: "no optimization line and no sound" on a
// device is undiagnosable when every skip path is silent (learned 08-01).
function skip(mediaItemId: string, reason: string): "skipped" {
  console.log(`evey-audiopass skip ${mediaItemId}: ${reason}`);
  return "skipped";
}

async function runPassNow(mediaItemId: string): Promise<AudioPassResult> {
  let entry = await getDownloadedMedia(mediaItemId);
  if (!entry) return skip(mediaItemId, "not in index");
  if (entry.audioPassFailed) return skip(mediaItemId, `terminal failure recorded: ${entry.audioPassFailed}`);
  if ((entry.audioCodecs ?? []).length === 0) {
    // No recorded codecs is a metadata gap, not a verdict - reading it as
    // "renderable" is how a silent episode stayed silent. One server lookup
    // settles it; unreachable leaves the entry for the next sweep.
    const codecs = await refreshDownloadedAudioCodecs(mediaItemId);
    if (!codecs) return skip(mediaItemId, "codecs unknown and the server has no answer");
    entry = { ...entry, audioCodecs: codecs };
  }
  const device = getDeviceAudioCodecs();
  if (!needsAudioCompatPass(entry, device)) {
    return skip(mediaItemId, `audio renderable (file=${(entry.audioCodecs ?? []).join(",") || "unknown"} device=${device.join(",")})`);
  }
  if (!(await isOfflineAudioAvailable())) return skip(mediaItemId, "native module absent");

  // The pass runs as soon as the download lands, on whatever power and network
  // the phone happens to be on. It was briefly gated on charging + Wi-Fi; both
  // were dropped on purpose. Wi-Fi has nothing to do with it - the pass reads a
  // file already on disk and never opens a socket - and waiting for a charger
  // only produced a download that finished with nothing visibly happening
  // afterwards. Running it right away is what the user actually asked for.
  //
  // NO extension gate here: a movie site that forgot its formatHint used to
  // save Matroska bytes under a ".mp4" name, and gating on the name skipped
  // the exact files this pass exists for. The Go side sniffs the container -
  // a real MP4 fails Open() in milliseconds with the terminal "not-matroska".
  //
  // Same directory as the source on purpose: the final rename must stay on
  // one volume to be atomic, and the space for it was reserved BEFORE the
  // download started (requiredDownloadBytes's 2x).
  const stem = entry.filePath.replace(/\.[^./\\]+$/, "");
  const outPath = `${stem}.opt.mkv`;
  console.log(`evey-audiopass start ${mediaItemId} ${entry.filePath}`);
  setPassState(mediaItemId, { stage: "transcode", permille: 0 });
  try {
    const summary = await optimizeAudio(
      entry.filePath,
      outPath,
      (p) => {
        setPassState(mediaItemId, { stage: p.stage, permille: combinedPermille(p) });
      },
      entry.title,
      audioPassNotificationTitle(),
    );

    // Swap: the original disappears for the instant between the two calls -
    // acceptable, the alternative (backup rename) doubles the peak disk use
    // the space guard promised NOT to exceed. The rewritten file IS Matroska,
    // so it also takes the honest ".mkv" name a misnamed source lacked.
    //
    // Two mechanisms per operation: expo's legacy FS handles the app-private
    // paths but REJECTS anything under the public Downloads collection (even
    // our own files - it cost one finished 1.7 GB rewrite in the field), and
    // the plain-File native helpers cover exactly that gap.
    const finalPath = `${stem}.mkv`;
    if (!(await deleteMediaFile(entry.filePath))) {
      await FileSystem.deleteAsync(toFileUri(entry.filePath), { idempotent: true });
    }
    if (!(await moveMediaFile(outPath, finalPath))) {
      await FileSystem.moveAsync({ from: toFileUri(outPath), to: toFileUri(finalPath) });
    }

    let fileSize = entry.fileSize;
    try {
      const info = await FileSystem.getInfoAsync(toFileUri(finalPath));
      if (info.exists && info.size) fileSize = info.size;
    } catch {
      /* size patch is cosmetic - the swap already happened */
    }
    // Only the converted track changed codec; the others (a second AC-3
    // language, say) are still in the file untouched.
    const codecs = [...(entry.audioCodecs ?? [])];
    const i = codecs.findIndex((c) => c.trim().toLowerCase() === summary.sourceCodec);
    if (i >= 0) codecs[i] = "aac";
    else codecs.push("aac");
    await updateDownloadedAudio(mediaItemId, { fileSize, audioCodecs: codecs, filePath: finalPath });
    console.log(
      `evey-audiopass done ${mediaItemId} ${summary.sourceCodec}(${summary.sourceChannels}ch)->aac ` +
        `frames=${summary.aacFrames} skipped=${summary.skippedFrames}`,
    );
    return "done";
  } catch (err) {
    // The native side already deletes its partial output on failure; this is
    // the belt for a failure in the JS half of the swap.
    try {
      if (!(await deleteMediaFile(outPath))) {
        await FileSystem.deleteAsync(toFileUri(outPath), { idempotent: true });
      }
    } catch {
      /* nothing left to clean */
    }
    const code = err instanceof OptimizeError ? err.code : "failed";
    console.warn(`evey-audiopass failed ${mediaItemId} ${code}: ${err instanceof Error ? err.message : err}`);
    if (TERMINAL_CODES.has(code)) {
      await updateDownloadedAudio(mediaItemId, { audioPassFailed: code });
    }
    return "failed";
  } finally {
    setPassState(mediaItemId, null);
  }
}

/**
 * Startup sweep: finish what a kill or crash interrupted. Serialized through
 * the same chain as the post-download triggers, and cheap when there is
 * nothing to do - every skip path above answers without touching the file.
 */
export async function resumePendingAudioPasses(): Promise<void> {
  let entries;
  try {
    entries = await getDownloads();
  } catch {
    return;
  }
  for (const entry of entries) {
    await runAudioCompatPass(entry.mediaItemId);
  }
}
