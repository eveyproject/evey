import { ScanSearch } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { PageHeader } from "@/components/ui/PageHeader";
import { Card } from "@/components/ui/Card";
import { Checkbox } from "@/components/ui/Checkbox";
import { EmptyState } from "@/components/ui/EmptyState";
import { errText } from "@/features/admin/seekIndex/helpers";
import { useSeekIndexPage } from "@/features/admin/seekIndex/hooks";
import { JobProgressCard } from "@/features/admin/seekIndex/JobProgressCard";
import { LastJobCard } from "@/features/admin/seekIndex/LastJobCard";
import { SeekIndexControls } from "@/features/admin/seekIndex/SeekIndexControls";
import { StatsGrid } from "@/features/admin/seekIndex/StatsGrid";
import { SeekIndexTable } from "@/features/admin/seekIndex/SeekIndexTable";
import { RepairConfirmModal } from "@/features/admin/seekIndex/RepairConfirmModal";

export const Route = createLazyFileRoute("/_auth/admin/seek-index")({
  component: SeekIndexPage,
});

function SeekIndexPage() {
  const { t } = useTranslation();
  const {
    libraryId,
    setLibraryId,
    onlyAttention,
    setOnlyAttention,
    confirmBatch,
    setConfirmBatch,
    keepBackups,
    setKeepBackups,
    alsoAudio,
    setAlsoAudio,
    libraries,
    job,
    running,
    scan,
    result,
    items,
    missing,
    withBackup,
    delayedAudio,
    wrongContainer,
    trailingJunk,
    audioShort,
    seekCoarse,
    repairable,
    repairVolumes,
    backupsRisky,
    startScan,
    repairOne,
    rollbackOne,
    repairLib,
    rollbackLib,
    remuxAudioOne,
    deleteBackup,
    purgeLib,
    visible,
    busy,
  } = useSeekIndexPage();

  return (
    <div className="flex flex-col gap-6" data-testid="seek-index-page">
      <PageHeader title={t("admin.seekIndex.title")} />

      <Card>
        <div className="flex flex-col gap-2 text-sm text-[var(--text-muted)]">
          <p>{t("admin.seekIndex.description")}</p>
          <p data-testid="seek-index-ro-note">{t("admin.seekIndex.readOnlyNote")}</p>
        </div>
      </Card>

      <SeekIndexControls
        libraryId={libraryId}
        setLibraryId={setLibraryId}
        libraries={libraries}
        startScan={startScan}
        busy={busy}
        result={result}
        repairable={repairable}
        delayedAudio={delayedAudio}
        backupsRisky={backupsRisky}
        setKeepBackups={setKeepBackups}
        setAlsoAudio={setAlsoAudio}
        setConfirmBatch={setConfirmBatch}
        withBackup={withBackup}
      />

      {running && job && <JobProgressCard job={job} />}
      {!running && job?.last && <LastJobCard last={job.last} />}

      {scan.isError && (
        <p className="text-sm text-[var(--color-danger)]" role="alert">
          {errText(scan.error, t("admin.seekIndex.scanFailed"))}
        </p>
      )}

      {!result ? (
        <div className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]">
          <EmptyState
            icon={<ScanSearch size={48} strokeWidth={1.5} />}
            title={t("admin.seekIndex.emptyTitle")}
            description={t("admin.seekIndex.emptyDescription")}
          />
        </div>
      ) : (
        <>
          <StatsGrid
            result={result}
            missing={missing}
            withBackup={withBackup}
            delayedAudio={delayedAudio}
            wrongContainer={wrongContainer}
            trailingJunk={trailingJunk}
            audioShort={audioShort}
            seekCoarse={seekCoarse}
          />

          <div className="flex flex-wrap items-center justify-between gap-2">
            <Checkbox
              label={t("admin.seekIndex.onlyAttention")}
              checked={onlyAttention}
              onChange={setOnlyAttention}
            />
            <span className="text-xs text-[var(--text-muted)]" data-testid="seek-index-scanned-at">
              {t("admin.seekIndex.scannedAt", {
                date: new Date(Number(result.scannedAtMs)).toLocaleString(),
              })}
            </span>
          </div>

          <SeekIndexTable
            items={items}
            visible={visible}
            busy={busy}
            remuxAudioOne={remuxAudioOne}
            repairOne={repairOne}
            rollbackOne={rollbackOne}
            deleteBackup={deleteBackup}
          />
        </>
      )}

      <RepairConfirmModal
        confirmBatch={confirmBatch}
        setConfirmBatch={setConfirmBatch}
        withBackup={withBackup}
        repairable={repairable}
        repairVolumes={repairVolumes}
        delayedAudio={delayedAudio}
        alsoAudio={alsoAudio}
        setAlsoAudio={setAlsoAudio}
        keepBackups={keepBackups}
        setKeepBackups={setKeepBackups}
        repairLib={repairLib}
        rollbackLib={rollbackLib}
        purgeLib={purgeLib}
        libraryId={libraryId}
      />
    </div>
  );
}
