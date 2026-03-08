import { Card } from "@/components/ui/Card";
import type { V1DashboardInfo } from "@evey/api/src/generated/admin.schemas";

function formatBytes(bytes: string | undefined) {
  const n = parseInt(bytes ?? "0");
  if (n > 1e9) return `${(n / 1e9).toFixed(1)} GB`;
  if (n > 1e6) return `${(n / 1e6).toFixed(1)} MB`;
  return `${n} B`;
}

interface StatCardProps {
  label: string;
  value: string | number;
}

function StatCard({ label, value }: StatCardProps) {
  return (
    <Card className="flex flex-col gap-1">
      <span className="text-xs font-medium uppercase tracking-wide" style={{ color: "var(--text-muted)" }}>
        {label}
      </span>
      <span className="text-2xl font-bold" style={{ color: "var(--text)" }}>
        {value}
      </span>
    </Card>
  );
}

interface DashboardStatsProps {
  info: V1DashboardInfo;
}

export function DashboardStats({ info }: DashboardStatsProps) {
  return (
    <div className="flex flex-col gap-6">
      <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-6 gap-4">
        <StatCard label="Active Streams" value={info.activeStreams ?? 0} />
        <StatCard label="Active Transcodes" value={info.activeTranscodes ?? 0} />
        <StatCard label="CPU" value={`${(info.cpuPercent ?? 0).toFixed(1)}%`} />
        <StatCard label="Memory" value={`${(info.memoryPercent ?? 0).toFixed(1)}%`} />
        <StatCard label="Total Media" value={info.totalMediaItems ?? 0} />
        <StatCard label="Total Users" value={info.totalUsers ?? 0} />
      </div>

      {info.storage && info.storage.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold mb-3" style={{ color: "var(--text-muted)" }}>
            Storage
          </h3>
          <div className="flex flex-col gap-2">
            {info.storage.map((s, i) => {
              const used = parseInt(s.usedBytes ?? "0");
              const total = parseInt(s.totalBytes ?? "1");
              const pct = total > 0 ? Math.round((used / total) * 100) : 0;
              return (
                <div
                  key={i}
                  className="p-3 rounded-[var(--radius-md)]"
                  style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
                >
                  <div className="flex justify-between text-sm mb-2">
                    <span style={{ color: "var(--text)" }} className="truncate">{s.path ?? "—"}</span>
                    <span style={{ color: "var(--text-muted)" }}>
                      {formatBytes(s.usedBytes)} / {formatBytes(s.totalBytes)}
                    </span>
                  </div>
                  <div className="h-1.5 rounded-full" style={{ backgroundColor: "var(--border)" }}>
                    <div
                      className="h-1.5 rounded-full"
                      style={{ width: `${pct}%`, backgroundColor: "var(--accent)" }}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      )}
    </div>
  );
}
