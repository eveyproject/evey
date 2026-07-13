import { useEffect, useState } from "react";
import { ChevronDown } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  eveyFetch,
  useAdminSystemServiceGetSystemInfo,
  useServerConfigServiceListServerConfigEntries,
  getServerConfigServiceListServerConfigEntriesQueryKey,
  serverConfigServiceSetServerConfigEntry,
  type V1VideoPool,
  type V1VideoRungFit,
} from "@evey/api";
import { toast } from "@evey/store";
import { PageHeader } from "@/components/ui/PageHeader";
import { CollapsibleCard } from "@/components/ui/CollapsibleCard";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { Spinner } from "@/components/ui/Spinner";
import { EncodeCalibrationCard } from "@/features/admin/EncodeCalibrationCard";

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
function VideoSessionsCard() {
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

// --- LoadTestCard --------------------------------------------------------
//
// The backend side of this contract (POST/GET/DELETE /api/v1/admin/load-test)
// is not yet expressed as a proto/orval-generated client in this tree (the
// service is landing in a parallel worktree). Until it does, this talks to
// the endpoint through eveyFetch directly with hand-written types mirroring
// the documented wire shape - the same pattern ConcurrencyCard below already
// uses for /admin/server-settings. Swap these local types for the generated
// ones once the client lands; the shape is a 1:1 match so no behavior change
// is expected.

// The 3 "pure" scenarios exercise a single playback path; the 3 usage
// profiles mix mkvgo direct-play and transcode viewers in a fixed ratio to
// stand in for a real household's device mix ("how many different humans").
type LoadTestScenario = "transcode" | "natural" | "mkvgo" | "light" | "balanced" | "heavy";

type LoadTestStopReason =
  | "slo_breach"
  | "admission_full"
  | "cpu_saturated"
  | "memory_saturated"
  | "throughput_saturated"
  | "startup_too_slow"
  | "server_failing"
  | "library_limit"
  | "max_viewers"
  | "cancelled"
  | "token_expired"
  | "error";

// What the server was actually spending. Read from its own cgroup, so every
// ffmpeg it spawned is already counted.
interface LoadTestResources {
  cpuPercentAvg?: number;
  cpuPercentPeak?: number;
  memUsedBytesAvg?: number;
  memUsedBytesMax?: number;
  memLimitBytes?: number;
  diskReadBytesPerSec?: number;
  servedBytesPerSec?: number;
  samples?: number;
}

interface LoadTestReport {
  scenario?: LoadTestScenario;
  sustainedViewers?: number;
  stopReason?: LoadTestStopReason;
  stopDetail?: string;
  ttffMsP50?: number;
  ttffMsP95?: number;
  segmentMsP50?: number;
  segmentMsP95?: number;
  segmentSloMs?: number;
  samples?: number;
  sharedCacheViewers?: number;
  startedAt?: string;
  endedAt?: string;
  // skippedCandidates: titles that weren't mkvgo-eligible and were passed
  // over while picking media for the mkvgo/light/balanced/heavy scenarios.
  skippedCandidates?: number;
  // viewerMix: composition of the sustained rung by playback path, e.g.
  // {"mkvgo": 4, "transcode": 1} - keyed the same as segmentMsP95ByType.
  viewerMix?: Record<string, number>;
  segmentMsP95ByType?: Record<string, number>;
  // segmentSloMsByType: the latency budget each path was JUDGED against. The
  // paths don't share a segment length, so they can't share a budget.
  segmentSloMsByType?: Record<string, number>;
  // resources: what the box consumed while sustaining that viewer count - the
  // evidence behind a cpu_saturated / memory_saturated verdict.
  resources?: LoadTestResources;
  // perViewerCost: what ONE more viewer costs, as a slope across tiers.
  perViewerCost?: LoadTestResources;
  resourceBaseline?: LoadTestResources;
  // throughputCeilingBytesPerSec: the byte rate the server could not exceed. On
  // a media server this is the usual ceiling - how fast it can READ the films -
  // and no CPU or memory gauge can see it.
  throughputCeilingBytesPerSec?: number;
  diskReadStarved?: boolean;
  // diskReadCeilingBytesPerSec: how fast this box reads its media - every disk
  // the library spans, read at once. The kernel cannot report this when the
  // library sits on a network share or a 9p mount, so the run measures it itself
  // rather than printing a misleading zero.
  diskReadCeilingBytesPerSec?: number;
  // slowestDiskBytesPerSec: what the WEAKEST drive delivered. Several disks are
  // not one big disk - each saturates on its own, and the film a viewer picks
  // decides which.
  slowestDiskBytesPerSec?: number;
  disksProbed?: number;
  // storageCollapsesAtReaders: how many concurrent readers the storage takes
  // before it FALLS OVER (throughput halves, a single read takes seconds). On a
  // direct-play server this is roughly the number of people who can watch at
  // once before the storage - not the CPU, not Evey - makes playback crawl.
  storageCollapsesAtReaders?: number;
  // endpoint: the origin the viewers actually hit (the reverse proxy when it was
  // reachable, the server's own loopback otherwise). Runs against different
  // endpoints are not comparable.
  endpoint?: string;
  viaProxy?: boolean;
}

interface LoadTestStatus {
  running?: boolean;
  phase?: string;
  viewers?: number;
  elapsedMs?: number;
  lastReport?: LoadTestReport;
}

const LOAD_TEST_URL = "/api/v1/admin/load-test";
const LOAD_TEST_QUERY_KEY = ["admin-load-test-status"];

// A POST refused because real playback is active surfaces as 409 (conflict),
// 423 (locked) or 500 (the admission guard erroring out defensively) - all
// three read the same to an operator: "something is already streaming".
const ACTIVE_STREAMS_STATUSES = [409, 423, 500];

const STOP_REASON_KEYS: Partial<Record<LoadTestStopReason, string>> = {
  slo_breach: "admin.loadtest.stopReason.sloBreach",
  admission_full: "admin.loadtest.stopReason.admissionFull",
  cpu_saturated: "admin.loadtest.stopReason.cpuSaturated",
  memory_saturated: "admin.loadtest.stopReason.memorySaturated",
  throughput_saturated: "admin.loadtest.stopReason.throughputSaturated",
  startup_too_slow: "admin.loadtest.stopReason.startupTooSlow",
  server_failing: "admin.loadtest.stopReason.serverFailing",
  library_limit: "admin.loadtest.stopReason.libraryLimit",
  max_viewers: "admin.loadtest.stopReason.maxViewers",
  cancelled: "admin.loadtest.stopReason.cancelled",
  token_expired: "admin.loadtest.stopReason.tokenExpired",
  error: "admin.loadtest.stopReason.error",
};

// A run that ended because a RESOURCE ran out has found the machine's real
// ceiling; anything else means the ramp was stopped by something other than
// capacity (the operator's own limit, the clock, an error).
const BOTTLENECK_REASONS: LoadTestStopReason[] = [
  "cpu_saturated",
  "memory_saturated",
  "throughput_saturated",
  "startup_too_slow",
  "server_failing",
  "slo_breach",
  "admission_full",
];

function formatBytes(bytes: number): string {
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} Go`;
  if (bytes >= 1 << 20) return `${Math.round(bytes / (1 << 20))} Mo`;
  return `${Math.round(bytes / (1 << 10))} Ko`;
}

// Throughput is quoted in bits per second, the way a network link is.
function formatMbps(bytesPerSec: number): string {
  return `${((bytesPerSec * 8) / 1e6).toFixed(1)} Mb/s`;
}

// Noun form of each scenario for the report headline ("...in {{scenario}}.").
const SCENARIO_NOUN_KEYS: Record<LoadTestScenario, string> = {
  transcode: "admin.loadtest.scenarioTranscodeNoun",
  natural: "admin.loadtest.scenarioNaturalNoun",
  mkvgo: "admin.loadtest.scenarioMkvgoNoun",
  light: "admin.loadtest.scenarioLightNoun",
  balanced: "admin.loadtest.scenarioBalancedNoun",
  heavy: "admin.loadtest.scenarioHeavyNoun",
};

// Playback-path label for a viewerMix / segmentMsP95ByType key. Falls back to
// the raw key for any type the server reports that the panel doesn't know
// about yet, so a new backend type never renders blank.
const MIX_TYPE_LABEL_KEYS: Record<string, string> = {
  mkvgo: "admin.loadtest.mixType.mkvgo",
  transcode: "admin.loadtest.mixType.transcode",
  natural: "admin.loadtest.mixType.natural",
};

function mixTypeLabel(t: (key: string, opts?: Record<string, unknown>) => string, type: string): string {
  const key = MIX_TYPE_LABEL_KEYS[type];
  return key ? t(key) : type;
}

// Renders "{{count}} {{label}}" (or "{{label}} {{ms}} ms") per playback path
// present in the record, joined into one discreet line - never a table.
function mixParts(
  t: (key: string, opts?: Record<string, unknown>) => string,
  record: Record<string, number> | undefined,
  partKey: string,
  valueField: "count" | "ms",
): string {
  return Object.entries(record ?? {})
    .filter(([, v]) => (v ?? 0) > 0)
    .map(([type, v]) => t(partKey, { [valueField]: v, label: mixTypeLabel(t, type) }))
    .join(", ");
}

function formatElapsed(ms: number): string {
  const totalSec = Math.max(0, Math.round(ms / 1000));
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

// predictedStreams reads the SAME calibration table VideoSessionsCard renders
// above (pools[].classes[].rungFits[]) and returns the median rung's
// max_streams for the ladder class the load test actually exercises: "hw"
// when a GPU pool exists, "software" otherwise - independent of which
// scenario the operator picked, since the pool a stream lands on is a
// hardware fact, not a test setting.
function predictedStreams(pools: V1VideoPool[]): number | undefined {
  const gpuPool = pools.find((p) => p.pool === "gpu");
  const pool = gpuPool ?? pools.find((p) => p.pool === "cpu");
  const classes = pool?.classes ?? [];
  const wanted = gpuPool ? "hw" : "software";
  const cls = classes.find((c) => c.ladderClass === wanted) ?? classes[0];
  const rungs = cls?.rungFits ?? [];
  if (rungs.length === 0) return undefined;
  return rungs[Math.floor(rungs.length / 2)]?.maxStreams;
}

// LoadTestCard is the "constat, not a control panel" companion to
// VideoSessionsCard: instead of trusting the calibration's prediction, it
// throws real simulated viewers at the server and reports what actually held
// up. Repose (launch) -> running (poll) -> report, with the calibration's own
// prediction shown alongside the measured number for a sanity check.
function LoadTestCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: sysData } = useAdminSystemServiceGetSystemInfo();
  const pools = sysData?.videoSessions?.pools ?? [];
  // "balanced" (50/50 mkvgo/transcode) answers the question that actually
  // matters to an operator - "how many different humans, with a realistic
  // device mix, can this host hold" - so it's the default, not a pure lane.
  const [scenario, setScenario] = useState<LoadTestScenario>("balanced");

  const statusQuery = useQuery<LoadTestStatus>({
    queryKey: LOAD_TEST_QUERY_KEY,
    queryFn: () => eveyFetch<LoadTestStatus>({ url: LOAD_TEST_URL, method: "GET" }),
    refetchInterval: (query) => (query.state.data?.running ? 2000 : false),
  });
  const status = statusQuery.data;
  const running = status?.running ?? false;
  const report = status?.lastReport;

  const start = useMutation({
    // No viewer ceiling is sent, and that is the point: the run climbs until a
    // RESOURCE gives out. A number picked here would just cap the answer, and
    // the whole question is how far this machine actually goes.
    mutationFn: () =>
      eveyFetch<LoadTestStatus>({
        url: LOAD_TEST_URL,
        method: "POST",
        data: { scenario },
      }),
    onSuccess: (data) => {
      qc.setQueryData(LOAD_TEST_QUERY_KEY, data);
      void qc.invalidateQueries({ queryKey: LOAD_TEST_QUERY_KEY });
    },
  });

  const cancel = useMutation({
    mutationFn: () => eveyFetch<LoadTestStatus>({ url: LOAD_TEST_URL, method: "DELETE" }),
    onSuccess: (data) => {
      qc.setQueryData(LOAD_TEST_QUERY_KEY, data);
      void qc.invalidateQueries({ queryKey: LOAD_TEST_QUERY_KEY });
    },
  });

  const startError = start.error as (Error & { httpStatus?: number }) | null;
  const blockedByActiveStreams =
    !!startError && ACTIVE_STREAMS_STATUSES.includes(startError.httpStatus ?? 0);

  const predicted = predictedStreams(pools);

  return (
    <div className="flex flex-col gap-3 pt-3 mt-2 border-t border-[var(--border)]" data-testid="load-test-card">
      <h4 className="text-sm font-semibold text-[var(--text)]">{t("admin.loadtest.heading")}</h4>

      {running && status ? (
        <div className="flex flex-col gap-2" data-testid="load-test-running">
          <p className="text-sm text-[var(--text)]">
            {t("admin.loadtest.runningViewers", { count: status.viewers ?? 0 })}
            {" · "}
            {t("admin.loadtest.runningPhase", { phase: status.phase ?? "" })}
            {" · "}
            {t("admin.loadtest.runningElapsed", { elapsed: formatElapsed(status.elapsedMs ?? 0) })}
          </p>
          <div className="flex justify-start">
            <Button
              variant="secondary"
              loading={cancel.isPending}
              onClick={() => cancel.mutate()}
              data-testid="load-test-cancel-btn"
            >
              {t("admin.loadtest.cancelButton")}
            </Button>
          </div>
        </div>
      ) : (
        <>
          {report && (
            <div className="flex flex-col gap-1.5" data-testid="load-test-report">
              <p className="text-xl font-bold text-[var(--text)]">
                {t("admin.loadtest.reportHeadline", {
                  count: report.sustainedViewers ?? 0,
                  scenario: t(SCENARIO_NOUN_KEYS[report.scenario ?? "balanced"]),
                })}
              </p>
              {/* The calibration predicts TRANSCODE streams, so it may only be
                  compared against the transcode viewers this run actually held.
                  Showing "~2 predicted" next to 61 direct-play viewers compares
                  an encoder budget to a file server and reads as a wild miss,
                  when the two numbers simply measure different things. */}
              {predicted !== undefined && (report.viewerMix?.transcode ?? 0) > 0 && (
                <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-predicted">
                  {t("admin.loadtest.predictedLine", {
                    count: predicted,
                    measured: report.viewerMix?.transcode ?? 0,
                  })}
                </p>
              )}
              {report.viewerMix && Object.keys(report.viewerMix).length > 0 && (
                <p className="text-sm text-[var(--text)]" data-testid="load-test-viewer-mix">
                  {t("admin.loadtest.viewerMixLine", {
                    parts: mixParts(t, report.viewerMix, "admin.loadtest.mixPart", "count"),
                  })}
                </p>
              )}
              <p className="text-sm text-[var(--text-muted)]">
                {t("admin.loadtest.ttffLine", {
                  p50: Math.round(report.ttffMsP50 ?? 0),
                  p95: Math.round(report.ttffMsP95 ?? 0),
                })}
              </p>
              <p className="text-sm text-[var(--text-muted)]">
                {t("admin.loadtest.segmentLine", {
                  p50: Math.round(report.segmentMsP50 ?? 0),
                  p95: Math.round(report.segmentMsP95 ?? 0),
                })}
              </p>
              {report.segmentMsP95ByType && Object.keys(report.segmentMsP95ByType).length > 0 && (
                <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-segment-by-type">
                  {t("admin.loadtest.segmentByTypeLine", {
                    parts: mixParts(t, report.segmentMsP95ByType, "admin.loadtest.segmentByTypePart", "ms"),
                  })}
                </p>
              )}
              {/* Each path is judged against its OWN segment length: mkvgo packages
                  on keyframe boundaries, the ffmpeg ladder emits 3s segments. A
                  single shared budget would measure one path with the other's
                  yardstick - and the looser one always wins. */}
              {report.segmentSloMsByType && Object.keys(report.segmentSloMsByType).length > 0 && (
                <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-slo-by-type">
                  {t("admin.loadtest.sloByTypeLine", {
                    parts: mixParts(t, report.segmentSloMsByType, "admin.loadtest.segmentByTypePart", "ms"),
                  })}
                </p>
              )}

              {/* What those viewers actually COST the box, and therefore what ran
                  out first. A viewer count without the resource behind it says
                  nothing about WHY the server stopped there. */}
              {(report.resources?.samples ?? 0) > 0 && (
                <p className="text-sm text-[var(--text)]" data-testid="load-test-resources">
                  {t("admin.loadtest.resourcesLine", {
                    cpu: Math.round(report.resources?.cpuPercentAvg ?? 0),
                    cpuPeak: Math.round(report.resources?.cpuPercentPeak ?? 0),
                    mem: formatBytes(report.resources?.memUsedBytesAvg ?? 0),
                    memLimit: formatBytes(report.resources?.memLimitBytes ?? 0),
                    disk: formatMbps(report.resources?.diskReadBytesPerSec ?? 0),
                    net: formatMbps(report.resources?.servedBytesPerSec ?? 0),
                  })}
                </p>
              )}
              {/* Several disks are not one big disk: the weakest decides how bad
                  it gets when viewers land on films that live there. */}
              {(report.disksProbed ?? 0) > 1 && (report.slowestDiskBytesPerSec ?? 0) > 0 && (
                <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-slowest-disk">
                  {t("admin.loadtest.slowestDiskLine", {
                    rate: formatMbps(report.slowestDiskBytesPerSec ?? 0),
                  })}
                </p>
              )}
              {/* Only when it IS the verdict. The run notices a flat throughput
                  curve at some point in almost every climb (a tier that gained
                  nothing over a smaller one), but if the server ultimately gave
                  out for another reason, announcing "the throughput is the
                  bottleneck" contradicts the verdict two lines below. */}
              {report.stopReason === "throughput_saturated" && (report.throughputCeilingBytesPerSec ?? 0) > 0 && (
                <p className="text-sm font-medium text-[var(--text)]" data-testid="load-test-throughput-ceiling">
                  {t("admin.loadtest.throughputCeilingLine", {
                    rate: formatMbps(report.throughputCeilingBytesPerSec ?? 0),
                  })}
                </p>
              )}
              {/* The single most actionable line in the report when it fires:
                  storage that serialises (a 9p/virtiofs mount on one channel, an
                  over-subscribed NFS export, a disk thrashing between streams)
                  looks perfectly healthy on every gauge, and quietly decides how
                  many people can watch. */}
              {(report.storageCollapsesAtReaders ?? 0) > 0 && (
                <p className="text-sm font-medium text-[var(--color-warning)]" data-testid="load-test-storage-collapse">
                  {t("admin.loadtest.storageCollapseLine", {
                    count: report.storageCollapsesAtReaders ?? 0,
                  })}
                </p>
              )}
              {(report.diskReadCeilingBytesPerSec ?? 0) > 0 && (
                <p className="text-sm text-[var(--text)]" data-testid="load-test-disk-ceiling">
                  {t("admin.loadtest.diskCeilingLine", {
                    rate: formatMbps(report.diskReadCeilingBytesPerSec ?? 0),
                    count: report.disksProbed ?? 1,
                  })}
                </p>
              )}
              {(report.perViewerCost?.samples ?? 0) > 0 && (
                <p className="text-sm text-[var(--text)]" data-testid="load-test-per-viewer">
                  {t("admin.loadtest.perViewerLine", {
                    cpu: Math.round(report.perViewerCost?.cpuPercentAvg ?? 0),
                    mem: formatBytes(report.perViewerCost?.memUsedBytesAvg ?? 0),
                    disk: formatMbps(report.perViewerCost?.diskReadBytesPerSec ?? 0),
                    net: formatMbps(report.perViewerCost?.servedBytesPerSec ?? 0),
                  })}
                </p>
              )}
              {(report.sharedCacheViewers ?? 0) > 0 && (
                <p className="text-sm text-[var(--text-muted)]">
                  {t("admin.loadtest.sharedCacheLine", { count: report.sharedCacheViewers })}
                </p>
              )}
              {(report.skippedCandidates ?? 0) > 0 && (
                <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-skipped">
                  {t("admin.loadtest.skippedCandidatesLine", { count: report.skippedCandidates })}
                </p>
              )}
              {/* The verdict. When a resource gave out, THAT is the answer the
                  operator came for - the viewer count above is only its
                  consequence. When it didn't, say so plainly rather than let a
                  ceiling that was never reached pass for a capacity result. */}
              <p
                className={
                  report.stopReason && BOTTLENECK_REASONS.includes(report.stopReason)
                    ? "text-sm font-medium text-[var(--text)]"
                    : "text-sm text-[var(--text-muted)]"
                }
                data-testid="load-test-stop-reason"
              >
                {/* The verdict, and only the verdict. stopDetail is the server's
                    own diagnostic string - English, technical, and written for a
                    log line; every figure it carries is already on screen above,
                    translated. */}
                {t(
                  (report.stopReason && STOP_REASON_KEYS[report.stopReason]) ??
                    "admin.loadtest.stopReason.error",
                )}
              </p>
              {report.stopReason === "max_viewers" && (
                <p className="text-xs text-[var(--text-muted)]" data-testid="load-test-no-bottleneck">
                  {t("admin.loadtest.noBottleneckHint")}
                </p>
              )}
            </div>
          )}

          <div className="flex flex-col gap-2" data-testid="load-test-idle">
            <p className="text-sm text-[var(--text-muted)]">{t("admin.loadtest.idleDescription")}</p>
            <p className="text-sm text-[var(--text-muted)]" data-testid="load-test-no-ceiling">
              {t("admin.loadtest.noCeilingLine")}
            </p>
            <p className="text-xs text-[var(--color-warning)]">{t("admin.loadtest.idleWarning")}</p>

            {blockedByActiveStreams ? (
              <p className="text-sm text-[var(--color-danger)]" role="alert" data-testid="load-test-error">
                {t("admin.loadtest.errorActiveStreams")}
              </p>
            ) : startError ? (
              <p className="text-sm text-[var(--color-danger)]" role="alert" data-testid="load-test-error">
                {t("admin.loadtest.errorGeneric")}
              </p>
            ) : null}

            <div className="flex flex-wrap items-end gap-3">
              {/* Native <select> with <optgroup> - the shared Select component
                  doesn't support groups, and grouping "usage profile" (the
                  question that matters: how many different humans) from the
                  3 "pure" single-path lanes is the whole point here. */}
              <div className="flex flex-col gap-1 w-64">
                <label htmlFor="load-test-scenario" className="text-sm font-medium text-[var(--text-muted)]">
                  {t("admin.loadtest.scenarioLabel")}
                </label>
                <div className="relative">
                  <select
                    id="load-test-scenario"
                    value={scenario}
                    onChange={(e) => setScenario(e.target.value as LoadTestScenario)}
                    disabled={start.isPending}
                    data-testid="load-test-scenario-select"
                    className="w-full appearance-none px-3 py-2 pr-8 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] text-base sm:text-sm focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:border-transparent disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <optgroup label={t("admin.loadtest.scenarioGroupUsage")}>
                      <option value="light">{t("admin.loadtest.scenarioLight")}</option>
                      <option value="balanced">{t("admin.loadtest.scenarioBalanced")}</option>
                      <option value="heavy">{t("admin.loadtest.scenarioHeavy")}</option>
                    </optgroup>
                    <optgroup label={t("admin.loadtest.scenarioGroupPure")}>
                      <option value="transcode">{t("admin.loadtest.scenarioTranscode")}</option>
                      <option value="mkvgo">{t("admin.loadtest.scenarioMkvgo")}</option>
                      <option value="natural">{t("admin.loadtest.scenarioNatural")}</option>
                    </optgroup>
                  </select>
                  <ChevronDown
                    className="pointer-events-none absolute right-2 top-1/2 -translate-y-1/2 w-4 h-4 text-[var(--text-muted)]"
                    size={16}
                  />
                </div>
              </div>
              <Button
                loading={start.isPending}
                onClick={() => start.mutate()}
                data-testid="load-test-start-btn"
              >
                {t("admin.loadtest.startButton")}
              </Button>
            </div>
          </div>
        </>
      )}
    </div>
  );
}
export const Route = createLazyFileRoute("/_auth/admin/transcoding")({
  component: TranscodingConfigPage,
});

const SETTINGS_KEY = ["/api/v1/admin/server-settings"];

type SettingValue<T> = { value: T; default: T; source: string };
interface ServerSettings {
  maxConcurrentTranscodes: SettingValue<number>;
}

// ConcurrencyCard owns the max-simultaneous-transcodes policy: Auto (no
// override → the server's own default) or a manual cap. A weak NAS that can
// barely sustain one 4K transcode sets a low manual number; most hosts leave
// it on Auto.
function ConcurrencyCard() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data, isLoading } = useQuery<ServerSettings>({
    queryKey: SETTINGS_KEY,
    queryFn: () => eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "GET" }),
  });

  const [auto, setAuto] = useState(true);
  const [manual, setManual] = useState("");
  useEffect(() => {
    if (data) {
      const overridden = data.maxConcurrentTranscodes.source === "override";
      setAuto(!overridden);
      setManual(overridden ? String(data.maxConcurrentTranscodes.value) : "");
    }
  }, [data]);

  const save = useMutation({
    mutationFn: (body: Record<string, unknown>) =>
      eveyFetch<ServerSettings>({ url: "/api/v1/admin/server-settings", method: "PUT", data: body }),
    onSuccess: () => { qc.invalidateQueries({ queryKey: SETTINGS_KEY }); toast.success(t("admin.transcoding.saved", "Saved")); },
    onError: (err: Error) => toast.error(err.message),
  });

  if (isLoading || !data) return <div className="flex justify-center py-6"><Spinner size="md" /></div>;

  const defaultVal = data.maxConcurrentTranscodes.default;

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-3">
        <label className="text-sm font-medium text-[var(--text)]">
          {t("admin.transcoding.concurrencyAuto", "Detect automatically")}
        </label>
        <button
          type="button"
          role="switch"
          aria-checked={auto}
          aria-label={t("admin.transcoding.concurrencyAuto", "Detect automatically")}
          data-testid="max-concurrent-auto-toggle"
          onClick={() => setAuto((v) => !v)}
          className="relative inline-flex h-6 w-11 items-center rounded-full transition-colors"
          style={{ backgroundColor: auto ? "var(--accent)" : "var(--border)" }}
        >
          <span className="inline-block h-4 w-4 rounded-full bg-white transition-transform"
            style={{ transform: auto ? "translateX(1.375rem)" : "translateX(0.25rem)" }} />
        </button>
        <span className="text-xs text-[var(--text-muted)]">
          {auto
            ? t("admin.transcoding.concurrencyAutoHint", "Server default ({{n}})", { n: defaultVal })
            : t("admin.transcoding.concurrencyManualHint", "Cap concurrent transcodes")}
        </span>
      </div>

      {!auto && (
        <Input
          label={t("admin.serverSettings.maxConcurrentTranscodes", "Max simultaneous transcodes")}
          type="number"
          min={1}
          placeholder={String(defaultVal)}
          value={manual}
          onChange={(e) => setManual(e.target.value)}
          data-testid="max-concurrent-input"
        />
      )}

      <div className="flex justify-end">
        <Button
          data-testid="concurrency-save-btn"
          loading={save.isPending}
          onClick={() => {
            // Auto → clear the override (server falls back to its default).
            // Manual → persist the chosen cap.
            save.mutate({ maxConcurrentTranscodes: auto || manual === "" ? null : parseInt(manual, 10) });
          }}
        >
          {t("common.save")}
        </Button>
      </div>
    </div>
  );
}

// TranscodingConfigPage groups every transcoding-related admin control -
// hardware acceleration, encode calibration, AV1 and concurrency - on the
// server-config side of the admin (a sibling of "Server configuration"),
// out of the Media submenu where it used to live.
function TranscodingConfigPage() {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t("admin.transcoding.title", "Transcoding configuration")}
        subtitle={t("admin.transcoding.subtitle", "Hardware acceleration, encode calibration and codec policy")}
      />

      <CollapsibleCard
        title={t("admin.calibration.cardTitle", "Encode calibration")}
        subtitle={t(
          "admin.calibration.cardSubtitle",
          "Measured at boot - sets the cap on the HLS ladder so the host doesn't overshoot what it can encode in realtime.",
        )}
        alwaysOpen
      >
        <EncodeCalibrationCard />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.transcoding.livePlaybackTitle")}
        subtitle={t("admin.transcoding.livePlaybackSubtitle")}
        alwaysOpen
      >
        <VideoSessionsCard />
        <LoadTestCard />
      </CollapsibleCard>

      <CollapsibleCard
        title={t("admin.transcoding.jobConcurrencyTitle")}
        subtitle={t("admin.transcoding.jobConcurrencySubtitle")}
        alwaysOpen
      >
        <ConcurrencyCard />
      </CollapsibleCard>
    </div>
  );
}
