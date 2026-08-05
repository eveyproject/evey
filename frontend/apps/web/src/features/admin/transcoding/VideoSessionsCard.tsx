import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useAdminSystemServiceGetSystemInfo,
  useServerConfigServiceListServerConfigEntries,
  getServerConfigServiceListServerConfigEntriesQueryKey,
  serverConfigServiceSetServerConfigEntry,
  type V1VideoPool,
  type V1VideoRungFit,
} from "@evey/api";
import { toast } from "@evey/store";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";

const MAX_VIDEO_SESSIONS_KEY = "transcoding.max_video_sessions";
const CONFIG_QUERY_KEY = getServerConfigServiceListServerConfigEntriesQueryKey();

// poolRungFits picks the ONE ladder class each pool is best described by -
// "software" for the cpu pool (falling back to whatever else is there), and
// the (only) "hw" class for the gpu pool - so the card shows one clean list
// of paliers per pool instead of every benched class at once.
function poolRungFits(pool: V1VideoPool): V1VideoRungFit[] {
  const classes = pool.classes ?? [];
  const primary = classes.find((c) => c.ladderClass === "software") ?? classes[0];
  return primary?.rungFits ?? [];
}

// VideoSessionsCard governs the LIVE playback path: how many streams may be
// video-encoded at once. Distinct from ConcurrencyCard above, which queues the
// background transcode JOBS - two different subsystems that used to sit on two
// different admin pages and read as the same knob.
//
// This card reads as a CONSTAT, not a settings panel: the host was measured
// at boot and re-measures itself on hardware change / GPU loss, so the
// numbers here are a statement of fact, not something the admin is expected
// to configure. The one admin control that exists (reduce the measured
// capacity) stays a discreet, secondary control at the bottom - a safety
// valve, never the headline. An audio-only transcode (video copied) never
// consumes a slot, and one viewer switching quality holds a single slot.
export function VideoSessionsCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: sysData } = useAdminSystemServiceGetSystemInfo();
  const { data: entries } = useServerConfigServiceListServerConfigEntries({
    query: { select: (d) => d.entries ?? [] },
  });

  const stored = entries?.find((e) => e.key === MAX_VIDEO_SESSIONS_KEY)?.value ?? "";
  const [value, setValue] = useState(stored);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    if (!dirty) setValue(stored);
  }, [stored, dirty]);

  const save = useMutation({
    mutationFn: () => serverConfigServiceSetServerConfigEntry(MAX_VIDEO_SESSIONS_KEY, { value: value.trim() }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: CONFIG_QUERY_KEY });
      setDirty(false);
      toast.success(t("admin.transcoding.videoSessionsSaved", "Video transcode capacity updated"));
    },
    onError: () => toast.error(t("admin.transcoding.videoSessionsFailed", "Could not update the capacity")),
  });

  const vs = sysData?.videoSessions;
  const ev = vs?.evidence;
  const pools = vs?.pools ?? [];

  return (
    <div className="flex flex-col gap-3" data-testid="video-sessions-card">
      {vs && (
        <p className="text-xs text-[var(--text-muted)]">{t("admin.transcoding.videoSessionsAutoAdjusts")}</p>
      )}

      {pools.length > 0 ? (
        <div className="grid gap-3 sm:grid-cols-2" data-testid="video-pools">
          {pools.map((pool) => {
            const used = pool.budgetUsed ?? 0;
            const total = pool.budgetTotal ?? 0;
            const pct = total > 0 ? Math.min(100, Math.max(0, (used / total) * 100)) : 0;
            const rungs = poolRungFits(pool);
            const label =
              pool.pool === "gpu"
                ? t("admin.transcoding.videoPoolGpu")
                : t("admin.transcoding.videoPoolCpu");
            return (
              <div
                key={pool.pool}
                data-testid={`video-pool-${pool.pool}`}
                className="flex flex-col gap-2 rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--surface)] p-3"
              >
                <div className="flex items-center justify-between">
                  <span className="text-sm font-semibold text-[var(--text)]">{label}</span>
                  <span className="text-xs text-[var(--text-muted)]">
                    {t("admin.transcoding.videoPoolBudget", {
                      used: Math.round(used * 100) / 100,
                      total: Math.round(total * 100) / 100,
                    })}
                  </span>
                </div>
                <div
                  className="h-1.5 rounded-full bg-[var(--border)]"
                  role="progressbar"
                  aria-label={label}
                  aria-valuemin={0}
                  aria-valuemax={100}
                  aria-valuenow={Math.round(pct)}
                >
                  <div
                    className="h-1.5 rounded-full bg-[var(--accent)] transition-all duration-500"
                    style={{ width: `${pct}%` }}
                  />
                </div>
                {rungs.length > 0 && (
                  <ul className="flex flex-col gap-0.5 text-xs text-[var(--text-muted)]">
                    {rungs.map((r) => (
                      <li key={r.height}>
                        {t("admin.transcoding.videoPoolRungFit", {
                          count: r.maxStreams ?? 1,
                          height: r.height,
                        })}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            );
          })}
        </div>
      ) : (
        <>
          {vs && (
            <p className="text-sm text-[var(--text-muted)]" data-testid="video-sessions-measured">
              {t("admin.transcoding.videoSessionsMeasured", { auto: vs.autoCapacity, active: vs.active })}
            </p>
          )}
          {ev && ev.derived !== false && (
            <p className="text-xs text-[var(--text-muted)]">
              {t("admin.transcoding.videoSessionsEvidence", {
                height: ev.height,
                fps: Math.round((ev.fps ?? 0) * 10) / 10,
                requiredFps: Math.round((ev.requiredFps ?? 0) * 10) / 10,
                cores: ev.cpuBudget,
              })}
            </p>
          )}
          {ev && ev.derived === false && (
            <p className="text-xs text-[var(--warning)]">{t("admin.transcoding.videoSessionsNotMeasured")}</p>
          )}
        </>
      )}

      <div className="flex flex-col gap-1 pt-2 mt-1 border-t border-[var(--border)]">
        <Input
          id="max-video-sessions"
          label={t("admin.transcoding.maxVideoSessionsLabel")}
          type="number"
          min={1}
          placeholder={t("admin.transcoding.maxVideoSessionsAuto")}
          value={value}
          onChange={(e) => {
            setValue(e.target.value);
            setDirty(true);
          }}
          className="max-w-[12rem]"
        />
        <p className="text-xs text-[var(--text-muted)]">{t("admin.transcoding.maxVideoSessionsDesc")}</p>
        <div className="flex justify-end pt-1">
          <Button
            variant="ghost"
            onClick={() => save.mutate()}
            loading={save.isPending}
            disabled={!dirty}
            data-testid="max-video-sessions-save-btn"
          >
            {t("common.save")}
          </Button>
        </div>
      </div>
    </div>
  );
}
