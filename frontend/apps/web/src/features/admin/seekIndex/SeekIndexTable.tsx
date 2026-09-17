import { useTranslation } from "react-i18next";
import type { V1SeekIndexItem } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { useState } from "react";
import { Button } from "@/components/ui/Button";
import { RepairHistoryPanel } from "./RepairHistoryPanel";
import { SyncPreviewPanel } from "./SyncPreviewPanel";
import { EmptyState } from "@/components/ui/EmptyState";
import { Pagination } from "@/components/ui/Pagination";
import { audioDelayed, audioPictureOff, audioTrackZeroed, baseName, brokenInterleave, formatBytes, isAckable, isAcked, needsResync, verdictOf, audioTracksDisagree } from "@evey/api";
import { SEEK_INDEX_PAGE_SIZE, type SeekIndexPageState } from "./hooks";

type SeekIndexTableProps = Pick<
  SeekIndexPageState,
  | "items"
  | "visible"
  | "paged"
  | "page"
  | "setPage"
  | "totalPages"
  | "busy"
  | "containerFixAvailable"
  | "reinterleaveAvailable"
  | "repairOne"
  | "rollbackOne"
  | "deleteBackup"
  | "fixContainerOne"
  | "reinterleaveOne"
  | "ackOne"
>;

const VERDICT_KEYS = {
  healthy: "admin.seekIndex.verdictHealthy",
  repairable: "admin.seekIndex.verdictRepairable",
  informational: "admin.seekIndex.verdictInformational",
} as const;

