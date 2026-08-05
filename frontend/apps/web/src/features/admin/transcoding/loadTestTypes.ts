// --- LoadTestCard types --------------------------------------------------
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

// What the server was actually spending. Read from its own cgroup, so every
// ffmpeg it spawned is already counted.
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
  // coldCache: the run kept evicting the library from the page cache, so the
  // figures measure the STORAGE rather than the RAM. A warm and a cold run of
  // the same box are two different answers - never compare them as one.
  coldCache?: boolean;
  // diskReadCeilingBytesPerSec: how fast this box reads its media - every disk
  // the library spans, read at once. The kernel cannot report this when the
  // library sits on a network share or a 9p mount, so the run measures it itself
  // rather than printing a misleading zero.
  diskReadCeilingBytesPerSec?: number;
  // diskCeilingAtReaders / diskReadAtAudienceBytesPerSec: the ceiling above is
  // measured at whatever width the hardware likes (1 or 4 sequential readers on
  // spinning rust). An audience of 21 never sees it. Quoting the peak alone next
  // to the bytes actually read is how a saturated disk reads as 40% busy - so the
  // report quotes the curve at the width the audience imposes too.
  diskCeilingAtReaders?: number;
  diskReadAtAudienceBytesPerSec?: number;
  // tiers: one row per audience size the run measured - the climb itself. Every
  // cpu figure is the SERVER's share (the load generator's own overhead has been
  // subtracted), and memUsedBytes is anonymous memory, never the page cache.
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
  // harnessCpuFraction: how much of the measured CPU was the load GENERATOR, not
  // the server. Already subtracted from every figure above; shown so the operator
  // knows the correction happened.
  harnessCpuFraction?: number;
  harnessCpuMeasured?: boolean;
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
  // readAmplification: bytes READ from disk per byte SERVED. One is perfect;
  // four means three quarters of the storage bandwidth is spent on data the
  // server throws away - and on a direct-play box, read bandwidth IS the ceiling.
  readAmplification?: number;
  // appReadAmplification: what the SERVER asked for, per byte served. Set against
  // readAmplification (what the DISK delivered), it says whether the code is
  // greedy or the kernel is reading ahead - two problems with different fixes.
  appReadAmplification?: number;
  // endpoint: the origin the viewers actually hit (the reverse proxy when it was
  // reachable, the server's own loopback otherwise). Runs against different
  // endpoints are not comparable.
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
