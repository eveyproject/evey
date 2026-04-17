export function formatDuration(ms?: string): string {
  if (!ms) return "";
  const totalMs = parseInt(ms, 10);
  // Series carry durationMs="0" because only their episodes have a runtime.
  // Return empty so the UI doesn't show a meaningless "0m" badge.
  if (isNaN(totalMs) || totalMs <= 0) return "";
  const totalSec = Math.floor(totalMs / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export function formatSeriesRuntime(totalMs?: string, episodeCount?: number): string {
  const ms = totalMs ? parseInt(totalMs, 10) : 0;
  const eps = episodeCount ?? 0;
  if (eps === 0 || isNaN(ms) || ms <= 0) return "";
  const totalMin = Math.floor(ms / 60000);
  const days = Math.floor(totalMin / (60 * 24));
  const h = Math.floor((totalMin % (60 * 24)) / 60);
  const m = totalMin % 60;
  if (days > 0) return `${days}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

export function formatFileSize(bytes?: string): string {
  if (!bytes) return "";
  const n = parseInt(bytes, 10);
  if (isNaN(n)) return "";
  if (n >= 1_073_741_824) return `${(n / 1_073_741_824).toFixed(1)} GB`;
  if (n >= 1_048_576) return `${(n / 1_048_576).toFixed(0)} MB`;
  return `${(n / 1024).toFixed(0)} KB`;
}
