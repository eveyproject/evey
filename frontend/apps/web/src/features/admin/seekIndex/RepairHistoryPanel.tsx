import { useTranslation } from "react-i18next";
import {
  useAdminMediaServiceGetMediaRepairHistory,
  useAdminMediaServiceUndoMediaRepair,
} from "@evey/api";
import type { V1MediaRepairEntry } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";

// What was done to this file's bytes, and the way back.
//
// Evey keeps no copy of a repaired file - duplicating multi-gigabyte media for
// every repair costs a library its disk, and the copies that were kept during
// the 2026-07-13 batch got purged by the rolling window anyway. The bargain is
// this journal instead: each entry records the transformation precisely enough
// to be replayed backwards, so "undo" reads the entry rather than a backup.
//
// The panel shows the evidence alongside the action on purpose. An entry that
// says "declared-defect" acted on what the file claimed about itself; one that
// says a measured lag and a confidence acted on proof. That distinction is the
// whole difference between a repair and the incident.

function opLabel(op: string | undefined, t: (k: string) => string): string {
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

function outcomeVariant(outcome: string | undefined): "success" | "warning" | "error" | "default" {
  switch (outcome) {
    case "applied":
      return "success";
    case "skipped":
      return "default";
    case "rolled_back":
      return "warning";
    case "failed":
      return "error";
    default:
      return "default";
  }
}

// A shift is the one number that matters to a viewer: it says how far the sound
// moved, and its opposite is the undo.
function shiftSummary(entry: V1MediaRepairEntry): string {
  const shift = entry.shiftSec ?? {};
  const parts = Object.entries(shift).map(([track, raw]) => {
    const sec = Number(raw);
    return `${track}: ${sec > 0 ? "+" : ""}${sec.toFixed(3)}s`;
  });
  return parts.join(", ");
}

export function RepairHistoryPanel({ mediaId, title, onClose }: { mediaId: string; title: string; onClose: () => void }) {
  const { t } = useTranslation();
  const history = useAdminMediaServiceGetMediaRepairHistory(mediaId, { limit: 20 });
  const undo = useAdminMediaServiceUndoMediaRepair({
    mutation: { onSuccess: () => void history.refetch() },
  });
  const entries = history.data?.entries ?? [];
  const undoable = entries.find((e) => e.reversible);

  return (
    <div className="mt-2 rounded border border-[var(--border)] bg-[var(--surface-2)] p-3" data-testid="repair-history-panel">
      <div className="flex items-center justify-between gap-2">
        <h4 className="text-sm font-semibold text-[var(--text)]">{t("admin.repairHistory.title", { title })}</h4>
        <Button variant="ghost" className="text-xs" onClick={onClose} data-testid="repair-history-close">
          {t("common.close")}
        </Button>
      </div>

      <p className="mt-1 text-xs text-[var(--text-muted)]">{t("admin.repairHistory.intro")}</p>

      {history.isLoading && <p className="mt-2 text-xs text-[var(--text-muted)]">{t("common.loading")}</p>}

      {!history.isLoading && entries.length === 0 && (
        <p className="mt-2 text-xs text-[var(--text-muted)]" data-testid="repair-history-empty">
          {t("admin.repairHistory.empty")}
        </p>
      )}

      {entries.length > 0 && (
        <ul className="mt-2 space-y-2">
          {entries.map((e, i) => (
            <li key={i} className="text-xs text-[var(--text-muted)]" data-testid="repair-history-entry">
              <div className="flex flex-wrap items-center gap-1.5">
                <span className="text-[var(--text)]">{new Date(Number(e.atMs ?? 0)).toLocaleString()}</span>
                <Badge variant="info">{opLabel(e.op, t)}</Badge>
                <Badge variant={outcomeVariant(e.outcome)}>{t(`admin.repairHistory.outcome.${e.outcome}`)}</Badge>
                {e.reversible && <Badge variant="default">{t("admin.repairHistory.reversible")}</Badge>}
              </div>
              {shiftSummary(e) && (
                <div data-testid="repair-history-shift">{t("admin.repairHistory.shift", { shift: shiftSummary(e) })}</div>
              )}
              {/* The evidence, said plainly: acting on a measurement and acting
                  on the file's own claim are not the same thing. */}
              <div>
                {e.evidenceMethod === "track-coherence"
                  ? t("admin.repairHistory.evidenceMeasured", {
                      lag: (e.evidenceLagSec ?? 0).toFixed(3),
                      confidence: (e.evidenceConfidence ?? 0).toFixed(2),
                    })
                  : t("admin.repairHistory.evidenceDeclared")}
              </div>
              {e.error && <div className="text-[var(--error)]">{e.error}</div>}
            </li>
          ))}
        </ul>
      )}

      {undoable && (
        <div className="mt-3 border-t border-[var(--border)] pt-2">
          <Button
            variant="secondary"
            className="text-xs"
            loading={undo.isPending}
            onClick={() => undo.mutate({ mediaId, data: {} })}
            data-testid="repair-history-undo-btn"
          >
            {t("admin.repairHistory.undoBtn")}
          </Button>
          <p className="mt-1 text-xs text-[var(--text-muted)]">{t("admin.repairHistory.undoHint")}</p>
        </div>
      )}

      {entries.length > 0 && !undoable && (
        <p className="mt-2 text-xs text-[var(--text-muted)]" data-testid="repair-history-not-reversible">
          {t("admin.repairHistory.notReversible")}
        </p>
      )}
    </div>
  );
}
