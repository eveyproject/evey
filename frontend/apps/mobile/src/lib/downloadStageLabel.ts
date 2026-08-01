// What a download-in-progress says about itself, in one place.
//
// A download is no longer "bytes arriving": it can be waiting on the server to
// transcode audio, waiting on the server to encode a 720p copy, collecting
// that copy part by part, or assembling the parts on the device. Two screens
// show this (the Downloads list and the media page's button) and they were
// already drifting apart on the compat case - the media page had the "a static
// label reads as a hang" comment, the list had the same logic without it.
//
// So the condition lives here once. The lesson is the project's own: when the
// same decision is spelled out twice it stops being the same decision.

import { isPreparingCompat, getCompatPreparePermille, type ProgressBytes } from "./downloads";
// From the state module, NOT reducedDownload: importing the flow would drag
// expo-file-system, the native downloader and the embedded Go module into
// every screen that only wants a label.
import { isReducedDownloadRunning, getReducedPreparePermille, getReducedSliceProgress } from "./reducedDownloadState";

type T = (key: string, defaultValue: string, options?: Record<string, unknown>) => string;

export interface StageLabelDeps {
  /** Byte-size formatter the calling screen already owns (they differ). */
  formatSize: (bytes: number) => string;
}

/**
 * The line under an in-flight download. progress is 0..1 (NaN = "running, no
 * figure yet"), bytes/rate come from the transfer, and everything else is read
 * from the modules that own each stage.
 */
export function downloadStageLabel(
  t: T,
  mediaItemId: string,
  progress: number | null,
  bytes: ProgressBytes | null,
  rate: number,
  deps: StageLabelDeps,
): string {
  // Server-side audio preparation: minutes behind a static "..." reads as a
  // hang, so the percent is not decoration - it is the thing that says "it is
  // working". (Reported in the field within a day of shipping the compat path.)
  if (isPreparingCompat(mediaItemId)) {
    return withPercent(t("downloads.preparingCompat", "Preparing audio on the server..."), getCompatPreparePermille(mediaItemId));
  }

  if (isReducedDownloadRunning(mediaItemId)) {
    const { collected, slices } = getReducedSliceProgress(mediaItemId);
    // Nothing collected yet: the server is still encoding, which is the long
    // wait (roughly realtime on a weak NAS - an hour for a feature film).
    if (collected === 0) {
      return withPercent(
        // 720 is the only reduced quality on offer (owner decision:
        // "Original + 720p, rien d'autre"), so it is a constant, not a knob.
        t("downloads.reduced.preparing", "Preparing a {{height}}p copy on the server...", { height: 720 }),
        getReducedPreparePermille(mediaItemId),
      );
    }
    // Collected everything: the last tenth of the bar is the device-side join.
    if (slices > 0 && collected >= slices) {
      // The join owns the last tenth of the bar (0.9..1.0), shown as its own
      // 0..100%: "97%" for ten minutes of copying reads as stuck, "40%" does
      // not.
      return t("downloads.reduced.assembling", "Assembling the film... {{percent}}%", {
        percent: Math.round(Math.max(0, Math.min(0.1, (progress ?? 0.9) - 0.9)) * 1000),
      });
    }
    const part = t("downloads.reduced.collecting", "Part {{done}} of {{total}}", { done: collected + 1, total: slices });
    const transfer = transferLabel(t, progress, bytes, rate, deps);
    return transfer ? `${part} · ${transfer}` : part;
  }

  return transferLabel(t, progress, bytes, rate, deps) || t("downloads.downloading", "Downloading...");
}

function withPercent(label: string, permille: number): string {
  return permille > 0 ? `${label} ${Math.round(permille / 10)}%` : label;
}

function transferLabel(
  t: T,
  progress: number | null,
  bytes: ProgressBytes | null,
  rate: number,
  deps: StageLabelDeps,
): string {
  const parts: string[] = [];
  if (progress != null && Number.isFinite(progress)) {
    parts.push(t("downloads.inProgress", "{{percent}}%", { percent: Math.round(progress * 100) }));
  }
  if (bytes && bytes.downloaded > 0) {
    parts.push(bytes.total > 0 ? `${deps.formatSize(bytes.downloaded)} / ${deps.formatSize(bytes.total)}` : deps.formatSize(bytes.downloaded));
  }
  if (rate > 0) parts.push(`${deps.formatSize(rate)}/s`);
  return parts.join(" · ");
}
