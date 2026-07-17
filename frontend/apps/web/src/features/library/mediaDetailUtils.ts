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

// hasMediaFile reports whether an item is a real, downloadable single media file
// (a movie or episode) rather than a container (series/season). It must NOT gate
// on item.filePath: the absolute server path is redacted for non-admins by design
// (mediaItemToProto's includeFilePath, pentest 2026-05-26), so a filePath check
// would hide the download button - and even suppress the stream-token fetch - for
// every non-admin viewer. fileSize IS emitted to every viewer, so it's the signal
// available to all. Guard on a REAL numeric size: proto EmitUnpopulated sends a
// container's zero as the STRING "0", which is truthy in JS, so `!!item.fileSize`
// would wrongly treat a series/season as a file. filePath stays as a belt-and-
// suspenders OR so admins keep the button even in any edge case where size is 0.
export function hasMediaFile(item: { fileSize?: string; filePath?: string }): boolean {
  return Number(item.fileSize ?? 0) > 0 || !!item.filePath;
}

export function formatFileSize(bytes?: string): string {
  if (!bytes) return "";
  const n = parseInt(bytes, 10);
  if (isNaN(n)) return "";
  if (n >= 1_073_741_824) return `${(n / 1_073_741_824).toFixed(1)} GB`;
  if (n >= 1_048_576) return `${(n / 1_048_576).toFixed(0)} MB`;
  return `${(n / 1024).toFixed(0)} KB`;
}
