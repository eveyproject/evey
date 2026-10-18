import { useEffect, useRef, useState } from "react";

// The admin's background-job switches and rates (Server settings). The web and
// the phone kept the same field lists and the same form logic in two copies;
// a key added to one list only would have been invisible on the other device.
//
// These flags are read once by resolveFlag() at server boot and never re-read:
// a write lands in server_config at once but only takes effect on restart,
// which is why both cards carry the restart warning.
export const BACKGROUND_JOB_TOGGLES = [
  {
    key: "transcoding.pause_bg_on_playback",
    labelKey: "admin.serverConfig.backgroundJobs.pauseBgOnPlaybackLabel",
    descKey: "admin.serverConfig.backgroundJobs.pauseBgOnPlaybackDesc",
  },
  {
    key: "transcoding.level_backfill",
    labelKey: "admin.serverConfig.backgroundJobs.levelBackfillLabel",
    descKey: "admin.serverConfig.backgroundJobs.levelBackfillDesc",
  },
  {
    key: "transcoding.keyframe_backfill",
    labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillLabel",
    descKey: "admin.serverConfig.backgroundJobs.keyframeBackfillDesc",
  },
  {
    key: "transcoding.snapshot_fastpath",
    labelKey: "admin.serverConfig.backgroundJobs.snapshotFastpathLabel",
    descKey: "admin.serverConfig.backgroundJobs.snapshotFastpathDesc",
  },
] as const;

export const BACKGROUND_JOB_RATES = [
  { key: "transcoding.level_backfill_batch", labelKey: "admin.serverConfig.backgroundJobs.levelBackfillBatchLabel" },
  { key: "transcoding.level_backfill_pause_ms", labelKey: "admin.serverConfig.backgroundJobs.levelBackfillPauseMsLabel" },
  { key: "transcoding.level_backfill_idle_sec", labelKey: "admin.serverConfig.backgroundJobs.levelBackfillIdleSecLabel" },
  { key: "transcoding.keyframe_backfill_batch", labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillBatchLabel" },
  { key: "transcoding.keyframe_backfill_pause_ms", labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillPauseMsLabel" },
  { key: "transcoding.keyframe_backfill_idle_sec", labelKey: "admin.serverConfig.backgroundJobs.keyframeBackfillIdleSecLabel" },
] as const;

// save/remove are mutateAsync: TanStack keeps the callbacks of the last
// mutate() call only, so per-call callbacks over six writes never all fired.
// A toggle is one write and toasts its own outcome. The pace Save is six: it
// goes through the quiet writers and reports once, through onRatesSaved.
export function useBackgroundJobsForm(opts: {
  entries: { key?: string; value?: string }[] | undefined;
  save: (key: string, value: string) => Promise<unknown>;
  remove: (key: string) => Promise<unknown>;
  quietSave: (key: string, value: string) => Promise<unknown>;
  quietRemove: (key: string) => Promise<unknown>;
  onRatesSaved: (failedKeys: string[]) => void;
}) {
  const { entries, save, remove, quietSave, quietRemove, onRatesSaved } = opts;
  const [rateDrafts, setRateDrafts] = useState<Record<string, string>>({});
  const [rateDirty, setRateDirty] = useState(false);

  // The card can mount before the config query resolves: seed the drafts only
  // the first time real data arrives, so a refetch never clobbers edits.
  const seeded = useRef(false);
  useEffect(() => {
    if (seeded.current || !entries) return;
    seeded.current = true;
    const draft: Record<string, string> = {};
    for (const field of BACKGROUND_JOB_RATES) {
      draft[field.key] = entries.find((e) => e.key === field.key)?.value ?? "";
    }
    setRateDrafts(draft);
  }, [entries]);

  return {
    rateDrafts,
    rateDirty,
    setRate: (key: string, value: string) => {
      setRateDrafts((p) => ({ ...p, [key]: value }));
      setRateDirty(true);
    },
    isEnabled: (key: string) => entries?.find((e) => e.key === key)?.value !== "off",
    // Re-enabling reverts to the server default rather than writing "on".
    toggle: (key: string, enabled: boolean) => {
      (enabled ? remove(key) : save(key, "off")).catch(() => {});
    },
    // The dirty flag clears only once every write has landed: clearing it up
    // front reported success when every write failed, and dropped the edits.
    saveRates: async () => {
      const results = await Promise.allSettled(
        BACKGROUND_JOB_RATES.map((field) => {
          const value = (rateDrafts[field.key] ?? "").trim();
          return value ? quietSave(field.key, value) : quietRemove(field.key);
        }),
      );
      const failed = BACKGROUND_JOB_RATES.filter((_, i) => results[i].status === "rejected").map((f) => f.key);
      if (failed.length === 0) setRateDirty(false);
      onRatesSaved(failed);
    },
  };
}
