import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Select } from "@/components/ui/Select";
import type { SeekIndexPageState } from "./hooks";

type SeekIndexControlsProps = Pick<
  SeekIndexPageState,
  | "libraryId"
  | "setLibraryId"
  | "libraries"
  | "startScan"
  | "busy"
  | "result"
  | "repairable"
  | "backupsRisky"
  | "setKeepBackups"
  | "setConfirmBatch"
  | "withBackup"
>;

export function SeekIndexControls({
  libraryId,
  setLibraryId,
  libraries,
  startScan,
  busy,
  result,
  repairable,
  backupsRisky,
  setKeepBackups,
  setConfirmBatch,
  withBackup,
}: SeekIndexControlsProps) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-wrap items-end gap-3">
      <div className="w-64">
        <Select
          label={t("admin.seekIndex.library")}
          value={libraryId}
          onChange={setLibraryId}
          options={[
            { value: "", label: t("admin.seekIndex.allLibraries") },
            ...libraries.map((l) => ({ value: l.id ?? "", label: l.name ?? l.id ?? "" })),
          ]}
        />
      </div>
      <Button
        onClick={() => startScan.mutate({ data: libraryId ? { libraryId } : {} })}
        loading={busy}
        data-testid="seek-index-scan-btn"
      >
        {result ? t("admin.seekIndex.scanAgain") : t("admin.seekIndex.scan")}
      </Button>
      {result && (
        <>
          {/* The count IS the repairable verdict count: exactly the rows the
              job will process, nothing hidden behind an option. */}
          <Button
            variant="secondary"
            disabled={repairable === 0 || busy}
            onClick={() => {
              // Pre-set the safe backup choice; the operator can still flip it.
              setKeepBackups(!backupsRisky);
              setConfirmBatch("repair");
            }}
            data-testid="seek-index-repair-all-btn"
          >
            {t("admin.seekIndex.repairAll", { count: repairable })}
          </Button>
          <Button
            variant="secondary"
            disabled={withBackup === 0 || busy}
            onClick={() => setConfirmBatch("rollback")}
            data-testid="seek-index-rollback-all-btn"
          >
            {t("admin.seekIndex.rollbackAll", { count: withBackup })}
          </Button>
          <Button
            variant="ghost"
            disabled={withBackup === 0 || busy}
            onClick={() => setConfirmBatch("purge")}
            data-testid="seek-index-purge-all-btn"
          >
            {t("admin.seekIndex.purgeAll", { count: withBackup })}
          </Button>
        </>
      )}
    </div>
  );
}
