import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { Checkbox } from "@/components/ui/Checkbox";
import { BACKUP_WINDOW, formatSize, volumeBackupsRisky, volumeTempRisky } from "./helpers";
import type { SeekIndexPageState } from "./hooks";

type RepairConfirmModalProps = Pick<
  SeekIndexPageState,
  | "confirmBatch"
  | "setConfirmBatch"
  | "withBackup"
  | "repairable"
  | "repairVolumes"
  | "delayedAudio"
  | "alsoAudio"
  | "setAlsoAudio"
  | "keepBackups"
  | "setKeepBackups"
  | "repairLib"
  | "rollbackLib"
  | "purgeLib"
  | "libraryId"
>;

export function RepairConfirmModal({
  confirmBatch,
  setConfirmBatch,
  withBackup,
  repairable,
  repairVolumes,
  delayedAudio,
  alsoAudio,
  setAlsoAudio,
  keepBackups,
  setKeepBackups,
  repairLib,
  rollbackLib,
  purgeLib,
  libraryId,
}: RepairConfirmModalProps) {
  const { t } = useTranslation();

  return (
    <Modal
      open={confirmBatch !== null}
      onClose={() => setConfirmBatch(null)}
      title={
        confirmBatch === "rollback"
          ? t("admin.seekIndex.rollbackAllConfirmTitle")
          : confirmBatch === "purge"
            ? t("admin.seekIndex.purgeAllConfirmTitle")
            : t("admin.seekIndex.repairAllConfirmTitle")
      }
    >
      <p className="text-sm text-[var(--text-muted)] mb-4">
        {confirmBatch === "rollback"
          ? t("admin.seekIndex.rollbackAllConfirm", { count: withBackup })
          : confirmBatch === "purge"
            ? t("admin.seekIndex.purgeAllConfirm", { count: withBackup })
            : t("admin.seekIndex.repairAllConfirm", { count: repairable })}
      </p>
      {confirmBatch === "repair" && repairVolumes.length > 0 && (
        <ul className="mb-4 flex flex-col gap-1" data-testid="seek-index-volumes">
          {repairVolumes.map((v) => (
            <li key={v.root} className="text-xs text-[var(--text-muted)]">
              {t("admin.seekIndex.volumeLine", {
                root: v.root,
                free: formatSize(Number(v.freeBytes ?? 0)),
                missing: formatSize(Number(v.missingBytes ?? 0)),
                count: v.missingFiles ?? 0,
              })}
              {volumeBackupsRisky(v) && (
                <span className="text-[var(--color-warning)]">
                  {" "}
                  - {t("admin.seekIndex.volumeBackupsRisky")}
                </span>
              )}
              {volumeTempRisky(v) && (
                <span className="text-[var(--color-danger)]">
                  {" "}
                  - {t("admin.seekIndex.volumeTempRisky", {
                    largest: formatSize(Number(v.largestMissingBytes ?? 0)),
                  })}
                </span>
              )}
            </li>
          ))}
        </ul>
      )}
      {confirmBatch === "repair" && (
        <div className="mb-4 flex flex-col gap-3">
          {delayedAudio > 0 && (
            <div>
              <Checkbox
                label={t("admin.seekIndex.alsoAudio", { count: delayedAudio })}
                checked={alsoAudio}
                onChange={setAlsoAudio}
                data-testid="seek-index-also-audio"
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t("admin.seekIndex.alsoAudioHint")}
              </p>
            </div>
          )}
          <div>
            <Checkbox
              label={t("admin.seekIndex.keepBackups")}
              checked={keepBackups}
              onChange={setKeepBackups}
              data-testid="seek-index-keep-backups"
            />
            <p className="mt-1 text-xs text-muted-foreground">
              {keepBackups
                ? t("admin.seekIndex.keepBackupsHint")
                : t("admin.seekIndex.keepBackupsOffHint", { n: BACKUP_WINDOW })}
            </p>
          </div>
        </div>
      )}
      <div className="flex justify-end gap-2">
        <Button variant="ghost" onClick={() => setConfirmBatch(null)}>
          {t("common.cancel")}
        </Button>
        <Button
          loading={repairLib.isPending || rollbackLib.isPending || purgeLib.isPending}
          onClick={() => {
            if (confirmBatch === "rollback") rollbackLib.mutate({ data: libraryId ? { libraryId } : {} });
            else if (confirmBatch === "purge") purgeLib.mutate({ data: libraryId ? { libraryId } : {} });
            else
              // Strategy fixed to auto; when backups are not kept, a rolling
              // window (drained at job end) protects the run and deep-verify
              // proves each repair in place of the lost post-run rollback.
              repairLib.mutate({
                data: {
                  ...(libraryId ? { libraryId } : {}),
                  mode: "auto",
                  includeAudio: alsoAudio,
                  deepVerify: !keepBackups,
                  ...(keepBackups ? {} : { backupWindow: BACKUP_WINDOW }),
                },
              });
            setConfirmBatch(null);
          }}
          data-testid="seek-index-confirm-batch-btn"
        >
          {t("common.confirm")}
        </Button>
      </div>
    </Modal>
  );
}
