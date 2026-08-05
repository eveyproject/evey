export function formatSeriesRuntime(totalMs?: string, episodeCount?: number): string {
  const ms = totalMs ? parseInt(totalMs, 10) : 0;
  const eps = episodeCount ?? 0;
  if (eps === 0 || isNaN(ms) || ms <= 0) return "";
  const totalMin = Math.floor(ms / 60000);
  const days = Math.floor(totalMin / (60 * 24));
  const h = Math.floor((totalMin % (60 * 24)) / 60);
  const m = totalMin % 60;
  if (days > 0) return `${days}d ${h}h`;
  if (h > 0) return m > 0 ? `${h}h ${m}min` : `${h}h`;
  return `${m}min`;
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
