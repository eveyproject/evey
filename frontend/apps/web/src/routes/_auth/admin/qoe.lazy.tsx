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
  if (n === undefined || n === null) return "—";
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
                        {Math.round(row.avg_buffer_ms).toLocaleString()}
                      </td>
                      <td className="p-3 text-right">
                        {Math.round(row.avg_encoded_kbps).toLocaleString()}
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
// the Phase C worker has written. Read-only — the worker is the only
// writer, and we deliberately don't ship a force-reset button (admins
// can wait for TTL, or recalibrate the encode capability if the
// constraint comes from the underlying hardware).
function EncodeOverridesCard() {
  const { t, i18n } = useTranslation();
  const { data } = useQuery<{ overrides: EncodeOverride[] }>({
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
        <h2 className="text-lg font-semibold">
          {t("admin.qoe.overrides.title", "Adaptive encode caps")}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t(
            "admin.qoe.overrides.subtitle",
            "CMCD-feedback adjustments on top of the calibrated ceiling. Each row lowers the cap for one source bucket until it expires.",
          )}
        </p>
      </div>
      {overrides.length === 0 ? (
        <p className="p-4 text-sm text-muted-foreground">
          {t(
            "admin.qoe.overrides.empty",
            "No active adaptive overrides — the calibrated ceiling is in effect on every source class.",
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
            </tr>
          </thead>
          <tbody>
            {overrides.map((o) => (
              <tr key={o.source_class} className="border-t border-border">
                <td className="p-3 font-medium">{o.source_class}</td>
                <td className="p-3 text-right">{o.max_height}p</td>
                <td className="p-3 text-right">{(o.starved_ratio * 100).toFixed(1)}%</td>
                <td className="p-3 text-right">{o.samples_count}</td>
                <td className="p-3 text-xs text-muted-foreground">{fmtDate(o.applied_at)}</td>
                <td className="p-3 text-xs text-muted-foreground">{fmtDate(o.expires_at)}</td>
                <td className="p-3 text-xs text-muted-foreground">{o.reason}</td>
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
      ? "border-amber-500/30 bg-amber-500/5"
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
