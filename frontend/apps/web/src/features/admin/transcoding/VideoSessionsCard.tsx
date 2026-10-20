import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  useAdminSystemServiceGetSystemInfo,
  useServerConfigServiceListServerConfigEntries,
  getServerConfigServiceListServerConfigEntriesQueryKey,
  serverConfigServiceSetServerConfigEntry,
  type V1VideoPool,
  type V1VideoRungFit,
  type V1VideoSourceFit,
} from "@evey/api";
import { toast } from "@evey/store";
import { Input } from "@/components/ui/Input";
import { SettingRow } from "@/components/ui/SettingRow";

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

// sourceFits answers "how many can I serve" for the rung load actually lands
// on - the CHEAPEST one, the same rung the count cap is derived from. Reading
// the breakdown off the top rung instead would quote a ceiling nobody hits.
function sourceFits(rungs: V1VideoRungFit[]): V1VideoSourceFit[] {
  let cheapest: V1VideoRungFit | undefined;
  for (const r of rungs) {
    if (!r.bySource?.length) continue;
    if (!cheapest || (r.cost ?? 0) < (cheapest.cost ?? 0)) cheapest = r;
  }
  return cheapest?.bySource ?? [];
}

// codecLabel uppercases the wire codec for display ("h264" -> "H264").
function codecLabel(codec?: string): string {
  return (codec ?? "").toUpperCase();
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
  const { data: sysData } = useAdminSystemServiceGetSystemInfo();
  const vs = sysData?.videoSessions;
  const ev = vs?.evidence;
  const pools = vs?.pools ?? [];
  const benchSource = vs?.benchSource;
  const decodeRates = vs?.decodeThroughput ?? [];

  return (
    <div className="flex flex-col gap-3" data-testid="video-sessions-card">
      {vs && (
        <p className="text-xs text-[var(--text-muted)]">{t("admin.transcoding.videoSessionsAutoAdjusts")}</p>
      )}

      {(benchSource?.codec || decodeRates.length > 0) && (
        <p className="text-xs text-[var(--text-muted)]" data-testid="video-decode-evidence">
          {benchSource?.codec
            ? t("admin.transcoding.videoBenchSource", {
                codec: codecLabel(benchSource.codec),
                height: benchSource.height,
              })
            : null}
          {decodeRates.length > 0 && benchSource?.codec ? " · " : null}
          {decodeRates
            .map((d) =>
              t("admin.transcoding.videoDecodeRate", {
                codec: codecLabel(d.codec),
                fps: Math.round(d.fps ?? 0),
              }),
            )
            .join(" · ")}
        </p>
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
                {sourceFits(rungs).length > 0 && (
                  <div
                    className="flex flex-col gap-1 border-t border-[var(--border)] pt-2"
                    data-testid={`video-pool-${pool.pool}-sources`}
                  >
                    <span className="text-xs font-medium text-[var(--text)]">
                      {t("admin.transcoding.videoSourceTitle")}
                    </span>
                    <ul className="flex flex-col gap-0.5 text-xs text-[var(--text-muted)]">
                      {sourceFits(rungs).map((f) => (
                        <li key={`${f.source?.codec}-${f.source?.height}`} className="flex justify-between gap-2">
                          <span>
                            {codecLabel(f.source?.codec)} {f.source?.height}p
                          </span>
                          <span className="tabular-nums">
                            {t("admin.transcoding.videoSourceStreams", { count: f.maxStreams ?? 1 })}
                          </span>
                        </li>
                      ))}
                    </ul>
                  </div>
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

    </div>
  );
}

// useMaxVideoSessionsField is the one control over live playback capacity:
// lower the measured number of simultaneous video streams. Blank keeps the
// measured value ("Auto"); the server never lets it go above the measure.
// It returns its row and its save, so the Capacity group offers one Save for
// both of its limits.
export function useMaxVideoSessionsField(): { row: ReactNode; dirty: boolean; submit: () => Promise<boolean> } {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: entries } = useServerConfigServiceListServerConfigEntries({
    query: { select: (d) => d.entries ?? [] },
  });

  const stored = entries?.find((e) => e.key === MAX_VIDEO_SESSIONS_KEY)?.value ?? "";
  const [value, setValue] = useState(stored);
  const [dirty, setDirty] = useState(false);
  const [error, setError] = useState<string | undefined>();

  useEffect(() => {
    if (!dirty) setValue(stored);
  }, [stored, dirty]);

  const save = useMutation({
    mutationFn: (v: string) => serverConfigServiceSetServerConfigEntry(MAX_VIDEO_SESSIONS_KEY, { value: v }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: CONFIG_QUERY_KEY });
      setDirty(false);
      toast.success(t("admin.transcoding.videoSessionsSaved", "Video transcode capacity updated"));
    },
    onError: () => toast.error(t("admin.transcoding.videoSessionsFailed", "Could not update the capacity")),
  });

  async function submit(): Promise<boolean> {
    if (!dirty) return true;
    const v = value.trim();
    if (v !== "" && (!/^\d+$/.test(v) || parseInt(v, 10) < 1)) {
      setError(t("admin.settingsLayout.errorNumber", { min: 1 }));
      return false;
    }
    try {
      await save.mutateAsync(v);
      return true;
    } catch {
      return false;
    }
  }

  const row = (
    <SettingRow
      key="max-video-sessions"
      label={t("admin.transcoding.maxVideoSessionsLabel")}
      htmlFor="max-video-sessions"
      info={t("admin.transcoding.maxVideoSessionsDesc")}
      error={error}
      data-testid="max-video-sessions-row"
    >
      <Input
        id="max-video-sessions"
        type="number"
        inputMode="numeric"
        min={1}
        placeholder={t("admin.transcoding.maxVideoSessionsAuto")}
        value={value}
        onChange={(e) => {
          setValue(e.target.value);
          setDirty(true);
          setError(undefined);
        }}
        className="w-full"
      />
    </SettingRow>
  );
  return { row, dirty, submit };
}
