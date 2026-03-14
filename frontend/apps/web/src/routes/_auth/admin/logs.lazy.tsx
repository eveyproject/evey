import { useEffect, useRef, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
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
    case "debug": return "#94a3b8";
    default: return "var(--text-muted)";
  }
}

function LogsPage() {
  const [lines, setLines] = useState(200);
  const [filterLevel, setFilterLevel] = useState<Level>("all");
  const [search, setSearch] = useState("");
  const [autoRefresh, setAutoRefresh] = useState(true);
  const bottomRef = useRef<HTMLDivElement>(null);

  const { data, isLoading, dataUpdatedAt } = useQuery<{ lines: string[] }>({
    queryKey: ["/api/v1/admin/logs", lines],
    queryFn: () => eveyFetch<{ lines: string[] }>({ url: `/api/v1/admin/logs?lines=${lines}`, method: "GET" }),
    refetchInterval: autoRefresh ? 5_000 : false,
  });

  const rawLines = data?.lines ?? [];
  const parsed = rawLines.map(parseLine);
  const filtered = parsed.filter((l) => {
    if (filterLevel !== "all" && l.level?.toLowerCase() !== filterLevel) return false;
    if (search && !l.raw.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  useEffect(() => {
    if (autoRefresh) bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [dataUpdatedAt, autoRefresh]);

  return (
    <div className="flex flex-col gap-4 h-full">
      <div className="flex items-center gap-3 flex-wrap">
        <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>Live Logs</h1>
        <div className="flex-1" />
        {dataUpdatedAt > 0 && (
          <span className="text-xs" style={{ color: "var(--text-muted)" }}>
            Updated {new Date(dataUpdatedAt).toLocaleTimeString()}
          </span>
        )}
        <label className="flex items-center gap-1.5 text-sm cursor-pointer" style={{ color: "var(--text)" }}>
          <input
            type="checkbox"
            checked={autoRefresh}
            onChange={(e) => setAutoRefresh(e.target.checked)}
            className="accent-[var(--accent)]"
          />
          Auto-refresh
        </label>
        <select
          value={lines}
          onChange={(e) => setLines(Number(e.target.value))}
          className="text-sm px-2 py-1 rounded"
          style={{ backgroundColor: "var(--surface-raised)", color: "var(--text)", border: "1px solid var(--border)" }}
        >
          {[50, 100, 200, 500, 1000].map((n) => (
            <option key={n} value={n}>{n} lines</option>
          ))}
        </select>
      </div>

      <div className="flex gap-2 flex-wrap">
        <input
          placeholder="Filter logs…"
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
        <div
          className="flex-1 overflow-y-auto rounded-[var(--radius-lg)] p-3 font-mono text-xs"
          style={{ backgroundColor: "#0d1117", border: "1px solid var(--border)", minHeight: 400, maxHeight: "60vh" }}
        >
          {filtered.length === 0 ? (
            <p style={{ color: "var(--text-muted)" }}>No log entries match your filter.</p>
          ) : (
            filtered.map((line, i) => (
              <div key={i} className="flex gap-2 mb-0.5 leading-relaxed hover:bg-white/5 px-1 rounded">
                {line.time && (
                  <span className="flex-shrink-0" style={{ color: "#6e7681" }}>
                    {line.time.substring(11, 19)}
                  </span>
                )}
                {line.level && (
                  <span className="flex-shrink-0 uppercase font-bold w-5" style={{ color: levelColor(line.level) }}>
                    {line.level.substring(0, 1)}
                  </span>
                )}
                <span style={{ color: "#e6edf3", wordBreak: "break-all" }}>
                  {line.msg ?? line.raw}
                </span>
              </div>
            ))
          )}
          <div ref={bottomRef} />
        </div>
      )}
    </div>
  );
}
