import type { V1MediaRepairEntry } from "@evey/api";

// How a media repair reads in the admin's journal and per-title history, on the
// web and the phone. Four copies until 2026-09-24, and the two history panels
// had missed a case: an operator's repair read as the file's own claim.

type T = (k: string, o?: Record<string, unknown>) => string;

export function repairOpLabel(op: string | undefined, t: T): string {
  switch (op) {
    case "reindex":
      return t("admin.repairHistory.opReindex");
    case "resync_damaged":
      return t("admin.repairHistory.opResync");
    case "container_fix":
      return t("admin.repairHistory.opContainerFix");
    case "audio_retime":
      return t("admin.repairHistory.opAudioRetime");
    case "rollback":
      return t("admin.repairHistory.opRollback");
    default:
      return op ?? "";
  }
}

// The evidence, said plainly: acting on a measurement, on an operator's call
// and on the file's own claim are not the same thing.
export function repairEvidenceLine(e: V1MediaRepairEntry, t: T): string {
  switch (e.evidenceMethod) {
    case "track-coherence":
      return t("admin.repairHistory.evidenceMeasured", {
        lag: (e.evidenceLagSec ?? 0).toFixed(3),
        confidence: (e.evidenceConfidence ?? 0).toFixed(2),
      });
    case "operator":
    case "operator-undo":
      return t("admin.repairJournal.evidenceOperator");
    default:
      return t("admin.repairHistory.evidenceDeclared");
  }
}

// Per-track shift, "audio: +0.120s, video: -0.040s".
export function repairShiftSummary(entry: V1MediaRepairEntry): string {
  return Object.entries(entry.shiftSec ?? {})
    .map(([track, raw]) => {
      const sec = Number(raw);
      return `${track}: ${sec > 0 ? "+" : ""}${sec.toFixed(3)}s`;
    })
    .join(", ");
}
