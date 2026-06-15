import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Gauge, AlertTriangle, Users, Film } from "lucide-react";
import { eveyFetch } from "@evey/api";
import { Spinner } from "@/components/ui/Spinner";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { PageHeader } from "@/components/ui/PageHeader";
import { InfoTooltip } from "@/components/ui/InfoTooltip";

export const Route = createLazyFileRoute("/_auth/admin/qoe")({
  component: QoEPage,
});

interface QoESummary {
  window: string;
  total_events: number;
  starved_events: number;
  starved_ratio: number;
  unique_sessions: number;
  unique_media: number;
}

interface QoEStarvedTitle {
  media_item_id: string;
  title: string;
  file_path: string;
  starved_events: number;
  total_events: number;
  starved_ratio: number;
  last_starved_at: string;
}

interface QoEDeviceBreakdown {
  device_class: string;
  total_events: number;
  starved_events: number;
  starved_ratio: number;
  avg_buffer_ms: number;
  avg_encoded_kbps: number;
}

interface QoEDashboard {
  summary: QoESummary;
  top_starved_titles: QoEStarvedTitle[];
  by_device: QoEDeviceBreakdown[];
}

type Window = "24h" | "7d";

function useQoEDashboard(window: Window) {
  return useQuery<QoEDashboard>({
    queryKey: ["admin", "qoe", "dashboard", window],
    queryFn: () =>
      eveyFetch<QoEDashboard>({
        url: `/api/v1/admin/qoe/dashboard?window=${window}`,
        method: "GET",
      }),
    // Refetch on every window toggle: 24h and 7d use separate React Query
    // caches, and a 30s staleTime made one of them go stale while the other
    // was the fresh focused view — you'd see 24h > 7d which is
    // mathematically impossible. keepPreviousData avoids a loading flash
    // during the refetch.
    staleTime: 0,
    refetchInterval: 60_000,
    placeholderData: keepPreviousData,
  });
}

function formatPct(ratio: number): string {
  if (!Number.isFinite(ratio) || ratio <= 0) return "0%";
  return `${(ratio * 100).toFixed(2)}%`;
}

function formatInt(n: number | undefined): string {
  if (n === undefined || n === null || !Number.isFinite(n)) return "—";
  return n.toLocaleString();
}

function formatRelative(iso: string, t: (k: string) => string): string {
  if (!iso) return "—";
  const then = new Date(iso).getTime();
  const delta = (Date.now() - then) / 1000;
  if (delta < 60) return t("admin.qoe.justNow");
  if (delta < 3600) return `${Math.floor(delta / 60)}m`;
  if (delta < 86400) return `${Math.floor(delta / 3600)}h`;
  return `${Math.floor(delta / 86400)}d`;
}

// HealthLevel buckets the starvation ratio into a qualitative streaming-health
// verdict. Thresholds are deliberately tight: rebuffering is the single most
// hated playback event, so even a few percent is "marginal".
type HealthLevel =
  | "excellent"
  | "good"
  | "fair"
  | "marginal"
  | "poor"
  | "insufficient";

// qualityStatus maps the starvation ratio (+ sample size) to a HealthLevel.
// Below ~50 events the ratio is too noisy to be a verdict, so we report
// "insufficient" rather than a green/red that flips on the next session.
function qualityStatus(ratio: number, totalEvents: number): HealthLevel {
  if (!totalEvents || totalEvents < 50) return "insufficient";
  if (ratio < 0.01) return "excellent";
  if (ratio < 0.03) return "good";
  if (ratio < 0.05) return "fair";
  if (ratio < 0.1) return "marginal";
  return "poor";
}

