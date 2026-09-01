import { useState } from "react";
import { useTranslation } from "react-i18next";
import { SEEK_SCAN_PACE_DEFAULT, scanRequestBody } from "@evey/api";
import type { SeekScanPace } from "@evey/api";
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
  // The pace is an argument of THIS scan and of nothing else on the page, so it
  // lives with the button that starts it. Gentle by default: nobody watches a
  // scan, everybody watches their films.
  const [pace, setPace] = useState<SeekScanPace>(SEEK_SCAN_PACE_DEFAULT);

  return (
    <div className="flex flex-col gap-2">
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
        <div className="w-56">
          <Select
            label={t("admin.seekIndex.pace")}
            value={pace}
            onChange={(v) => setPace(v as SeekScanPace)}
            disabled={busy}
            data-testid="seek-index-pace-select"
            options={[
              { value: "gentle", label: t("admin.seekIndex.paceGentle") },
              { value: "fast", label: t("admin.seekIndex.paceFast") },
            ]}
          />
        </div>
        <Button
          onClick={() => startScan.mutate({ data: scanRequestBody(libraryId, pace) })}
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
      <p className="text-sm text-[var(--text-muted)]" data-testid="seek-index-pace-hint">
        {pace === "fast" ? t("admin.seekIndex.paceFastHint") : t("admin.seekIndex.paceGentleHint")}
      </p>
    </div>
  );
}
