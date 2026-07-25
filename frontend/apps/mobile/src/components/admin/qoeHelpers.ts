// Pure helpers extracted from the QoE admin screen so the percent/duration/
// health-bucket logic can be unit-tested without mounting react-native
// components or hitting the network. Mirrors the web dashboard
// (frontend/apps/web/src/routes/_auth/admin/qoe.lazy.tsx) so the two clients
// agree on what "healthy" streaming looks like.

export function formatPct(ratio: number): string {
  if (!Number.isFinite(ratio) || ratio <= 0) return "0%";
  return `${(ratio * 100).toFixed(2)}%`;
}

export function formatInt(n: number | undefined): string {
  if (n === undefined || n === null || !Number.isFinite(n)) return "-";
  return n.toLocaleString();
}

// formatRelative renders an ISO timestamp as a short "Xm/Xh/Xd ago" label.
// `justNow` is injected by the caller since it's an i18n string, not a unit.
export function formatRelative(iso: string | undefined, justNow: string): string {
  if (!iso) return "-";
  const then = new Date(iso).getTime();
  if (!Number.isFinite(then)) return "-";
  const delta = (Date.now() - then) / 1000;
  if (delta < 60) return justNow;
  if (delta < 3600) return `${Math.floor(delta / 60)}m`;
  if (delta < 86400) return `${Math.floor(delta / 3600)}h`;
  return `${Math.floor(delta / 86400)}d`;
}

// HealthLevel buckets the starvation ratio into a qualitative streaming-health
// verdict. Thresholds are deliberately tight: rebuffering is the single most
// hated playback event, so even a few percent is "marginal".
export type HealthLevel =
  | "excellent"
  | "good"
  | "fair"
  | "marginal"
  | "poor"
  | "insufficient";

// qualityStatus maps the starvation ratio (+ sample size) to a HealthLevel.
// Below ~50 events the ratio is too noisy to be a verdict, so we report
// "insufficient" rather than a green/red that flips on the next session.
export function qualityStatus(ratio: number, totalEvents: number): HealthLevel {
  if (!totalEvents || totalEvents < 50) return "insufficient";
  if (ratio < 0.01) return "excellent";
  if (ratio < 0.03) return "good";
  if (ratio < 0.05) return "fair";
  if (ratio < 0.1) return "marginal";
  return "poor";
}