// HealthBanner shows the verdict badge + a concrete recommendation. Colour is
// keyed to the level: green (excellent/good), neutral (fair/insufficient),
// amber (marginal), red (poor).
function HealthBanner({ level, ratio }: { level: HealthLevel; ratio: number }) {
  const { t } = useTranslation();
  const card: Record<HealthLevel, string> = {
    excellent: "border-[color-mix(in_srgb,var(--color-success)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-success)_5%,transparent)]",
    good: "border-[color-mix(in_srgb,var(--color-success)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-success)_5%,transparent)]",
    fair: "border-border bg-muted/20",
    marginal: "border-[color-mix(in_srgb,var(--color-warning)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_5%,transparent)]",
    poor: "border-destructive/40 bg-destructive/5",
    insufficient: "border-border bg-muted/10",
  };
  const badge: Record<HealthLevel, string> = {
    excellent: "bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]",
    good: "bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]",
    fair: "bg-muted text-foreground",
    marginal: "bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)] text-[var(--color-warning)]",
    poor: "bg-destructive/15 text-destructive",
    insufficient: "bg-muted text-muted-foreground",
  };
  const recoKey =
    "admin.qoe.health.reco" + level.charAt(0).toUpperCase() + level.slice(1);
  return (
    <Card
      className={`flex flex-col gap-2 p-4 sm:flex-row sm:items-center sm:gap-4 ${card[level]}`}
    >
      <div className="flex shrink-0 items-center gap-2">
        <span className="text-sm text-muted-foreground">
          {t("admin.qoe.health.label")}
        </span>
        <span
          className={`rounded-full px-2.5 py-0.5 text-sm font-semibold ${badge[level]}`}
        >
          {t(`admin.qoe.health.${level}`)}
        </span>
        {level !== "insufficient" && (
          <span className="text-xs text-muted-foreground">
            {formatPct(ratio)}
          </span>
        )}
      </div>
      <p className="text-sm text-foreground/90">{t(recoKey)}</p>
    </Card>
  );
}

