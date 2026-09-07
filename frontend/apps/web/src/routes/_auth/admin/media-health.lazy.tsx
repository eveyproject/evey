import { ScanSearch } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { PageHeader } from "@/components/ui/PageHeader";
import { Card } from "@/components/ui/Card";
import { InfoTooltip } from "@/components/ui/InfoTooltip";
import { EmptyState } from "@/components/ui/EmptyState";
import { errText } from "@evey/api";
import { useSeekIndexPage } from "@/features/admin/seekIndex/hooks";
import { JobProgressCard } from "@/features/admin/seekIndex/JobProgressCard";
import { LastJobCard } from "@/features/admin/seekIndex/LastJobCard";
import { RepairJournalPanel } from "@/features/admin/seekIndex/RepairJournalPanel";
import { SeekIndexControls } from "@/features/admin/seekIndex/SeekIndexControls";
import { StatsGrid } from "@/features/admin/seekIndex/StatsGrid";
import { SeekIndexTable } from "@/features/admin/seekIndex/SeekIndexTable";
import { SeekIndexFilters } from "@/features/admin/seekIndex/SeekIndexFilters";
import { RepairConfirmModal } from "@/features/admin/seekIndex/RepairConfirmModal";

export const Route = createLazyFileRoute("/_auth/admin/media-health")({
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

      {/* The header used to be one nine-line paragraph carrying the whole
          model: the two verdicts, every defect behind each of them, the backup
          policy and the read-only caveat. Nobody read it. What stays here is
          the one sentence that decides how to read the page plus a line per
          verdict; the rest is one click away, where it is looked up rather
          than waded through. */}
      <Card>
        <div className="flex flex-col gap-3 text-sm text-[var(--text-muted)]">
          <p className="text-[var(--text)]">
            {t("admin.seekIndex.description")}
            <span className="ml-1">
              <InfoTooltip label={t("admin.seekIndex.introMore")}>
                <span className="flex flex-col gap-2 text-xs text-[var(--text-muted)]">
                  <span className="block">{t("admin.seekIndex.introDetailScope")}</span>
                  <span className="block">{t("admin.seekIndex.introDetailRemarks")}</span>
                  <span className="block" data-testid="seek-index-ro-note">
                    {t("admin.seekIndex.readOnlyNote")}
                  </span>
                </span>
              </InfoTooltip>
            </span>
          </p>
          <div className="grid gap-2 sm:grid-cols-2">
            <p data-testid="seek-index-intro-repairable">
              <strong className="font-semibold text-[var(--text)]">{t("admin.seekIndex.statsRepairable")}</strong>
              {" - "}
              {t("admin.seekIndex.introRepairable")}
            </p>
            <p data-testid="seek-index-intro-informational">
              <strong className="font-semibold text-[var(--text)]">{t("admin.seekIndex.statsInformational")}</strong>
              {" - "}
              {t("admin.seekIndex.introInformational")}
            </p>
          </div>
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
      <RepairJournalPanel />

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
