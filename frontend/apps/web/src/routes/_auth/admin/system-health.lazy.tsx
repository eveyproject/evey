import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api/src/fetcher";
import { Spinner } from "@/components/ui/Spinner";

export const Route = createLazyFileRoute("/_auth/admin/system-health")({
  component: SystemHealthPage,
});

interface SystemStats {
  goroutines: number;
  memAllocBytes: number;
  memSysBytes: number;
  numGC: number;
  uptimeSeconds: number;
  goVersion: string;
  os: string;
  arch: string;
  numCPU: number;
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
  return `${(bytes / 1024 / 1024 / 1024).toFixed(2)} GB`;
}

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (d > 0) return `${d}d ${h}h ${m}m`;
  if (h > 0) return `${h}h ${m}m ${s}s`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

function StatCard({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div
      className="p-4 rounded-[var(--radius-lg)]"
      style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
    >
      <p className="text-xs font-semibold uppercase tracking-wider mb-1" style={{ color: "var(--text-muted)" }}>
        {label}
      </p>
      <p className="text-2xl font-bold" style={{ color: "var(--text)" }}>{value}</p>
      {sub && <p className="text-xs mt-0.5" style={{ color: "var(--text-muted)" }}>{sub}</p>}
    </div>
  );
}

function SystemHealthPage() {
  const { data, isLoading, dataUpdatedAt } = useQuery<SystemStats>({
    queryKey: ["/api/v1/admin/system/stats"],
    queryFn: () => eveyFetch<SystemStats>({ url: "/api/v1/admin/system/stats", method: "GET" }),
    refetchInterval: 5_000,
  });

  return (
    <div className="flex flex-col gap-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>System Health</h1>
          {dataUpdatedAt > 0 && (
            <p className="text-xs mt-1" style={{ color: "var(--text-muted)" }}>
              Updated {new Date(dataUpdatedAt).toLocaleTimeString()} · auto-refreshes every 5s
            </p>
          )}
        </div>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16"><Spinner size="lg" /></div>
      ) : data ? (
        <>
          <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-4">
            <StatCard label="Uptime" value={formatUptime(data.uptimeSeconds)} />
            <StatCard label="Goroutines" value={String(data.goroutines)} />
            <StatCard label="Heap Alloc" value={formatBytes(data.memAllocBytes)} sub={`sys: ${formatBytes(data.memSysBytes)}`} />
            <StatCard label="GC Cycles" value={String(data.numGC)} />
            <StatCard label="CPUs" value={String(data.numCPU)} />
            <StatCard label="Go Version" value={data.goVersion} />
            <StatCard label="OS / Arch" value={`${data.os}/${data.arch}`} />
          </div>
        </>
      ) : null}
    </div>
  );
}
