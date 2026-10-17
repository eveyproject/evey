import { useTranslation } from "react-i18next";
import { HardDrive, AlertTriangle } from "lucide-react";
import type { V1DiskIOInfo } from "@evey/api";

import { StatCard } from "./DashboardStatCard";

// formatBytesPerSecond renders a byte rate the way an admin reads iostat:
// MB/s above a megabyte, kB/s under it.
export function formatBytesPerSecond(bps: string | number | undefined): string {
  const n = typeof bps === "string" ? parseInt(bps) : (bps ?? 0);
  if (!Number.isFinite(n) || n < 0) return "-";
  if (n >= 1e9) return `${(n / 1e9).toFixed(2)} GB/s`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)} MB/s`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(0)} kB/s`;
  return `${n} B/s`;
}

// busiestDisk is the disk that waited the most. Not the busiest by share: a
// disk 100 % busy serving every request in 2 ms keeps up, one 60 % busy that
// makes each request wait 40 ms is the one that broke the load test.
export function busiestDisk(disks: V1DiskIOInfo[] | undefined): V1DiskIOInfo | undefined {
  if (!disks || disks.length === 0) return undefined;
  return disks.reduce((worst, d) => ((d.awaitMs ?? 0) > (worst.awaitMs ?? 0) ? d : worst), disks[0]);
}

const DiskIcon = () => <HardDrive size={20} strokeWidth={1.5} />;

interface DisksStatCardProps {
  disks: V1DiskIOInfo[] | undefined;
}

// DisksStatCard is the dashboard's tile: the worst disk's wait, with its
// label, and a saturated word when the server judged it so. Nothing when the
// machine exposes no disk, rather than a reassuring zero.
export function DisksStatCard({ disks }: DisksStatCardProps) {
  const { t } = useTranslation();
  const worst = busiestDisk(disks);
  if (!worst) return null;
  const saturated = worst.saturated === true;
  return (
    <div data-testid="disks-stat-card">
      <StatCard
        icon={<DiskIcon />}
        label={t("admin.dashboardStats.disks")}
        value={`${(worst.awaitMs ?? 0).toFixed(1)} ms`}
        valueColor={saturated ? "var(--color-danger)" : undefined}
        sub={
          saturated
            ? t("admin.dashboardStats.diskSaturatedSub", { label: worst.label ?? worst.device ?? "" })
            : t("admin.dashboardStats.diskSub", { label: worst.label ?? worst.device ?? "", percent: Math.round(worst.utilPercent ?? 0) })
        }
      />
    </div>
  );
}

interface DiskIOListProps {
  disks: V1DiskIOInfo[];
  idleAwaitMs: number | undefined;
  saturationAwaitMs: number | undefined;
  iowaitPercent: number | undefined;
}

// DiskIOList is the system-health detail: one row per physical disk, the
// wait first, the busy share and rates after, and the two reference waits
// the verdict is drawn from, so the admin can see why a disk is flagged.
export function DiskIOList({ disks, idleAwaitMs, saturationAwaitMs, iowaitPercent }: DiskIOListProps) {
  const { t } = useTranslation();
  return (
    <div
      className="p-4 rounded-[var(--radius-lg)] flex flex-col gap-3 bg-[var(--surface)] border border-[var(--border)]"
      data-testid="disk-io-list"
    >
      <div className="flex flex-wrap justify-between gap-2 text-xs text-[var(--text-muted)]">
        <span>
          {t("admin.systemHealth.diskRestLine", {
            idle: (idleAwaitMs ?? 0).toFixed(1),
            line: (saturationAwaitMs ?? 0).toFixed(0),
          })}
        </span>
        <span>{t("admin.systemHealth.diskIowait", { percent: (iowaitPercent ?? 0).toFixed(1) })}</span>
      </div>
      <ul className="flex flex-col divide-y divide-[var(--border)]">
        {disks.map((d) => {
          const saturated = d.saturated === true;
          return (
            <li key={d.device ?? d.label} className="flex flex-wrap items-center gap-x-4 gap-y-1 py-2" data-testid="disk-io-row">
              <div className="flex flex-col min-w-0 flex-1">
                <span className="text-sm font-medium text-[var(--text)] truncate">{d.label || d.device}</span>
                <span className="text-xs text-[var(--text-muted)]">
                  {d.device}
                  {d.rotational === false ? ` · ${t("admin.systemHealth.diskSolidState")}` : ""}
                </span>
              </div>
              <span
                className={`text-lg font-bold tabular-nums ${saturated ? "text-[var(--color-danger)]" : "text-[var(--text)]"}`}
                title={t("admin.systemHealth.diskAwaitHint")}
              >
                {(d.awaitMs ?? 0).toFixed(1)} ms
              </span>
              <span className="text-xs text-[var(--text-muted)] tabular-nums w-24 text-right">
                {t("admin.systemHealth.diskUtil", { percent: Math.round(d.utilPercent ?? 0) })}
              </span>
              <span className="text-xs text-[var(--text-muted)] tabular-nums w-40 text-right">
                {t("admin.systemHealth.diskRates", {
                  read: formatBytesPerSecond(d.readBps),
                  write: formatBytesPerSecond(d.writeBps),
                })}
              </span>
              {saturated && (
                <span className="inline-flex items-center gap-1 text-xs font-semibold text-[var(--color-danger)]">
                  <AlertTriangle size={14} strokeWidth={2} aria-hidden />
                  {t("admin.systemHealth.diskSaturated")}
                </span>
              )}
            </li>
          );
        })}
      </ul>
      <p className="text-xs text-[var(--text-muted)]">{t("admin.systemHealth.diskMachineNote")}</p>
    </div>
  );
}