function QoEPage() {
  const { t } = useTranslation();
  const [window, setWindow] = useState<Window>("24h");
  const { data, isLoading, isError, error } = useQoEDashboard(window);

  return (
    <div className="space-y-6 p-6">
      <PageHeader
        title={t("admin.qoe.title")}
        subtitle={t("admin.qoe.subtitle")}
      />

      <div className="flex gap-2">
        <Button
          variant={window === "24h" ? "primary" : "ghost"}
          onClick={() => setWindow("24h")}
        >
          {t("admin.qoe.window24h")}
        </Button>
        <Button
          variant={window === "7d" ? "primary" : "ghost"}
          onClick={() => setWindow("7d")}
        >
          {t("admin.qoe.window7d")}
        </Button>
      </div>

      {isLoading && (
        <div className="flex items-center gap-2 text-muted-foreground">
          <Spinner /> {t("admin.qoe.loading")}
        </div>
      )}

      {isError && (
        <Card className="border-destructive/30 bg-destructive/5 p-4">
          <p className="text-destructive">
            {t("admin.qoe.loadError")}: {(error as Error)?.message ?? "unknown"}
          </p>
        </Card>
      )}

      {data && (
        <>
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <KpiCard
              label={t("admin.qoe.kpi.events")}
              value={formatInt(data.summary?.total_events)}
              icon={<Gauge className="size-5" />}
            />
            <KpiCard
              label={t("admin.qoe.kpi.starvedRatio")}
              value={formatPct(data.summary?.starved_ratio ?? 0)}
              icon={<AlertTriangle className="size-5" />}
              variant={
                (data.summary?.starved_ratio ?? 0) > 0.05
                  ? "warning"
                  : "default"
              }
            />
            <KpiCard
              label={t("admin.qoe.kpi.sessions")}
              value={formatInt(data.summary?.unique_sessions)}
              icon={<Users className="size-5" />}
            />
            <KpiCard
              label={t("admin.qoe.kpi.uniqueMedia")}
              value={formatInt(data.summary?.unique_media)}
              icon={<Film className="size-5" />}
            />
          </div>

          <HealthBanner
            level={qualityStatus(
              data.summary?.starved_ratio ?? 0,
              data.summary?.total_events ?? 0,
            )}
            ratio={data.summary?.starved_ratio ?? 0}
          />

          <Card>
            <div className="border-b border-border p-4">
              <h2 className="text-lg font-semibold">
                {t("admin.qoe.topStarved.title")}
              </h2>
              <p className="text-sm text-muted-foreground">
                {t("admin.qoe.topStarved.subtitle")}
              </p>
            </div>
            {data.top_starved_titles.length === 0 ? (
              <p className="p-4 text-sm text-muted-foreground">
                {t("admin.qoe.topStarved.empty")}
              </p>
            ) : (
              <table className="w-full text-sm">
                <thead className="bg-muted/40 text-left">
                  <tr>
                    <th className="p-3">{t("admin.qoe.col.title", "Title")}</th>
                    <th className="p-3 text-right">
                      {t("admin.qoe.col.starvedRatio")}
                    </th>
                    <th className="p-3 text-right">
                      {t("admin.qoe.col.starvedEvents")}
                    </th>
                    <th className="p-3 text-right">
                      {t("admin.qoe.col.totalEvents")}
                    </th>
                    <th className="p-3 text-right">
                      {t("admin.qoe.col.lastStarved")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.top_starved_titles.map((row) => (
                    <tr key={row.media_item_id} className="border-t border-border align-top">
                      <td className="p-3">
                        {/* Title (or fallback to short ID for orphans
                            after a library cleanup) + secondary line
                            with file_path so an admin can grep the
                            disk / re-encode source without an extra
                            SQL hop. */}
                        <div className="font-medium">
                          {row.title || (
                            <span className="font-mono text-xs text-muted-foreground">
                              {row.media_item_id.slice(0, 8)}…
                            </span>
                          )}
                        </div>
                        {row.file_path && (
                          <div
                            className="mt-0.5 truncate font-mono text-xs text-muted-foreground"
                            title={row.file_path}
                          >
                            {row.file_path}
                          </div>
                        )}
                      </td>
                      <td className="p-3 text-right">
                        {formatPct(row.starved_ratio)}
                      </td>
                      <td className="p-3 text-right">
                        {formatInt(row.starved_events)}
                      </td>
                      <td className="p-3 text-right">
                        {formatInt(row.total_events)}
                      </td>
                      <td className="p-3 text-right text-muted-foreground">
                        {formatRelative(row.last_starved_at, t)}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>

          <Card>
            <div className="border-b border-border p-4">
              <h2 className="text-lg font-semibold">
                {t("admin.qoe.byDevice.title")}
              </h2>
              <p className="text-sm text-muted-foreground">
                {t("admin.qoe.byDevice.subtitle")}
              </p>
            </div>
            {data.by_device.length === 0 ? (
              <p className="p-4 text-sm text-muted-foreground">
                {t("admin.qoe.byDevice.empty")}
              </p>
            ) : (
              <table className="w-full text-sm">
                <thead className="bg-muted/40 text-left">
                  <tr>
                    <th className="p-3">{t("admin.qoe.col.deviceClass")}</th>
                    <th className="p-3 text-right">
                      {t("admin.qoe.col.totalEvents")}
                    </th>
                    <th className="p-3 text-right">
                      {t("admin.qoe.col.starvedRatio")}
                    </th>
                    <th className="p-3 text-right">
                      {t("admin.qoe.col.avgBufferMs")}
                    </th>
                    <th className="p-3 text-right">
                      {t("admin.qoe.col.avgEncodedKbps")}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.by_device.map((row) => (
                    <tr key={row.device_class} className="border-t border-border">
                      <td className="p-3 font-mono text-xs">
                        {row.device_class}
                      </td>
                      <td className="p-3 text-right">
                        {formatInt(row.total_events)}
                      </td>
                      <td className="p-3 text-right">
                        {formatPct(row.starved_ratio)}
                      </td>
                      <td className="p-3 text-right">
                        {formatInt(Math.round(row.avg_buffer_ms))}
                      </td>
                      <td className="p-3 text-right">
                        {formatInt(Math.round(row.avg_encoded_kbps))}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>

          <EncodeOverridesCard />
        </>
      )}
    </div>
  );
}

type EncodeOverride = {
  source_class: string;
  max_height: number;
  reason: string;
  starved_ratio: number;
  samples_count: number;
  applied_at: string;
  expires_at: string;
};

// EncodeOverridesCard surfaces the CMCD-feedback adaptive cap overrides
// the Phase C worker has written. Admin can wait for TTL (7 d) or use
// the "Reset" button to drop a row immediately — typically when they
// know the underlying limit changed (HW added, faster CPU swapped in).
function EncodeOverridesCard() {
  const { t, i18n } = useTranslation();
  const { data, refetch } = useQuery<{ overrides: EncodeOverride[] }>({
    queryKey: ["admin", "transcode-overrides"],
    queryFn: () =>
      eveyFetch<{ overrides: EncodeOverride[] }>({
        url: "/api/v1/admin/transcode/calibration/overrides",
        method: "GET",
      }),
    // Worker runs every 30 min; an admin viewing this card occasionally
    // doesn't need sub-second freshness.
    staleTime: 60_000,
  });

  const resetOverride = async (sourceClass: string) => {
    try {
      await eveyFetch<void>({
        url: `/api/v1/admin/transcode/calibration/overrides?source_class=${encodeURIComponent(sourceClass)}`,
        method: "DELETE",
      });
      await refetch();
    } catch (err) {
      // Surface the error in the console — the table will simply
      // refresh on the next staleTime expiry if the request silently
      // failed at the network layer.
      console.warn("override reset failed", err);
    }
  };

  const overrides = data?.overrides ?? [];

  const fmtDate = (iso: string) => {
    try {
      return new Date(iso).toLocaleString(i18n.language);
    } catch {
      return iso;
    }
  };

  return (
    <Card>
      <div className="border-b border-border p-4">
        <h2 className="flex items-center gap-1.5 text-lg font-semibold">
          {t("admin.qoe.overrides.title", "Automatic quality limits")}
          <InfoTooltip label={t("admin.qoe.overrides.title", "Automatic quality limits")}>
            {t(
              "admin.qoe.overrides.help",
              "Players continuously report how playback is going — buffer level, stalls and bandwidth (an industry standard called CMCD). When one kind of source keeps stalling viewers, Evey lowers its quality limit for a while to stop the stalls, then lifts it again automatically.",
            )}
          </InfoTooltip>
        </h2>
        <p className="text-sm text-muted-foreground">
          {t(
            "admin.qoe.overrides.subtitle",
            "Automatic adjustments based on how playback actually goes (buffering, throughput), on top of the calibrated ceiling. Each row temporarily lowers the maximum quality for one kind of source until it expires.",
          )}
        </p>
      </div>
      {overrides.length === 0 ? (
        <p className="p-4 text-sm text-muted-foreground">
          {t(
            "admin.qoe.overrides.empty",
            "No active automatic limits — the full calibrated quality ceiling applies to every kind of source.",
          )}
        </p>
      ) : (
        <table className="w-full text-sm">
          <thead className="bg-muted/40 text-left">
            <tr>
              <th className="p-3">{t("admin.qoe.overrides.col.sourceClass", "Source bucket")}</th>
              <th className="p-3 text-right">{t("admin.qoe.overrides.col.maxHeight", "Cap")}</th>
              <th className="p-3 text-right">{t("admin.qoe.overrides.col.starvedRatio", "Starved")}</th>
              <th className="p-3 text-right">{t("admin.qoe.overrides.col.samples", "Samples")}</th>
              <th className="p-3">{t("admin.qoe.overrides.col.appliedAt", "Applied")}</th>
              <th className="p-3">{t("admin.qoe.overrides.col.expiresAt", "Expires")}</th>
              <th className="p-3">{t("admin.qoe.overrides.col.reason", "Reason")}</th>
              <th className="p-3"></th>
            </tr>
          </thead>
          <tbody>
            {overrides.map((o) => (
              <tr key={o.source_class} className="border-t border-border">
                <td className="p-3 font-medium">{o.source_class}</td>
                <td className="p-3 text-right">{o.max_height}p</td>
                <td className="p-3 text-right">{formatPct(o.starved_ratio)}</td>
                <td className="p-3 text-right">{o.samples_count}</td>
                <td className="p-3 text-xs text-muted-foreground">{fmtDate(o.applied_at)}</td>
                <td className="p-3 text-xs text-muted-foreground">{fmtDate(o.expires_at)}</td>
                <td className="p-3 text-xs text-muted-foreground">{o.reason}</td>
                <td className="p-3 text-right">
                  <button
                    type="button"
                    onClick={() => resetOverride(o.source_class)}
                    className="text-xs text-[var(--accent)] hover:underline"
                    title={t(
                      "admin.qoe.overrides.resetHint",
                      "Drop this adaptive cap immediately. Worker will re-evaluate on the next tick.",
                    )}
                  >
                    {t("admin.qoe.overrides.reset", "Reset")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </Card>
  );
}

interface KpiCardProps {
  label: string;
  value: string;
  icon: React.ReactNode;
  variant?: "default" | "warning";
}

function KpiCard({ label, value, icon, variant = "default" }: KpiCardProps) {
  const accent =
    variant === "warning"
      ? "border-[color-mix(in_srgb,var(--color-warning)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_5%,transparent)]"
      : "border-border";
  return (
    <Card className={`p-4 ${accent}`}>
      <div className="flex items-center gap-2 text-sm text-muted-foreground">
        {icon}
        <span>{label}</span>
      </div>
      <div className="mt-2 text-2xl font-semibold">{value}</div>
    </Card>
  );
}
