// One formatter per user-facing quantity, shared by the three apps. The
// per-screen copies had drifted apart: decimal vs binary byte units, "min" vs
// "m" suffixes, French unit labels on one admin screen, and an unclamped clock
// on another. format.guard.test.ts keeps new copies from growing back.

const BYTE_UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

// Binary (1024) units; the server sends int64 sizes as strings.
export function formatBytes(bytes: number | string | null | undefined): string {
  const n = typeof bytes === "string" ? parseInt(bytes, 10) : (bytes ?? 0);
  if (!Number.isFinite(n) || n <= 0) return "0 B";
  const exponent = Math.min(Math.floor(Math.log(n) / Math.log(1024)), BYTE_UNITS.length - 1);
  const value = n / Math.pow(1024, exponent);
  return `${value.toFixed(exponent > 0 ? 1 : 0)} ${BYTE_UNITS[exponent]}`;
}

// Player/track clock: "m:ss", or "h:mm:ss" from one hour up.
export function formatTime(seconds: number): string {
  const s = Number.isFinite(seconds) ? Math.max(0, Math.floor(seconds)) : 0;
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
  return `${m}:${String(sec).padStart(2, "0")}`;
}

// Same clock from milliseconds - positions and durations arrive in ms.
export function formatTimeMs(ms: number): string {
  return formatTime(ms / 1000);
}

function humanMinutes(totalMin: number): string {
  if (totalMin < 60) return `${totalMin}min`;
  const h = Math.floor(totalMin / 60);
  const m = totalMin % 60;
  return m > 0 ? `${h}h ${m}min` : `${h}h`;
}

// Human runtime ("45min", "1h 32min", "2h") from milliseconds. Empty when the
// value is missing, unparseable or zero: series carry durationMs="0" because
// only their episodes have a runtime, and a "0min" badge is worse than none.
export function formatDurationMs(ms: number | string | null | undefined): string {
  const value = typeof ms === "string" ? Number(ms) : (ms ?? 0);
  if (!Number.isFinite(value) || value <= 0) return "";
  return humanMinutes(Math.floor(value / 60000));
}

// Same rendering from seconds (analytics totals). Callers that need a value
// for zero append `|| "0min"`.
export function formatDurationSec(seconds: number | string | null | undefined): string {
  const value = typeof seconds === "string" ? Number(seconds) : (seconds ?? 0);
  if (!Number.isFinite(value) || value <= 0) return "";
  return humanMinutes(Math.floor(value / 60));
}
