// Client-side mirror of the Go log classifier in
// internal/adapters/driving/http/log_classify.go. The search endpoint returns
// already-classified entries, so this is only used for the LIVE tail (SSE),
// which streams raw slog JSON lines. Keep the two taxonomies in sync.

export interface LogEntry {
  // Stable identity, assigned once when the line is parsed. React keys MUST use
  // this and never the array index: live tail prepends each new line, which
  // shifts every index, changes every key, and remounts every row - collapsing
  // any entry the operator had expanded to read.
  uid: string;
  time: string;
  level: string;
  msg: string;
  component?: string;
  where: string;
  source: string;
  origin: string;
  requestId?: string;
  traceId?: string;
  mediaId?: string;
  sessionId?: string;
  userId?: string;
  fields?: Record<string, unknown>;
  raw: string;
}

export interface LogSearchResponse {
  entries: LogEntry[];
  total: number;
  scanned: number;
  truncated: boolean;
}

export const WHERE_VALUES = [
  "player",
  "seek-index",
  "subtitle",
  "remux",
  "hw-detect",
  "intro",
  "trickplay",
  "qoe",
  "chromecast",
  "auth",
  "infra",
  "other",
] as const;

export const SOURCE_VALUES = ["ffmpeg", "mkvgo", "qoe", "grpc", "client", "other"] as const;

// Components whose work really is ffmpeg end to end. keyframe_backfill_worker
// is deliberately ABSENT: it is a MIXED component - its frame-rate pass is pure
// mkvgo (an in-process structural walk, no fork) and its keyframe pass picks
// mkvgo or ffprobe at runtime. Blanket-badging it "ffmpeg" was simply wrong, so
// it now relies on the `engine` field the server stamps on those lines.
const FFMPEG_COMPONENTS = new Set([
  "software_producer",
  "segment_orchestrator",
  "audio_segment_orchestrator",
  "level_backfill_worker",
  "complexity_worker",
]);
const MKVGO_COMPONENTS = new Set(["seek_index", "seek-index"]);

function hasAnyPrefix(s: string, ...prefixes: string[]): boolean {
  return prefixes.some((p) => s.startsWith(p));
}

// classifySource names the engine behind a line. `engine`, when the server
// stamped it, is the AUTHORITY: only the code that ran knows whether mkvgo's
// fast path or the ffprobe fallback served the call, and no amount of guessing
// from component names can recover that. The heuristics below are the fallback
// for the lines that do not carry it yet.
export function classifySource(component: string, msg: string, engine?: string): string {
  if (engine === "mkvgo" || engine === "ffmpeg") return engine;
  if (FFMPEG_COMPONENTS.has(component)) return "ffmpeg";
  if (MKVGO_COMPONENTS.has(component)) return "mkvgo";
  if (component === "qoe_sink") return "qoe";
  if (component === "grpc") return "grpc";
  // Reported BY the browser player, not produced by a server engine. It was
  // landing in "other", hiding a whole class of client-side playback errors.
  if (component === "player_client") return "client";
  const m = msg.toLowerCase();
  if (hasAnyPrefix(m, "hls:", "software:", "transcode:", "transcoder", "ffmpeg", "hwdetect:", "benchmark:", "av1 decoder probe", "no av1 decoder")) return "ffmpeg";
  if (m.includes("mkvgo") || m.startsWith("seek-index")) return "mkvgo";
  return "other";
}

export function classifyWhere(component: string, msg: string): string {
  switch (component) {
    case "software_producer":
    case "segment_orchestrator":
    case "audio_segment_orchestrator":
      return "player";
    case "keyframe_backfill_worker":
    case "level_backfill_worker":
    case "complexity_worker":
    case "remux_cache_evict":
    case "segment_cache_evict":
      return "infra";
    case "seek_index":
    case "seek-index":
      return "seek-index";
    case "qoe_sink":
      return "qoe";
    case "scheduler":
    case "webhook":
    case "backup-scheduler":
    case "grpc":
    case "watcher":
    case "metrics_sampler":
      return "infra";
  }
  const m = msg.toLowerCase();
  if (hasAnyPrefix(m, "hls:", "software:", "transcode:")) return "player";
  if (m.startsWith("subtitle")) return "subtitle";
  if (m.startsWith("remux")) return "remux";
  if (m.startsWith("seek-index") || m.includes("mkvgo")) return "seek-index";
  if (hasAnyPrefix(m, "hwdetect:", "benchmark:", "av1 decoder probe", "no av1 decoder")) return "hw-detect";
  if (m.includes("intro_detection") || m.startsWith("intro")) return "intro";
  if (m.includes("trickplay")) return "trickplay";
  if (hasAnyPrefix(m, "chromecast", "devices command")) return "chromecast";
  if (hasAnyPrefix(m, "oidc:", "csrf") || m.includes("token reuse") || m.startsWith("stream token")) return "auth";
  return "other";
}

// isAccessLog tells the gRPC interceptor's per-call access line of a call that
// SUCCEEDED apart from everything else. On a live instance these are four
// lines in five (heartbeats, dashboard polls) and bury the application log;
// failed calls are not access noise and stay. Mirrors isAccessLog in Go.
export function isAccessLog(e: LogEntry): boolean {
  if (e.msg !== "grpc unary" && e.msg !== "grpc stream") return false;
  return e.fields?.status === "OK";
}

function str(rec: Record<string, unknown>, key: string): string {
  const v = rec[key];
  return typeof v === "string" ? v : "";
}

function first(rec: Record<string, unknown>, ...keys: string[]): string {
  for (const k of keys) {
    const v = str(rec, k);
    if (v) return v;
  }
  return "";
}

const PROMOTED = new Set(["time", "level", "msg", "component", "request_id", "trace_id", "engine"]);

// parseLogLine turns a raw slog JSON line into a classified LogEntry, mirroring
// the Go parseLogLine. Returns null for non-JSON lines.
// Monotonic per session: only uniqueness matters, not the value.
let logEntrySeq = 0;

export function parseLogLine(line: string): LogEntry | null {
  let rec: Record<string, unknown>;
  try {
    rec = JSON.parse(line) as Record<string, unknown>;
  } catch {
    return null;
  }
  if (typeof rec !== "object" || rec === null) return null;

  const component = str(rec, "component");
  const msg = str(rec, "msg");
  const fields: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(rec)) {
    if (!PROMOTED.has(k)) fields[k] = v;
  }
  logEntrySeq += 1;
  return {
    uid: `live-${logEntrySeq}`,
    time: str(rec, "time"),
    level: str(rec, "level").toUpperCase(),
    msg,
    component: component || undefined,
    where: classifyWhere(component, msg),
    source: classifySource(component, msg, str(rec, "engine")),
    origin: "server",
    requestId: str(rec, "request_id") || undefined,
    traceId: str(rec, "trace_id") || undefined,
    mediaId: first(rec, "media_id", "mediaId", "mediaID") || undefined,
    sessionId: first(rec, "session_id", "sessionID") || undefined,
    userId: first(rec, "user_id", "userID") || undefined,
    fields: Object.keys(fields).length > 0 ? fields : undefined,
    raw: line,
  };
}
