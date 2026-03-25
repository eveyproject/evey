import { useEffect, useRef, useState, useCallback } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useVirtualizer } from "@tanstack/react-virtual";
import { eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { PageHeader } from "@/components/ui/PageHeader";
import { Checkbox } from "@/components/ui/Checkbox";
import { FilterChip } from "@/components/ui/FilterChip";

export const Route = createLazyFileRoute("/_auth/admin/logs")({
  component: LogsPage,
});

const LEVELS = ["all", "debug", "info", "warn", "error"] as const;
type Level = (typeof LEVELS)[number];

const MAX_LINES = 500;

interface ParsedLine {
  raw: string;
  level?: string;
  time?: string;
  msg?: string;
  component?: string;
}

function parseLine(raw: string): ParsedLine {
  try {
    const obj = JSON.parse(raw) as Record<string, string>;
    return {
      raw,
      level: obj.level,
      time: obj.time,
      msg: obj.msg ?? raw,
      component: obj.component,
    };
  } catch {
    return { raw, msg: raw };
  }
}

function levelColor(level?: string): string {
  switch ((level ?? "").toLowerCase()) {
    case "error": return "var(--color-danger)";
    case "warn": return "var(--color-warning)";
    case "info": return "var(--accent)";
    case "debug": return "var(--text-secondary)";
    default: return "var(--text-muted)";
  }
}

function levelBg(level?: string): string {
  switch ((level ?? "").toLowerCase()) {
    case "error": return "color-mix(in srgb, var(--color-danger) 12%, transparent)";
    case "warn": return "color-mix(in srgb, var(--color-warning) 8%, transparent)";
    default: return "transparent";
  }
}

const LOG_LINE_HEIGHT = 22;

function VirtualLogList({ lines, live, noMatchText }: { lines: ParsedLine[]; live: boolean; noMatchText: string }) {
  const parentRef = useRef<HTMLDivElement>(null);
  const prevLengthRef = useRef(0);

  const virtualizer = useVirtualizer({
    count: lines.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => LOG_LINE_HEIGHT,
    overscan: 20,
  });

  useEffect(() => {
    if (live && lines.length > prevLengthRef.current && lines.length > 0) {
      virtualizer.scrollToIndex(lines.length - 1, { behavior: "smooth" });
    }
    prevLengthRef.current = lines.length;
  // eslint-disable-next-line react-hooks/exhaustive-deps -- virtualizer is recreated every render; using primitive lines.length instead
  }, [lines.length, live]);

  return (
    <div
      ref={parentRef}
      className="flex-1 overflow-y-auto rounded-[var(--radius-lg)] p-3 font-mono text-xs bg-[var(--surface)] border border-[var(--border)] min-h-[400px] max-h-[60vh]"
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
              className="flex gap-2 items-center px-1 rounded hover:bg-[var(--log-hover-bg)] transition-colors-fast"
            >
              {line.time && (
                <span className="flex-shrink-0 text-[11px] text-[var(--text-muted)]">
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
              {line.component && (
                <span className="flex-shrink-0 text-[10px] px-1 rounded text-[var(--text-secondary)] bg-[var(--surface-raised)]">
                  {line.component}
                </span>
              )}
              <span className="truncate text-[var(--text)]">
                {line.msg ?? line.raw}
              </span>
            </div>
          );
        })}
      </div>
      {lines.length === 0 && (
        <p className="text-[var(--text-muted)]">{noMatchText}</p>
      )}
    </div>
  );
}

function LogsPage() {
  const { t } = useTranslation();
  const accessToken = useAuthStore((s) => s.accessToken);
  const [filterLevel, setFilterLevel] = useState<Level>("all");
  const [search, setSearch] = useState("");
  const [componentFilter, setComponentFilter] = useState("");
  const [live, setLive] = useState(true);
  const [lines, setLines] = useState<ParsedLine[]>([]);
  const [loading, setLoading] = useState(true);
  const esRef = useRef<EventSource | null>(null);

  const addLines = useCallback((newLines: ParsedLine[]) => {
    setLines((prev) => {
      const combined = [...prev, ...newLines];
      return combined.length > MAX_LINES ? combined.slice(combined.length - MAX_LINES) : combined;
    });
  }, []);

  useEffect(() => {
    setLoading(true);
    eveyFetch<{ lines: string[] }>({ url: "/api/v1/admin/logs?lines=200", method: "GET" })
      .then((data) => {
        const parsed = (data.lines ?? []).map(parseLine);
        setLines(parsed);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    if (!live) {
      esRef.current?.close();
      esRef.current = null;
      return;
    }

    const tok = accessToken ?? "";
    const url = `/api/v1/admin/logs/stream?lines=0&token=${encodeURIComponent(tok)}`;
    const es = new EventSource(url);
    esRef.current = es;

    es.onmessage = (e) => {
      if (!e.data || e.data.trim() === "") return;
      addLines([parseLine(e.data)]);
    };

    return () => {
      es.close();
      esRef.current = null;
    };
  }, [live, accessToken, addLines]);

  const filtered = lines.filter((l) => {
    if (filterLevel !== "all" && (l.level ?? "").toLowerCase() !== filterLevel) return false;
    if (componentFilter && !(l.component ?? "").toLowerCase().includes(componentFilter.toLowerCase())) return false;
    if (search && !l.raw.toLowerCase().includes(search.toLowerCase())) return false;
    return true;
  });

  return (
    <div className="flex flex-col gap-4 h-full">
      <PageHeader title={t("admin.logs.title")} actions={
        <div className="flex items-center gap-3">
          <Checkbox
            label={t("admin.logs.live")}
            checked={live}
            onChange={setLive}
          />
          <button
            onClick={() => setLines([])}
            className="text-sm px-3 py-1 rounded transition-colors-fast hover:opacity-80 bg-[var(--surface-raised)] text-[var(--text-muted)] border border-[var(--border)]"
          >
            {t("common.clear")}
          </button>
        </div>
      } />

      <div className="flex gap-2 flex-wrap">
        <input
          placeholder={t("admin.logs.searchLogs")}
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="px-3 py-1.5 text-sm rounded-[var(--radius-md)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] min-w-[180px]"
        />
        <input
          placeholder={t("admin.logs.component")}
          value={componentFilter}
          onChange={(e) => setComponentFilter(e.target.value)}
          className="px-3 py-1.5 text-sm rounded-[var(--radius-md)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] min-w-[130px]"
        />
        <div className="flex gap-1">
          {LEVELS.map((l) => (
            <FilterChip
              key={l}
              active={filterLevel === l}
              onClick={() => setFilterLevel(l)}
            >
              {l}
            </FilterChip>
          ))}
        </div>
      </div>

      {loading ? (
        <div className="flex justify-center py-16"><Spinner size="lg" /></div>
      ) : (
        <VirtualLogList lines={filtered} live={live} noMatchText={t("admin.logs.noMatch")} />
      )}
    </div>
  );
}
