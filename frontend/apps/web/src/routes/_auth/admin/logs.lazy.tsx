import { useEffect, useRef, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import { eveyFetch } from "@evey/api/src/fetcher";
import { Spinner } from "@/components/ui/Spinner";

export const Route = createLazyFileRoute("/_auth/admin/logs")({
  component: LogsPage,
});

const LEVELS = ["all", "debug", "info", "warn", "error"] as const;
type Level = (typeof LEVELS)[number];

interface ParsedLine {
  raw: string;
  level?: string;
  time?: string;
  msg?: string;
}

function parseLine(raw: string): ParsedLine {
  try {
    const obj = JSON.parse(raw) as Record<string, string>;
    return { raw, level: obj.level, time: obj.time, msg: obj.msg ?? raw };
  } catch {
    return { raw };
  }
}

function levelColor(level?: string): string {
  switch ((level ?? "").toLowerCase()) {
    case "error": return "#f87171";
    case "warn": return "#fbbf24";
    case "info": return "#60a5fa";
    case "debug": return "#94a3b8";
    default: return "var(--text-muted)";
  }
}

function levelBg(level?: string): string {
  switch ((level ?? "").toLowerCase()) {
    case "error": return "rgba(248,113,113,0.15)";
    case "warn": return "rgba(251,191,36,0.08)";
    default: return "transparent";
  }
}

const LOG_LINE_HEIGHT = 22;

function VirtualLogList({ lines }: { lines: ParsedLine[] }) {
  const parentRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: lines.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => LOG_LINE_HEIGHT,
    overscan: 20,
  });

  useEffect(() => {
    if (lines.length > 0) {
      virtualizer.scrollToIndex(lines.length - 1, { behavior: "smooth" });
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [lines.length]);

  return (
    <div
      ref={parentRef}
      className="flex-1 overflow-y-auto rounded-[var(--radius-lg)] p-3 font-mono text-xs"
      style={{ backgroundColor: "#0d1117", border: "1px solid var(--border)", minHeight: 400, maxHeight: "60vh" }}
    >
      <div style={{ height: virtualizer.getTotalSize(), position: "relative" }}>
        {virtualizer.getVirtualItems().map((vItem) => {
          const line = lines[vItem.index];
          return (
            <div
              key={vItem.index}
              style={{
                position: "absolute",
                top: 0,
                left: 0,
                right: 0,
                transform: `translateY(${vItem.start}px)`,
                height: LOG_LINE_HEIGHT,
                backgroundColor: levelBg(line.level),
              }}
              className="flex gap-2 items-center px-1 rounded hover:bg-white/5 transition-colors"
            >
              {line.time && (
                <span className="flex-shrink-0 text-[11px]" style={{ color: "#6e7681" }}>
                  {line.time.substring(11, 19)}
                </span>
              )}
              {line.level && (
                <span
                  className="flex-shrink-0 uppercase font-bold text-[10px] w-[26px] text-center px-0.5 rounded"
                  style={{ color: levelColor(line.level), border: `1px solid ${levelColor(line.level)}33` }}
                >
                  {line.level.substring(0, 4)}
                </span>
              )}
              <span className="truncate" style={{ color: "#e6edf3" }}>
                {line.msg ?? line.raw}
              </span>
            </div>
          );
        })}
      </div>
      {lines.length === 0 && (
        <p style={{ color: "var(--text-muted)" }}>No log entries match your filter.</p>
      )}
    </div>
  );
}

function LogsPage() {
  const [fetchLines, setFetchLines] = useState(200);
  const [filterLevel, setFilterLevel] = useState<Level>("all");
  const [search, setSearch] = useState("");
  const [live, setLive] = useState(true);
  const [cleared, setCleared] = useState<number>(0);

  const { data, isLoading } = useQuery<{ lines: string[] }>({
    queryKey: ["/api/v1/admin/logs", fetchLines, cleared],
    queryFn: () => eveyFetch<{ lines: string[] }>({ url: `/api/v1/admin/logs?lines=${fetchLines}`, method: "GET" }),
    refetchInterval: live ? 2_000 : false,
  });

  const rawLines = data?.lines ?? [];
  const parsed = rawLines.map(parseLine);
  const filtered = parsed.filter((l) => {
    if (filterLevel !== "all" && l.level?.toLowerCase() !== filterLevel) return false;
    if (search && !l.raw.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  return (
    <div className="flex flex-col gap-4 h-full">
      <div className="flex items-center gap-3 flex-wrap">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>Live Logs</h1>
        <div className="flex-1" />

        <label className="flex items-center gap-1.5 text-sm cursor-pointer" style={{ color: "var(--text)" }}>
          <input
            type="checkbox"
            checked={live}
            onChange={(e) => setLive(e.target.checked)}
            className="accent-[var(--accent)]"
          />
          Live
        </label>

        <select
          value={fetchLines}
          onChange={(e) => setFetchLines(Number(e.target.value))}
          className="text-sm px-2 py-1 rounded"
          style={{ backgroundColor: "var(--surface-raised)", color: "var(--text)", border: "1px solid var(--border)" }}
        >
          {[50, 100, 200, 500, 1000].map((n) => (
            <option key={n} value={n}>{n} lines</option>
          ))}
        </select>

        <button
          onClick={() => setCleared(Date.now())}
          className="text-sm px-3 py-1 rounded transition-colors hover:opacity-80"
          style={{ backgroundColor: "var(--surface-raised)", color: "var(--text-muted)", border: "1px solid var(--border)" }}
        >
          Clear display
        </button>
      </div>

      <div className="flex gap-2 flex-wrap">
        <input
          placeholder="Search logs…"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="px-3 py-1.5 text-sm rounded-[var(--radius-md)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
          style={{ backgroundColor: "var(--surface-raised)", border: "1px solid var(--border)", color: "var(--text)", minWidth: 200 }}
        />
        <div className="flex gap-1">
          {LEVELS.map((l) => (
            <button
              key={l}
              onClick={() => setFilterLevel(l)}
              className="px-3 py-1.5 rounded-full text-xs font-medium transition-colors"
              style={{
                backgroundColor: filterLevel === l ? "var(--accent)" : "var(--surface-raised)",
                color: filterLevel === l ? "#fff" : "var(--text-muted)",
                border: `1px solid ${filterLevel === l ? "var(--accent)" : "var(--border)"}`,
              }}
            >
              {l}
            </button>
          ))}
        </div>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-16"><Spinner size="lg" /></div>
      ) : (
        <VirtualLogList lines={filtered} />
      )}
    </div>
  );
}
