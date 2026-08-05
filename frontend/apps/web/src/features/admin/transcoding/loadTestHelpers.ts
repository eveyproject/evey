import type { V1VideoPool } from "@evey/api";
import type { LoadTestScenario, LoadTestStopReason } from "./loadTestTypes";

export const LOAD_TEST_URL = "/api/v1/admin/load-test";
export const LOAD_TEST_QUERY_KEY = ["admin-load-test-status"];

// A POST refused because real playback is active surfaces as 409 (conflict),
// 423 (locked) or 500 (the admission guard erroring out defensively) - all
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

// A run that ended because a RESOURCE ran out has found the machine's real
// ceiling; anything else means the ramp was stopped by something other than
// capacity (the operator's own limit, the clock, an error).
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

// Noun form of each scenario for the report headline ("...in {{scenario}}.").
export const SCENARIO_NOUN_KEYS: Record<LoadTestScenario, string> = {
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

// predictedStreams reads the SAME calibration table VideoSessionsCard renders
// above (pools[].classes[].rungFits[]) and returns the median rung's
// max_streams for the ladder class the load test actually exercises: "hw"
// when a GPU pool exists, "software" otherwise - independent of which
// scenario the operator picked, since the pool a stream lands on is a
// hardware fact, not a test setting.
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
