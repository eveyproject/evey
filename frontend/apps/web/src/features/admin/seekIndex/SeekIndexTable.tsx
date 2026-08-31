import { useTranslation } from "react-i18next";
import type { V1SeekIndexItem } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { Pagination } from "@/components/ui/Pagination";
import { audioDelayed, formatBytes, isAckable, isAcked, verdictOf } from "@evey/api";
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
  | "remuxAudioOne"
  | "repairOne"
  | "rollbackOne"
  | "deleteBackup"
  | "fixContainerOne"
  | "ackOne"
>;

const VERDICT_KEYS = {
  healthy: "admin.seekIndex.verdictHealthy",
  repairable: "admin.seekIndex.verdictRepairable",
  manual: "admin.seekIndex.verdictManual",
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
  remuxAudioOne,
  repairOne,
  rollbackOne,
  deleteBackup,
  fixContainerOne,
  ackOne,
}: SeekIndexTableProps) {
  const { t } = useTranslation();

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
            const damagedOnly = it.hasIndex && Number(it.damagedBytes ?? 0) > 0 && !it.truncated;
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
                      verdict === "healthy" ? "success" : verdict === "repairable" ? "warning" : verdict === "manual" ? "error" : "info"
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
                  {!it.hasIndex && !it.seekCoarse && !it.truncated && !it.wrongContainer && (
                    <Badge variant="warning" className="ml-1.5">{t("admin.seekIndex.statusMissing")}</Badge>
                  )}
                  {it.seekCoarse && !it.truncated && (
                    <span title={t("admin.seekIndex.seekCoarseHint")} className="ml-1.5 inline-block">
                      <Badge variant="info">{t("admin.seekIndex.seekCoarseBadge")}</Badge>
                    </span>
                  )}
                  {it.hasBackup && (
                    <Badge variant="info" className="ml-1.5">
                      {t("admin.seekIndex.backupBadge")}
                    </Badge>
                  )}
                  {audioDelayed(it) && (
                    <Badge variant="warning" className="ml-1.5">
                      {t("admin.seekIndex.audioDelayBadge", {
                        sec: (it.audioDelaySec ?? 0).toFixed(1),
                      })}
                    </Badge>
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
                    {audioDelayed(it) && (
                      <Button
                        variant="secondary"
                        className="text-xs"
                        disabled={busy}
                        loading={remuxAudioOne.isPending && remuxAudioOne.variables?.mediaId === it.mediaId}
                        onClick={() => it.mediaId && remuxAudioOne.mutate({ mediaId: it.mediaId })}
                        data-testid="seek-index-remux-audio-btn"
                      >
                        {t("admin.seekIndex.remuxAudioBtn")}
                      </Button>
                    )}
                    {/* No repair exists for a truncated file - the tail is
                        simply absent - nor for a coarse one, whose index was
                        already rebuilt and cannot beat the encode's keyframes.
                        The badge's tooltip says why; the operator acknowledges
                        instead. */}
                    {indexRepairable && !it.wrongContainer && (
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
                    {(indexRepairable || damagedOnly) && !it.wrongContainer && (
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
