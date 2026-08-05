import { Film, Monitor, Users, Zap, Cpu, MemoryStick, HardDrive, Activity, Gauge } from "lucide-react";

export function getUsageColor(pct: number): string {
  if (pct > 85) return "var(--color-danger)";
  if (pct > 70) return "var(--color-warning)";
  return "var(--color-success)";
}

export const MovieIcon = () => <Film size={20} strokeWidth={1.5} />;
export const SeriesIcon = () => <Monitor size={20} strokeWidth={1.5} />;
export const UsersIcon = () => <Users size={20} strokeWidth={1.5} />;
export const StreamIcon = () => <Zap size={20} strokeWidth={1.5} />;
export const CpuIcon = () => <Cpu size={20} strokeWidth={1.5} />;
export const MemoryIcon = () => <MemoryStick size={20} strokeWidth={1.5} />;
export const StorageIcon = () => <HardDrive size={20} strokeWidth={1.5} />;
export const ThroughputIcon = () => <Activity size={20} strokeWidth={1.5} />;
export const TheoreticalIcon = () => <Gauge size={20} strokeWidth={1.5} />;

// formatBps renders a bits/s rate. A negative value (-1 = no measurement yet,
// e.g. the first dashboard poll has no previous sample to diff against) shows
// an em dash rather than a misleading 0 or negative rate.
export function formatBps(bps: string | number | undefined): string {
  const n = typeof bps === "string" ? parseInt(bps) : (bps ?? 0);
  if (!Number.isFinite(n) || n < 0) return "-";
  if (n >= 1e9) return `${(n / 1e9).toFixed(2)} Gbps`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)} Mbps`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(0)} kbps`;
  return `${n} bps`;
}

interface StatCardProps {
  icon: React.ReactNode;
  label: string;
  value: string | number;
  valueColor?: string;
}

export function StatCard({ icon, label, value, valueColor }: StatCardProps) {
  return (
    <div
      className="flex items-center gap-3 p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
    >
      <div
        className="flex items-center justify-center w-10 h-10 rounded-[var(--radius-md)] bg-[var(--surface-raised)] text-[var(--accent)]"
      >
        {icon}
      </div>
      <div className="flex flex-col min-w-0">
        <span className="text-2xl font-bold leading-tight" style={{ color: valueColor ?? "var(--text)" }}>
          {value}
        </span>
        <span className="text-xs font-medium uppercase tracking-wide text-[var(--text-muted)]">
          {label}
        </span>
      </div>
    </div>
  );
}