export function SeekIndexTable({
  items,
  visible,
  paged,
  page,
  setPage,
  totalPages,
  busy,
  containerFixAvailable,
  reinterleaveAvailable,
  repairOne,
  rollbackOne,
  deleteBackup,
  fixContainerOne,
  reinterleaveOne,
  ackOne,
}: SeekIndexTableProps) {
  const { t } = useTranslation();
  // Which row has its journal open. One at a time: the history is a detour, not
  // a column, and stacking several would bury the table it belongs to.
  const [historyFor, setHistoryFor] = useState<string | null>(null);
  // The preview is the only thing that answers "does the sound follow the
  // picture". One row at a time, like the journal.
  const [previewFor, setPreviewFor] = useState<string | null>(null);

  return items.length === 0 ? (
    <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
      <EmptyState title={t("admin.seekIndex.noEbml")} />
    </div>
  ) : visible.length === 0 ? (
    <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
      <EmptyState title={t("admin.seekIndex.noneInView")} />
    </div>
  ) : (
    <div className="flex flex-col gap-3">
    <div className="overflow-x-auto rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
      <table className="w-full text-sm table-cards">
        <thead>
          <tr className="border-b border-[var(--border)]">
            <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
              {t("admin.seekIndex.tableHeaders.file")}
            </th>
            <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
              {t("admin.seekIndex.tableHeaders.container")}
            </th>
            <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
              {t("admin.seekIndex.tableHeaders.size")}
            </th>
            <th className="text-left px-4 py-3 font-medium text-[var(--text-muted)]">
              {t("admin.seekIndex.tableHeaders.status")}
            </th>
            <th className="px-4 py-3 w-44" />
          </tr>
        </thead>
        <tbody>
          {paged.map((it: V1SeekIndexItem) => {
            const verdict = verdictOf(it);
            const acked = isAcked(it);
            const indexRepairable = !it.hasIndex && !it.truncated && !it.seekCoarse;
            return (
              <tr key={it.mediaId} className="border-b border-[var(--border)]" data-testid="seek-index-row">
                <td className="px-4 py-3" data-label={t("admin.seekIndex.tableHeaders.file")}>
                  <div className="text-[var(--text)]">{it.title || it.path}</div>
                  <div
                    className="font-mono text-xs text-[var(--text-muted)] truncate max-w-[280px] sm:max-w-[480px]"
                    title={it.path}
                  >
                    {it.path}
                  </div>
                </td>
                <td className="px-4 py-3" data-label={t("admin.seekIndex.tableHeaders.container")}>
                  <Badge>{(it.container ?? "").toUpperCase()}</Badge>
                </td>
                <td className="px-4 py-3 whitespace-nowrap text-[var(--text-muted)]" data-label={t("admin.seekIndex.tableHeaders.size")}>
                  {formatBytes(Number(it.sizeBytes ?? 0))}
                </td>
                <td className="px-4 py-3 whitespace-nowrap" data-label={t("admin.seekIndex.tableHeaders.status")}>
                  {/* The verdict first - the one answer - then the defects
                      that make it up, each with its own explanation. */}
                  <Badge
                    variant={
                      verdict === "healthy" ? "success" : verdict === "repairable" ? "warning" : "info"
                    }
                    data-testid="seek-index-verdict"
                  >
                    {t(VERDICT_KEYS[verdict])}
                  </Badge>
                  {acked && (
                    <Badge variant="default" className="ml-1.5" data-testid="seek-index-acked-badge">
                      {t("admin.seekIndex.ackedBadge")}
                    </Badge>
                  )}
                  {/* Nothing here could read the file: the error itself is the
                      finding, in the prober's words on hover. */}
                  {it.unreadable && (
                    <span
                      title={t("admin.seekIndex.unreadableHint", { detail: it.unreadableDetail || "" })}
                      className="ml-1.5 inline-block"
                      data-testid="seek-index-unreadable-hint"
                    >
                      <Badge variant="error">{t("admin.seekIndex.unreadableBadge")}</Badge>
                    </span>
                  )}
                  {!it.hasIndex && !it.seekCoarse && !it.truncated && !it.wrongContainer && (
                    <Badge variant="warning" className="ml-1.5">{t("admin.seekIndex.statusMissing")}</Badge>
                  )}
                  {it.seekCoarse && !it.truncated && (
                    <span
                      title={t("admin.seekIndex.seekCoarseHint", { detail: it.seekCoarseDetail || t("admin.seekIndex.seekCoarseNoDetail") })}
                      className="ml-1.5 inline-block"
                      data-testid="seek-index-coarse-hint"
                    >
                      <Badge variant="info">{t("admin.seekIndex.seekCoarseBadge")}</Badge>
                    </span>
                  )}
                  {it.hasBackup && (
                    <Badge variant="info" className="ml-1.5">
                      {t("admin.seekIndex.backupBadge")}
                    </Badge>
                  )}
                  {/* An offset is a FACT, shown in the file's own terms - never
                      an alarm. It only turns into something to look at when the
                      file's own audio tracks are measured to disagree. */}
                  {audioTracksDisagree(it) ? (
                    <span
                      title={t("admin.seekIndex.audioTracksDisagreeHint", {
                        sec: Math.abs(Number(it.audioTracksLagSec ?? 0)).toFixed(1),
                      })}
                      className="ml-1.5 inline-block"
                    >
                      <Badge variant="warning">
                        {t("admin.seekIndex.audioTracksDisagreeBadge", {
                          sec: Math.abs(Number(it.audioTracksLagSec ?? 0)).toFixed(1),
                        })}
                      </Badge>
                    </span>
                  ) : (
                    audioDelayed(it) && (
                      <span
                        title={
                          it.audioTracksChecked
                            ? t("admin.seekIndex.audioOffsetProvenHint")
                            : t("admin.seekIndex.audioOffsetHint")
                        }
                        className="ml-1.5 inline-block"
                      >
                        <Badge variant="info">
                          {t("admin.seekIndex.audioOffsetBadge", {
                            sec: (it.audioDelaySec ?? 0).toFixed(1),
                          })}
                        </Badge>
                      </span>
                    )
                  )}
                  {/* The mark an earlier repair left: one track pulled to 0
                      while its sibling kept its priming. Names the file; the
                      amount removed is not in it any more. */}
                  {/* The sound measured against the PICTURE (its subtitles),
                      not against a sibling track: the one measure that sees
                      every track pulled early together. */}
                  {audioPictureOff(it) && (
                    <span
                      title={t("admin.seekIndex.audioPictureOffHint", {
                        sec: Number(it.audioPictureLagSec ?? 0).toFixed(2),
                        track: (it.audioPictureTrack ?? 0) + 1,
                      })}
                      className="ml-1.5 inline-block"
                      data-testid="seek-index-audio-picture-hint"
                    >
                      <Badge variant="warning">
                        {t("admin.seekIndex.audioPictureOffBadge", { sec: Number(it.audioPictureLagSec ?? 0).toFixed(2) })}
                      </Badge>
                    </span>
                  )}
                  {audioTrackZeroed(it) && (
                    <span
                      title={t("admin.seekIndex.audioTrackZeroedHint")}
                      className="ml-1.5 inline-block"
                      data-testid="seek-index-track-zeroed-hint"
                    >
                      <Badge variant="warning">{t("admin.seekIndex.audioTrackZeroedBadge")}</Badge>
                    </span>
                  )}
                  {it.truncated ? (
                    <span title={t("admin.seekIndex.truncatedHint")} className="ml-1.5 inline-block">
                      <Badge variant="error">
                        {Number(it.missingTailBytes ?? 0) > 0
                          ? t("admin.seekIndex.truncatedSizeBadge", {
                              size: formatBytes(Number(it.missingTailBytes ?? 0)),
                            })
                          : t("admin.seekIndex.truncatedBadge")}
                      </Badge>
                    </span>
                  ) : (
                    Number(it.damagedBytes ?? 0) > 0 && (
                      <span title={t("admin.seekIndex.repairResyncHint")} className="ml-1.5 inline-block">
                        <Badge variant="warning">
                          {t("admin.seekIndex.damagedBadge", {
                            size: formatBytes(Number(it.damagedBytes ?? 0)),
                          })}
                        </Badge>
                      </span>
                    )
                  )}
                  {it.wrongContainer && (
                    <span title={t("admin.seekIndex.wrongContainerHint")} className="ml-1.5 inline-block">
                      <Badge variant="error">{t("admin.seekIndex.wrongContainerBadge")}</Badge>
                    </span>
                  )}
                  {brokenInterleave(it) && (
                    <span
                      title={t("admin.seekIndex.brokenInterleaveHint", { sec: Math.round(Number(it.interleaveBackstepSec ?? 0)) })}
                      className="ml-1.5 inline-block"
                      data-testid="seek-index-broken-interleave-hint"
                    >
                      <Badge variant="error">{t("admin.seekIndex.brokenInterleaveBadge")}</Badge>
                    </span>
                  )}
                  {/* A repair already walked this file and the source refused
                      to parse. Said on the row, because the alternative is the
                      operator reading a raw parser message in the job card and
                      having no idea which button answers it. */}
                  {it.resyncRequired && (
                    <span
                      title={t("admin.seekIndex.resyncRequiredHint")}
                      className="ml-1.5 inline-block"
                      data-testid="seek-index-resync-required-hint"
                    >
                      <Badge variant="warning">{t("admin.seekIndex.resyncRequiredBadge")}</Badge>
                    </span>
                  )}
                  {it.trailingJunk && (
                    <span title={t("admin.seekIndex.trailingJunkHint")} className="ml-1.5 inline-block">
                      <Badge variant="info">{t("admin.seekIndex.trailingJunkBadge")}</Badge>
                    </span>
                  )}
                  {(it.audioShortfallSec ?? 0) > 0 && (
                    <span
                      title={t("admin.seekIndex.audioShortHint", {
                        sec: Math.round(it.audioShortfallSec ?? 0),
                      })}
                      className="ml-1.5 inline-block"
                    >
                      <Badge variant="info">
                        {t("admin.seekIndex.audioShortBadge", {
                          sec: Math.round(it.audioShortfallSec ?? 0),
                        })}
                      </Badge>
                    </span>
                  )}
                </td>
                <td className="px-4 py-3 whitespace-nowrap" data-label="">
                  <div className="flex items-center justify-end gap-2">
                    {/* No button for an audio start offset, on purpose: it is a
                        fact about the file, and the "fix" that used to live here
                        pulled tracks back by their own declared delay - which is
                        how 98 deliberate dub alignments were erased in July. */}
                    {/* No repair exists for a truncated file - the tail is
                        simply absent - nor for a coarse one, whose index was
                        already rebuilt and cannot beat the encode's keyframes.
                        The badge's tooltip says why; the operator acknowledges
                        instead. */}
                    {indexRepairable && !it.resyncRequired && !it.wrongContainer && (
                      <Button
                        variant="secondary"
                        className="text-xs"
                        disabled={busy}
                        loading={
                          repairOne.isPending &&
                          repairOne.variables?.mediaId === it.mediaId &&
                          !repairOne.variables?.data?.resync
                        }
                        title={t("admin.seekIndex.repairHint")}
                        onClick={() => it.mediaId && repairOne.mutate({ mediaId: it.mediaId, data: { mode: "auto" } })}
                        data-testid="seek-index-repair-btn"
                      >
                        {t("admin.seekIndex.repair")}
                      </Button>
                    )}
                    {/* Resync: the lossy rewrite, offered where it is the
                        answer - a damaged stretch (index fine or not) - and as
                        the fallback when the normal repair refused a file. */}
                    {(indexRepairable || needsResync(it)) && !it.wrongContainer && (
                      <Button
                        variant="ghost"
                        className="text-xs"
                        disabled={busy}
                        loading={
                          repairOne.isPending &&
                          repairOne.variables?.mediaId === it.mediaId &&
                          repairOne.variables?.data?.resync === true
                        }
                        title={t("admin.seekIndex.repairResyncHint")}
                        onClick={() => it.mediaId && repairOne.mutate({ mediaId: it.mediaId, data: { resync: true } })}
                        data-testid="seek-index-repair-resync-btn"
                      >
                        {t("admin.seekIndex.repairResync")}
                      </Button>
                    )}
                    {it.wrongContainer && containerFixAvailable && (
                      <Button
                        variant="secondary"
                        className="text-xs"
                        disabled={busy}
                        loading={fixContainerOne.isPending && fixContainerOne.variables?.mediaId === it.mediaId}
                        title={t("admin.seekIndex.fixContainerHint")}
                        onClick={() => it.mediaId && fixContainerOne.mutate({ mediaId: it.mediaId })}
                        data-testid="seek-index-fix-container-btn"
                      >
                        {t("admin.seekIndex.fixContainerBtn")}
                      </Button>
                    )}
                    {brokenInterleave(it) && reinterleaveAvailable && (
                      <Button
                        variant="secondary"
                        className="text-xs"
                        disabled={busy}
                        loading={reinterleaveOne.isPending && reinterleaveOne.variables?.mediaId === it.mediaId}
                        title={t("admin.seekIndex.reinterleaveHint")}
                        onClick={() => it.mediaId && reinterleaveOne.mutate({ mediaId: it.mediaId })}
                        data-testid="seek-index-reinterleave-btn"
                      >
                        {t("admin.seekIndex.reinterleaveBtn")}
                      </Button>
                    )}
                    {/* What was done to this file's bytes. Evey keeps no copy
                        of a repaired file, so this journal is both the history
                        and the way back. */}
                    <Button
                      variant="ghost"
                      className="text-xs"
                      onClick={() => setHistoryFor(historyFor === it.mediaId ? null : (it.mediaId ?? null))}
                      data-testid="seek-index-history-btn"
                    >
                      {t("admin.repairHistory.btn")}
                    </Button>
                    <Button
                      variant="ghost"
                      className="text-xs"
                      onClick={() => setPreviewFor(previewFor === it.mediaId ? null : (it.mediaId ?? null))}
                      data-testid="seek-index-sync-preview-btn"
                    >
                      {t("admin.syncPreview.btn")}
                    </Button>
                    {isAckable(it) && (
                      <Button
                        variant="ghost"
                        className="text-xs"
                        disabled={ackOne.isPending}
                        loading={ackOne.isPending && ackOne.variables?.data?.mediaId === it.mediaId}
                        onClick={() => it.mediaId && ackOne.mutate({ data: { mediaId: it.mediaId, acked: !acked } })}
                        data-testid={acked ? "seek-index-unack-btn" : "seek-index-ack-btn"}
                      >
                        {acked ? t("admin.seekIndex.unackBtn") : t("admin.seekIndex.ackBtn")}
                      </Button>
                    )}
                    {it.hasBackup && (
                      <Button
                        variant="ghost"
                        className="text-xs"
                        disabled={busy}
                        loading={rollbackOne.isPending && rollbackOne.variables?.mediaId === it.mediaId}
                        onClick={() => it.mediaId && rollbackOne.mutate({ mediaId: it.mediaId })}
                        data-testid="seek-index-rollback-btn"
                      >
                        {t("admin.seekIndex.rollback")}
                      </Button>
                    )}
                    {it.hasBackup && (
                      <Button
                        variant="ghost"
                        className="text-xs"
                        disabled={deleteBackup.isPending}
                        loading={deleteBackup.isPending && deleteBackup.variables?.data?.mediaId === it.mediaId}
                        onClick={() => it.mediaId && deleteBackup.mutate({ data: { mediaId: it.mediaId } })}
                        data-testid="seek-index-delete-backup-btn"
                      >
                        {t("admin.seekIndex.deleteBackup")}
                      </Button>
                    )}
                  </div>
                  {previewFor && previewFor === it.mediaId && (
                    <SyncPreviewPanel
                      item={it}
                      title={it.title || baseName(it.path ?? "")}
                      onClose={() => setPreviewFor(null)}
                    />
                  )}
                  {historyFor && historyFor === it.mediaId && (
                    <RepairHistoryPanel
                      mediaId={historyFor}
                      title={it.title || baseName(it.path ?? "")}
                      onClose={() => setHistoryFor(null)}
                    />
                  )}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
    <div className="flex flex-wrap items-center justify-between gap-2 px-1">
      <span className="text-xs text-[var(--text-muted)]" data-testid="seek-index-page-info">
        {t("admin.seekIndex.pageInfo", {
          from: (page - 1) * SEEK_INDEX_PAGE_SIZE + 1,
          to: Math.min(page * SEEK_INDEX_PAGE_SIZE, visible.length),
          total: visible.length,
        })}
      </span>
      {totalPages > 1 && <Pagination page={page} totalPages={totalPages} onPageChange={setPage} />}
    </div>
    </div>
  );
}
