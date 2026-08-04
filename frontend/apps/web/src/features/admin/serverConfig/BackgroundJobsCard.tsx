import { useState, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Switch } from "@/components/ui/Switch";
import { useSetConfigEntry, useDeleteConfigEntry, type ConfigEntry } from "./hooks";

// These flags are read once by resolveFlag() at server boot (cmd/evey/wire.go)
// and never re-read afterwards - editing them here writes to server_config
// immediately, but the change has zero effect on the running process until
// it's restarted. The warning banner below exists because this bit the
// operator during a real incident: the mitigation existed in the DB but
// wasn't discoverable without grepping the Go source for the exact key.
const BG_PAUSE_KEY = "transcoding.pause_bg_on_playback";
const BG_LEVEL_BACKFILL_KEY = "transcoding.level_backfill";
const BG_KEYFRAME_BACKFILL_KEY = "transcoding.keyframe_backfill";
const BG_SNAPSHOT_FASTPATH_KEY = "transcoding.snapshot_fastpath";

const BG_TOGGLE_FIELDS = [
  {
    key: BG_PAUSE_KEY,
    labelKey: "admin.serverConfig.backgroundJobs.pauseBgOnPlaybackLabel",
    descKey: "admin.serverConfig.backgroundJobs.pauseBgOnPlaybackDesc",
  },
  {
    key: BG_LEVEL_BACKFILL_KEY,
    labelKey: "admin.serverConfig.backgroundJobs.levelBackfillLabel",
    descKey: "admin.serverConfig.backgroundJobs.levelBackfillDesc",
  },
  {
    key: BG_KEYFRAME_BACKFILL_KEY,
    labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillLabel",
    descKey: "admin.serverConfig.backgroundJobs.keyframeBackfillDesc",
  },
  {
    key: BG_SNAPSHOT_FASTPATH_KEY,
    labelKey: "admin.serverConfig.backgroundJobs.snapshotFastpathLabel",
    descKey: "admin.serverConfig.backgroundJobs.snapshotFastpathDesc",
  },
] as const;

const BG_RATE_FIELDS = [
  { key: "transcoding.level_backfill_batch", labelKey: "admin.serverConfig.backgroundJobs.levelBackfillBatchLabel" },
  { key: "transcoding.level_backfill_pause_ms", labelKey: "admin.serverConfig.backgroundJobs.levelBackfillPauseMsLabel" },
  { key: "transcoding.level_backfill_idle_sec", labelKey: "admin.serverConfig.backgroundJobs.levelBackfillIdleSecLabel" },
  { key: "transcoding.keyframe_backfill_batch", labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillBatchLabel" },
  { key: "transcoding.keyframe_backfill_pause_ms", labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillPauseMsLabel" },
  { key: "transcoding.keyframe_backfill_idle_sec", labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillIdleSecLabel" },
] as const;

export function BackgroundJobsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();
  const deleteEntry = useDeleteConfigEntry();

  const [rateDrafts, setRateDrafts] = useState<Record<string, string>>({});
  const [rateDirty, setRateDirty] = useState(false);

  // Same re-seed-once pattern as SmtpSettingsCard: this card can mount before
  // the config query resolves, so seed rate drafts only the first time real
  // data arrives to avoid clobbering in-progress edits with an empty refetch.
  const seeded = useRef(false);
  useEffect(() => {
    if (seeded.current || !entries) return;
    seeded.current = true;
    const draft: Record<string, string> = {};
    for (const field of BG_RATE_FIELDS) {
      draft[field.key] = entries.find((e) => e.key === field.key)?.value ?? "";
    }
    setRateDrafts(draft);
  }, [entries]);

  function isEnabled(key: string) {
    return entries?.find((e) => e.key === key)?.value !== "off";
  }

  function handleToggle(key: string, enabled: boolean) {
    if (enabled) {
      // Re-enabling reverts to the server default rather than writing "on",
      // consistent with the reset-to-default pattern used elsewhere on this page.
      deleteEntry.mutate(key);
    } else {
      setEntry.mutate({ key, value: "off" });
    }
  }

  function handleSaveRates() {
    // The writes are async, so the form must not clear its dirty flag until
    // they all land: clearing up front reported success even when every write
    // failed, and dropped the edits the operator would need to retry.
    // Per-write feedback already comes from useSetConfigEntry/useDeleteConfigEntry.
    let remaining = BG_RATE_FIELDS.length;
    let hasError = false;
    const settle = () => {
      remaining -= 1;
      if (remaining === 0 && !hasError) setRateDirty(false);
    };
    for (const field of BG_RATE_FIELDS) {
      const value = (rateDrafts[field.key] ?? "").trim();
      const callbacks = { onSuccess: settle, onError: () => { hasError = true; } };
      if (value) {
        setEntry.mutate({ key: field.key, value }, callbacks);
      } else {
        deleteEntry.mutate(field.key, callbacks);
      }
    }
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="rounded-[var(--radius-md)] border border-[color-mix(in_srgb,var(--color-warning)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_5%,transparent)] p-3">
        <p className="text-sm font-medium text-[var(--color-warning)]">
          {t("admin.serverConfig.backgroundJobs.restartWarning")}
        </p>
      </div>

      <div className="flex flex-col gap-3">
        {BG_TOGGLE_FIELDS.map((field) => (
          <div key={field.key} className="flex flex-col gap-1">
            <div className="flex items-center gap-3">
              <span className="text-sm font-medium text-[var(--text)]">{t(field.labelKey)}</span>
              <Switch
                label={t(field.labelKey)}
                checked={isEnabled(field.key)}
                onChange={() => handleToggle(field.key, !isEnabled(field.key))}
                disabled={setEntry.isPending || deleteEntry.isPending}
              />
            </div>
            <p className="text-xs text-[var(--text-muted)]">{t(field.descKey)}</p>
          </div>
        ))}
      </div>

      <div className="flex flex-col gap-3 pt-2 border-t border-[var(--border)]">
        <p className="text-sm font-medium text-[var(--text)]">
          {t("admin.serverConfig.backgroundJobs.paceTitle")}
        </p>
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.serverConfig.backgroundJobs.paceDesc")}
        </p>
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
          {BG_RATE_FIELDS.map((field) => (
            <Input
              key={field.key}
              label={t(field.labelKey)}
              type="number"
              min={0}
              placeholder={t("admin.serverConfig.backgroundJobs.paceDefaultPlaceholder")}
              value={rateDrafts[field.key] ?? ""}
              onChange={(e) => {
                setRateDrafts((p) => ({ ...p, [field.key]: e.target.value }));
                setRateDirty(true);
              }}
            />
          ))}
        </div>
        <div className="flex justify-end">
          <Button onClick={handleSaveRates} loading={setEntry.isPending} disabled={!rateDirty}>
            {t("common.save")}
          </Button>
        </div>
      </div>

      {(setEntry.isPending || deleteEntry.isPending) && <Spinner size="sm" />}
    </div>
  );
}
