import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
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
    staleTime: 30_000,
    refetchInterval: 60_000,
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
                    <th className="p-3">{t("admin.qoe.col.mediaId")}</th>
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
                    <tr key={row.media_item_id} className="border-t border-border">
                      <td className="p-3 font-mono text-xs">
                        {row.media_item_id}
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
        </>
      )}
    </div>
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
