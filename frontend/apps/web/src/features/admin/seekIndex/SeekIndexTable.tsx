import { useTranslation } from "react-i18next";
import type { V1SeekIndexItem } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { EmptyState } from "@/components/ui/EmptyState";
import { audioDelayed, formatBytes } from "@evey/api";
import type { SeekIndexPageState } from "./hooks";

type SeekIndexTableProps = Pick<
  SeekIndexPageState,
  "items" | "visible" | "busy" | "remuxAudioOne" | "repairOne" | "rollbackOne" | "deleteBackup"
>;

export function SeekIndexTable({
  items,
  visible,
  busy,
  remuxAudioOne,
  repairOne,
  rollbackOne,
  deleteBackup,
}: SeekIndexTableProps) {
  const { t } = useTranslation();

  return items.length === 0 ? (
    <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
      <EmptyState title={t("admin.seekIndex.noEbml")} />
    </div>
  ) : visible.length === 0 ? (
    <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
      <EmptyState title={t("admin.seekIndex.allGood")} />
    </div>
  ) : (
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
          {visible.map((it: V1SeekIndexItem) => (
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
                {it.hasIndex ? (
                  <Badge variant="success">{t("admin.seekIndex.statusIndexed")}</Badge>
                ) : it.seekCoarse ? (
                  // Not "missing": the index exists and was rebuilt, it
                  // just cannot be finer than the encode's keyframes.
                  // Showing the repairable badge here promised a fix
                  // that no repair can deliver.
                  <span title={t("admin.seekIndex.seekCoarseHint")} className="inline-block">
                    <Badge variant="info">{t("admin.seekIndex.seekCoarseBadge")}</Badge>
                  </span>
                ) : (
                  <Badge variant="warning">{t("admin.seekIndex.statusMissing")}</Badge>
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
                    <Badge variant="warning" className="ml-1.5">
                      {t("admin.seekIndex.damagedBadge", {
                        size: formatBytes(Number(it.damagedBytes ?? 0)),
                      })}
                    </Badge>
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
                    <Badge variant="warning">
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
                      simply absent, a real attempt always fails a verify
                      step - nor for a coarse one, whose index was already
                      rebuilt and cannot beat the encode's keyframes. No
                      button is offered (the badge's tooltip says why);
                      the batch's needsReindex skips both for the same
                      reason. */}
                  {!it.hasIndex && !it.truncated && !it.seekCoarse && (
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
                  {!it.hasIndex && !it.truncated && !it.seekCoarse && (
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
          ))}
        </tbody>
      </table>
    </div>
  );
}
