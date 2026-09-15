import type { V1VideoPool } from "@evey/api";

// loadTest.ts mirrors the web admin's loadTestTypes.ts + loadTestHelpers.ts
// (frontend/apps/web/src/features/admin/transcoding/) 1:1 so the mobile
// Load-test section talks to the same POST/GET/DELETE /api/v1/admin/load-test
// contract with the same wire shape and the same i18n keys.

// The 3 "pure" scenarios exercise a single playback path; the 3 usage profiles
// mix mkvgo direct-play and transcode viewers in a fixed ratio.
export type LoadTestScenario = "transcode" | "natural" | "mkvgo" | "light" | "balanced" | "heavy";

export type LoadTestStopReason =
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

export interface LoadTestResources {
  cpuPercentAvg?: number;
  cpuPercentPeak?: number;
  memUsedBytesAvg?: number;
  memUsedBytesMax?: number;
  memLimitBytes?: number;
  diskReadBytesPerSec?: number;
  servedBytesPerSec?: number;
  samples?: number;
}

export interface LoadTestReport {
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
  skippedCandidates?: number;
  viewerMix?: Record<string, number>;
  segmentMsP95ByType?: Record<string, number>;
  segmentSloMsByType?: Record<string, number>;
  resources?: LoadTestResources;
  perViewerCost?: LoadTestResources;
  resourceBaseline?: LoadTestResources;
  throughputCeilingBytesPerSec?: number;
  diskReadStarved?: boolean;
  coldCache?: boolean;
  storageFsType?: string;
  storageOpaque?: boolean;
  diskReadCeilingBytesPerSec?: number;
  diskCeilingAtReaders?: number;
  diskReadAtAudienceBytesPerSec?: number;
  tiers?: {
    viewers?: number;
    cpuPercent?: number;
    memUsedBytes?: number;
    diskReadBytesPerSec?: number;
    servedBytesPerSec?: number;
    cpuPercentPerViewer?: number;
    memBytesPerViewer?: number;
    samples?: number;
  }[];
  harnessCpuFraction?: number;
  harnessCpuMeasured?: boolean;
  slowestDiskBytesPerSec?: number;
  disksProbed?: number;
  storageCollapsesAtReaders?: number;
  readAmplification?: number;
  appReadAmplification?: number;
  endpoint?: string;
  viaProxy?: boolean;
}

export interface LoadTestStatus {
  running?: boolean;
  phase?: string;
  viewers?: number;
  elapsedMs?: number;
  lastReport?: LoadTestReport;
}

export const LOAD_TEST_URL = "/api/v1/admin/load-test";
export const LOAD_TEST_QUERY_KEY = ["admin-load-test-status"];

// A POST refused because real playback is active surfaces as 409/423/500 - all
// three read the same to an operator: "something is already streaming".
export const ACTIVE_STREAMS_STATUSES = [409, 423, 500];

export const STOP_REASON_KEYS: Partial<Record<LoadTestStopReason, string>> = {
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

// A run that ended because a RESOURCE ran out found the machine's real ceiling;
// anything else means the ramp was stopped by something other than capacity.
export const BOTTLENECK_REASONS: LoadTestStopReason[] = [
  "cpu_saturated",
  "memory_saturated",
  "throughput_saturated",
  "startup_too_slow",
  "server_failing",
  "slo_breach",
  "admission_full",
];

// Throughput is quoted in bits per second, the way a network link is.
export function formatMbps(bytesPerSec: number): string {
  return `${((bytesPerSec * 8) / 1e6).toFixed(1)} Mb/s`;
}

export const SCENARIO_NOUN_KEYS: Record<LoadTestScenario, string> = {
  transcode: "admin.loadtest.scenarioTranscodeNoun",
  natural: "admin.loadtest.scenarioNaturalNoun",
  mkvgo: "admin.loadtest.scenarioMkvgoNoun",
  light: "admin.loadtest.scenarioLightNoun",
  balanced: "admin.loadtest.scenarioBalancedNoun",
  heavy: "admin.loadtest.scenarioHeavyNoun",
};

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
// present in the record, joined into one discreet line.
export function mixParts(
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

export function formatElapsed(ms: number): string {
  const totalSec = Math.max(0, Math.round(ms / 1000));
  const m = Math.floor(totalSec / 60);
  const s = totalSec % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

// predictedStreams reads the SAME calibration table the capacity card renders
// (pools[].classes[].rungFits[]) and returns the median rung's maxStreams for
// the ladder class the load test exercises: "hw" when a GPU pool exists,
// "software" otherwise.
export function predictedStreams(pools: V1VideoPool[]): number | undefined {
  const gpuPool = pools.find((p) => p.pool === "gpu");
  const pool = gpuPool ?? pools.find((p) => p.pool === "cpu");
  const classes = pool?.classes ?? [];
  const wanted = gpuPool ? "hw" : "software";
  const cls = classes.find((c) => c.ladderClass === wanted) ?? classes[0];
  const rungs = cls?.rungFits ?? [];
  if (rungs.length === 0) return undefined;
  return rungs[Math.floor(rungs.length / 2)]?.maxStreams;
}
