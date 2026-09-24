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

export interface WriteCallbacks {
  onSuccess?: () => void;
  onError?: () => void;
}

export function useBackgroundJobsForm(opts: {
  entries: { key?: string; value?: string }[] | undefined;
  save: (key: string, value: string, callbacks?: WriteCallbacks) => void;
  remove: (key: string, callbacks?: WriteCallbacks) => void;
}) {
  const { entries, save, remove } = opts;
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
    toggle: (key: string, enabled: boolean) => (enabled ? remove(key) : save(key, "off")),
    // The dirty flag clears only once every write has landed: clearing it up
    // front reported success when every write failed, and dropped the edits.
    saveRates: () => {
      let remaining = BACKGROUND_JOB_RATES.length;
      let hasError = false;
      const settle = () => {
        remaining -= 1;
        if (remaining === 0 && !hasError) setRateDirty(false);
      };
      for (const field of BACKGROUND_JOB_RATES) {
        const value = (rateDrafts[field.key] ?? "").trim();
        const callbacks = { onSuccess: settle, onError: () => { hasError = true; } };
        if (value) save(field.key, value, callbacks);
        else remove(field.key, callbacks);
      }
    },
  };
}
