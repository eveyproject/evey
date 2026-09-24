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

// Shared with the phone: packages/client/src/lib/mediaFile.ts.
export { hasMediaFile } from "@evey/client";
