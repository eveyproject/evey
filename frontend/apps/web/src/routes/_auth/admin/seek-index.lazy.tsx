import { ScanSearch } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { PageHeader } from "@/components/ui/PageHeader";
import { Card } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";
import { errText } from "@evey/api";
import { useSeekIndexPage } from "@/features/admin/seekIndex/hooks";
import { JobProgressCard } from "@/features/admin/seekIndex/JobProgressCard";
import { LastJobCard } from "@/features/admin/seekIndex/LastJobCard";
import { SeekIndexControls } from "@/features/admin/seekIndex/SeekIndexControls";
import { StatsGrid } from "@/features/admin/seekIndex/StatsGrid";
import { SeekIndexTable } from "@/features/admin/seekIndex/SeekIndexTable";
import { SeekIndexFilters } from "@/features/admin/seekIndex/SeekIndexFilters";
import { RepairConfirmModal } from "@/features/admin/seekIndex/RepairConfirmModal";

export const Route = createLazyFileRoute("/_auth/admin/seek-index")({
  component: SeekIndexPage,
});

function SeekIndexPage() {
  const { t } = useTranslation();
  const {
    libraryId,
    setLibraryId,
    view,
    setView,
    query,
    setQuery,
    sort,
    setSort,
    page,
    setPage,
    totalPages,
    counts,
    confirmBatch,
    setConfirmBatch,
    keepBackups,
    setKeepBackups,
    resyncDamaged,
    setResyncDamaged,
    libraries,
    job,
    running,
    scan,
    result,
    items,
    withBackup,
    containerFixAvailable,
    triage,
    breakdown,
    repairable,
    repairVolumes,
    backupsRisky,
    startScan,
    repairOne,
    rollbackOne,
    repairLib,
    rollbackLib,
    remuxAudioOne,
    fixContainerOne,
    deleteBackup,
    purgeLib,
    ackOne,
    ackGroup,
    visible,
    paged,
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
        backupsRisky={backupsRisky}
        setKeepBackups={setKeepBackups}
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
            triage={triage}
            breakdown={breakdown}
            withBackup={withBackup}
            ackGroup={ackGroup}
            libraryId={libraryId}
            busy={busy}
          />

          <SeekIndexFilters
            view={view}
            setView={setView}
            query={query}
            setQuery={setQuery}
            sort={sort}
            setSort={setSort}
            counts={counts}
            result={result}
          />

          <SeekIndexTable
            items={items}
            visible={visible}
            paged={paged}
            page={page}
            setPage={setPage}
            totalPages={totalPages}
            busy={busy}
            containerFixAvailable={containerFixAvailable}
            remuxAudioOne={remuxAudioOne}
            repairOne={repairOne}
            rollbackOne={rollbackOne}
            deleteBackup={deleteBackup}
            fixContainerOne={fixContainerOne}
            ackOne={ackOne}
          />
        </>
      )}

      <RepairConfirmModal
        confirmBatch={confirmBatch}
        setConfirmBatch={setConfirmBatch}
        withBackup={withBackup}
        repairable={repairable}
        repairVolumes={repairVolumes}
        breakdown={breakdown}
        resyncDamaged={resyncDamaged}
        setResyncDamaged={setResyncDamaged}
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
