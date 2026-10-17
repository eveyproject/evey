import { useEffect, useRef, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { useAdminSystemServiceGetDashboard, useAdminSystemServiceGetSystemInfo, formatBytes, eveyFetch } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { PageHeader } from "@/components/ui/PageHeader";
import { BackgroundTasksPanel } from "@/features/admin/BackgroundTasksPanel";
import { DiskIOList } from "@/features/admin/DiskIO";
import { SystemTrends } from "@/features/admin/SystemTrends";
import { DemoReadOnly } from "@/components/layout/DemoReadOnly";
import type { V1SystemInfo } from "@evey/api";

export const Route = createLazyFileRoute("/_auth/admin/system-health")({
  // Every control here is an operator write the demo refuses (DemoReadOnly).
  component: () => (
    <DemoReadOnly>
      <SystemHealthPage />
    </DemoReadOnly>
  ),
});

const MAX_HISTORY = 30;

function formatUptime(seconds: string | undefined): string {
  const s = parseInt(seconds ?? "0");
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s % 60}s`;
  return `${s}s`;
}

function formatBps(bps: number): string {
  if (bps >= 1e9) return `${(bps / 1e9).toFixed(2)} Gbps`;
  if (bps >= 1e6) return `${(bps / 1e6).toFixed(1)} Mbps`;
  if (bps >= 1e3) return `${(bps / 1e3).toFixed(0)} kbps`;
  return `${Math.max(0, bps)} bps`;
}

function getUsageColor(pct: number): string {
  if (pct > 85) return "var(--color-danger)";
  if (pct > 70) return "var(--color-warning)";
  return "var(--accent)";
}

function getUsageTextColor(pct: number): string {
  if (pct > 85) return "text-[var(--color-danger)]";
  if (pct > 70) return "text-[var(--color-warning)]";
  return "text-[var(--color-success)]";
}

function Sparkline({ values, color }: { values: number[]; color: string }) {
  if (values.length < 2) return null;
  const w = 120;
  const h = 32;
  const max = Math.max(...values, 1);
  const pts = values.map((v, i) => {
    const x = (i / (values.length - 1)) * w;
    const y = h - (v / max) * h;
    return `${x},${y}`;
  });
  return (
    <svg width={w} height={h} viewBox={`0 0 ${w} ${h}`} className="opacity-80">
      <polyline
        points={pts.join(" ")}
        fill="none"
        stroke={color}
        strokeWidth={1.5}
        strokeLinejoin="round"
        strokeLinecap="round"
      />
    </svg>
  );
}

interface MetricCardProps {
  label: string;
  value: number;
  history: number[];
  // A quieter second fact under the gauge ("of which 12 % waiting on disks").
  sub?: string;
}

function MetricCard({ label, value, history, sub }: MetricCardProps) {
  const pct = Math.min(100, Math.max(0, value));
  const color = getUsageColor(pct);
  const textClass = getUsageTextColor(pct);
  return (
    <div
      className="p-4 rounded-[var(--radius-lg)] flex flex-col gap-3 bg-[var(--surface)] border border-[var(--border)]"
    >
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">
          {label}
        </span>
        <span className={`text-xl font-bold ${textClass}`}>{pct.toFixed(1)}%</span>
      </div>
      <div
        className="h-2 rounded-full bg-[var(--border)]"
        role="progressbar"
        aria-label={label}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(pct)}
        aria-valuetext={`${pct.toFixed(1)}%`}
      >
        <div
          className="h-2 rounded-full transition-all duration-500"
          style={{ width: `${pct}%`, backgroundColor: color }}
        />
      </div>
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs text-[var(--text-muted)]">{sub ?? ""}</span>
        <Sparkline values={history} color={color} />
      </div>
    </div>
  );
}

interface BandwidthCardProps {
  bps: number; // measured streaming rate; -1 = not sampled yet
  theoreticalBps: number; // summed source bitrate of active sessions
  totalBytes: string | undefined; // served since server start
  history: number[];
}

function BandwidthCard({ bps, theoreticalBps, totalBytes, history }: BandwidthCardProps) {
  const { t } = useTranslation();
  return (
    <div
      className="p-4 rounded-[var(--radius-lg)] flex flex-col gap-3 bg-[var(--surface)] border border-[var(--border)]"
      data-testid="bandwidth-card"
    >
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">
          {t("admin.systemHealth.bandwidth")}
        </span>
        <span className="text-xl font-bold text-[var(--text)]">
          {bps >= 0 ? formatBps(bps) : "-"}
        </span>
      </div>
      <div className="flex justify-between gap-2 text-xs text-[var(--text-muted)]">
        <span>{t("admin.systemHealth.bandwidthTheoretical", { rate: formatBps(theoreticalBps) })}</span>
        <span>{t("admin.systemHealth.bandwidthServed", { size: formatBytes(totalBytes) })}</span>
      </div>
      <div className="flex justify-end">
        <Sparkline values={history} color="var(--accent)" />
      </div>
    </div>
  );
}

interface StorageCardProps {
  path: string;
  paths?: string[];
  usedBytes: string | undefined;
  totalBytes: string | undefined;
  freeBytes: string | undefined;
}

function StorageCard({ path, paths, usedBytes, totalBytes, freeBytes }: StorageCardProps) {
  const { t } = useTranslation();
  const used = parseInt(usedBytes ?? "0");
  const total = parseInt(totalBytes ?? "1");
  const pct = total > 0 ? Math.round((used / total) * 100) : 0;
  const color = getUsageColor(pct);
  const textClass = getUsageTextColor(pct);
  // One card per physical disk. When several libraries live on the same drive
  // the backend groups them, so list every path that shares this disk.
  const displayPaths = paths && paths.length > 0 ? paths : [path];
  return (
    <div
      className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
    >
      <div className="flex justify-between items-start mb-2 gap-2">
        <div className="flex flex-col gap-0.5 min-w-0">
          {displayPaths.map((p) => (
            <span key={p} className="text-sm font-mono truncate text-[var(--text)]">
              {p}
            </span>
          ))}
        </div>
        <span className={`text-sm font-bold whitespace-nowrap ${textClass}`}>{pct}%</span>
      </div>
      <div
        className="h-2 rounded-full mb-2 bg-[var(--border)]"
        role="progressbar"
        aria-label={displayPaths.join(", ")}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={pct}
        aria-valuetext={`${pct}%`}
      >
        <div
          className="h-2 rounded-full transition-all"
          style={{ width: `${pct}%`, backgroundColor: color }}
        />
      </div>
      <div className="flex justify-between text-xs text-[var(--text-muted)]">
        <span>{t("admin.systemHealth.used", { size: formatBytes(usedBytes) })}</span>
        <span>{t("admin.systemHealth.free", { size: formatBytes(freeBytes) })}</span>
        <span>{t("admin.systemHealth.total", { size: formatBytes(totalBytes) })}</span>
      </div>
    </div>
  );
}

function InfoRow({ label, value }: { label: string; value: string | undefined }) {
  if (!value) return null;
  return (
    <div>
      <dt className="text-xs font-medium uppercase tracking-wide mb-0.5 text-[var(--text-muted)]">{label}</dt>
      <dd className="text-sm font-mono text-[var(--text)]">{value}</dd>
    </div>
  );
}

type VideoSessionCapacity = NonNullable<V1SystemInfo["videoSessions"]>;
type VideoCapacityEvidence = NonNullable<VideoSessionCapacity["evidence"]>;

function VideoCapacityEvidenceRow({ evidence }: { evidence: VideoCapacityEvidence | undefined }) {
  const { t } = useTranslation();
  if (!evidence) return null;
  if (evidence.derived === false) {
    return (
      <p className="text-xs text-[var(--color-warning)]">
        {t("admin.systemHealth.videoCapacityNotMeasured")}
      </p>
    );
  }
  return (
    <p className="text-xs text-[var(--text-muted)]">
      {t("admin.systemHealth.videoCapacityEvidence", {
        height: evidence.height ?? 0,
        fps: (evidence.fps ?? 0).toFixed(1),
        requiredFps: (evidence.requiredFps ?? 0).toFixed(1),
        cpuBudget: evidence.cpuBudget ?? 0,
      })}
    </p>
  );
}

function VideoCapacityCard({ capacity }: { capacity: VideoSessionCapacity }) {
  const { t } = useTranslation();
  const active = capacity.active ?? 0;
  const limit = capacity.effectiveLimit ?? 0;
  const auto = capacity.autoCapacity ?? 0;
  const reduced = limit < auto;
  return (
    <div
      className="p-4 rounded-[var(--radius-lg)] flex flex-col gap-2 bg-[var(--surface)] border border-[var(--border)]"
      data-testid="video-capacity-card"
    >
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">
          {t("admin.systemHealth.videoCapacityTitle")}
        </span>
        <span className="text-xl font-bold text-[var(--text)]">
          {t("admin.systemHealth.videoCapacityRatio", { active, limit })}
        </span>
      </div>
      {reduced && (
        <p className="text-xs text-[var(--text-muted)]">
          {t("admin.systemHealth.videoCapacityAutoNote", { auto })}
        </p>
      )}
      <VideoCapacityEvidenceRow evidence={capacity.evidence} />
      <p className="text-xs text-[var(--text-muted)]">
        {t("admin.systemHealth.videoCapacityHint")}
      </p>
    </div>
  );
}

function ServerInfoPanel({ info }: { info: V1SystemInfo }) {
  const { t } = useTranslation();
  const devices = info.hwDevices ?? [];
  return (
    <div
      className="p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
    >
      <h3 className="text-sm font-semibold mb-4 text-[var(--text)]">{t("admin.systemHealth.serverInfo")}</h3>
      <dl className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-x-6 gap-y-4">
        <InfoRow label={t("admin.dashboard.version")} value={info.version} />
        <InfoRow label={t("admin.dashboard.os")} value={info.os && info.arch ? `${info.os}/${info.arch}` : info.os} />
        <InfoRow label="Go" value={info.goVersion} />
        <InfoRow label={t("admin.dashboard.ffmpeg")} value={info.ffmpegVersion} />
        <InfoRow label={t("admin.dashboard.uptime")} value={formatUptime(info.uptimeSeconds)} />
        <InfoRow label={t("admin.systemHealth.serverId")} value={info.serverId} />
      </dl>
      {devices.length > 0 && (
        <div className="mt-4">
          <p className="text-xs font-semibold uppercase tracking-wide mb-2 text-[var(--text-muted)]">
            {t("admin.systemHealth.hwAcceleration")}
          </p>
          <div className="flex flex-wrap gap-2">
            {devices.map((d, i) => (
              <span
                key={i}
                className="inline-flex items-center gap-1 px-2 py-1 rounded text-xs font-medium bg-[var(--surface-raised)] text-[var(--accent)]"
              >
                {d.type?.replace("HW_ACCEL_TYPE_", "") ?? "?"} - {d.name ?? "unknown"}
              </span>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

// SystemMetrics mirrors systemMetrics in the Go handler behind
// /api/v1/admin/system/metrics: the finer facts under the two percentages.
interface SystemMetrics {
  load: { avg1: number; avg5: number; avg15: number; known: boolean; cpus: number; cpuBudget: number };
  memory: { totalBytes: number; availableBytes: number; budgetBytes: number };
  process: { rssBytes: number; heapInUseBytes: number; heapSysBytes: number; goroutines: number; gcCount: number; gcPauseTotalNs: number };
  forks: {
    count: number;
    rssBytes: number;
    cpuPercent: number;
    byClass: { class: string; count: number; rssBytes: number; cpuPercent: number }[];
  };
}

function FineCard({ title, value, lines, testId }: { title: string; value: string; lines: string[]; testId: string }) {
  return (
    <div
      className="p-4 rounded-[var(--radius-lg)] flex flex-col gap-2 bg-[var(--surface)] border border-[var(--border)]"
      data-testid={testId}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">{title}</span>
        <span className="text-xl font-bold tabular-nums text-[var(--text)]">{value}</span>
      </div>
      {lines.map((l) => (
        <span key={l} className="text-xs text-[var(--text-muted)]">{l}</span>
      ))}
    </div>
  );
}

// LoadCard reads the load average against the CPUs Evey may actually use: a
// load of 6 is a calm number on 8 cores and a saturated one on a 2-CPU cgroup.
function LoadCard({ load }: { load: SystemMetrics["load"] }) {
  const { t } = useTranslation();
  const budget = load.cpuBudget > 0 ? load.cpuBudget : load.cpus;
  const pct = budget > 0 ? (load.avg1 / budget) * 100 : 0;
  return (
    <div
      className="p-4 rounded-[var(--radius-lg)] flex flex-col gap-2 bg-[var(--surface)] border border-[var(--border)]"
      data-testid="load-card"
    >
      <div className="flex items-center justify-between gap-2">
        <span className="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)]">{t("admin.systemHealth.load")}</span>
        <span className={`text-xl font-bold tabular-nums ${load.known ? getUsageTextColor(pct) : "text-[var(--text-muted)]"}`}>
          {load.known ? `${load.avg1.toFixed(2)} / ${load.avg5.toFixed(2)} / ${load.avg15.toFixed(2)}` : "-"}
        </span>
      </div>
      <span className="text-xs text-[var(--text-muted)]">{t("admin.systemHealth.loadCpus", { budget })}</span>
    </div>
  );
}

function ProcessCard({ process }: { process: SystemMetrics["process"] }) {
  const { t } = useTranslation();
  return (
    <FineCard
      testId="process-card"
      title={t("admin.systemHealth.process")}
      value={formatBytes(process.rssBytes)}
      lines={[
        `${t("admin.systemHealth.heap", { size: formatBytes(process.heapInUseBytes) })} · ${t("admin.systemHealth.goroutines", { count: process.goroutines })}`,
        t("admin.systemHealth.gcCount", { count: process.gcCount }),
      ]}
    />
  );
}

// ForksCard is where the CPU goes when it is not Evey itself: the ffmpeg,
// ffprobe and fpcalc processes in flight, by class (a viewer's transcode is
// "production", a library sweep is "scan").
function ForksCard({ forks }: { forks: SystemMetrics["forks"] }) {
  const { t } = useTranslation();
  const lines = forks.count === 0
    ? [t("admin.systemHealth.forksNone")]
    : forks.byClass.map((g) =>
        `${t(`admin.systemHealth.forkClass.${g.class}`, g.class)}: ${t("admin.systemHealth.forksLine", { count: g.count, memory: formatBytes(g.rssBytes), cpu: Math.round(g.cpuPercent) })}`,
      );
  return (
    <FineCard
      testId="forks-card"
      title={t("admin.systemHealth.forks")}
      value={forks.count === 0 ? "0" : t("admin.systemHealth.forksLine", { count: forks.count, memory: formatBytes(forks.rssBytes), cpu: Math.round(forks.cpuPercent) })}
      lines={lines}
    />
  );
}

function SystemHealthPage() {
  const { t } = useTranslation();
  const { data: dashData, isLoading: dashLoading, dataUpdatedAt } = useAdminSystemServiceGetDashboard({
    query: { refetchInterval: 5_000, refetchIntervalInBackground: false },
  });
  const { data: sysData, isLoading: sysLoading } = useAdminSystemServiceGetSystemInfo();
  // Pure in-process reads on the server: safe to poll as often as the
  // dashboard numbers.
  const { data: metrics } = useQuery<SystemMetrics>({
    queryKey: ["/api/v1/admin/system/metrics"],
    queryFn: () => eveyFetch<SystemMetrics>({ url: "/api/v1/admin/system/metrics", method: "GET" }),
    refetchInterval: 5_000,
    refetchIntervalInBackground: false,
  });

  const cpuHistory = useRef<number[]>([]);
  const memHistory = useRef<number[]>([]);
  const bwHistory = useRef<number[]>([]);
  const [, forceRender] = useState(0);

  useEffect(() => {
    if (dashData) {
      cpuHistory.current = [...cpuHistory.current, dashData.cpuPercent ?? 0].slice(-MAX_HISTORY);
      memHistory.current = [...memHistory.current, dashData.memoryPercent ?? 0].slice(-MAX_HISTORY);
      const bps = Number(dashData.streamingBps ?? -1);
      bwHistory.current = [...bwHistory.current, Math.max(0, bps)].slice(-MAX_HISTORY);
      forceRender((n) => n + 1);
    }
  }, [dashData]);

  const isLoading = dashLoading || sysLoading;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title={t("admin.systemHealth.title")}
        subtitle={dataUpdatedAt > 0 ? t("admin.systemHealth.autoRefresh", { time: new Date(dataUpdatedAt).toLocaleTimeString() }) : undefined}
      />

      {isLoading ? (
        <div className="flex justify-center py-16"><Spinner size="lg" /></div>
      ) : (
        <>
          {dashData && (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              <MetricCard
                label={t("admin.systemHealth.cpuUsage")}
                value={dashData.cpuPercent ?? 0}
                history={cpuHistory.current}
                sub={t("admin.systemHealth.iowaitSub", { percent: (dashData.iowaitPercent ?? 0).toFixed(1) })}
              />
              <MetricCard
                label={t("admin.systemHealth.memoryUsage")}
                value={dashData.memoryPercent ?? 0}
                history={memHistory.current}
              />
              <BandwidthCard
                bps={Number(dashData.streamingBps ?? -1)}
                theoreticalBps={Number(dashData.activeSessionsBps ?? 0)}
                totalBytes={dashData.streamingBytesTotal}
                history={bwHistory.current}
              />
            </div>
          )}

          {metrics && (
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
              <LoadCard load={metrics.load} />
              <ProcessCard process={metrics.process} />
              <ForksCard forks={metrics.forks} />
            </div>
          )}

          {dashData?.disks && dashData.disks.length > 0 && (
            <div>
              <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
                {t("admin.systemHealth.disksSection")}
              </h2>
              <DiskIOList
                disks={dashData.disks}
                idleAwaitMs={dashData.idleDiskAwaitMs}
                saturationAwaitMs={dashData.saturationAwaitMs}
                iowaitPercent={dashData.iowaitPercent}
              />
            </div>
          )}

          <div>
            <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
              {t("admin.systemHealth.trendsSection")}
            </h2>
            <SystemTrends />
          </div>

          <div>
            <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
              {t("admin.systemHealth.backgroundTasksSection")}
            </h2>
            <BackgroundTasksPanel showCoverage />
          </div>

          {dashData?.storage && dashData.storage.length > 0 && (
            <div>
              <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
                {t("admin.systemHealth.storage")}
              </h2>
              <div className="flex flex-col gap-3">
                {dashData.storage.map((s, i) => (
                  <StorageCard
                    key={i}
                    path={s.path ?? ""}
                    paths={s.paths}
                    usedBytes={s.usedBytes}
                    totalBytes={s.totalBytes}
                    freeBytes={s.freeBytes}
                  />
                ))}
              </div>
            </div>
          )}

          {sysData?.videoSessions && (
            <div>
              <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
                {t("admin.systemHealth.videoCapacitySection")}
              </h2>
              <VideoCapacityCard capacity={sysData.videoSessions} />
            </div>
          )}

          {sysData && (
            <div>
              <h2 className="text-sm font-semibold uppercase tracking-wide mb-3 text-[var(--text-muted)]">
                {t("admin.systemHealth.server")}
              </h2>
              <ServerInfoPanel info={sysData} />
            </div>
          )}
        </>
      )}
    </div>
  );
}
