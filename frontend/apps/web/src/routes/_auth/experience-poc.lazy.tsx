import { createLazyFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { eveyFetchRaw } from "@evey/api";
import { sanitizeSduiDocument } from "@/features/sdui/schema";
import { SduiRenderer } from "@/features/sdui/SduiRenderer";

export const Route = createLazyFileRoute("/_auth/experience-poc")({
  component: ExperiencePoc,
});

// Real server data (the subset we render). Everything below is fetched live -
// no placeholders. The page builds a pure-DATA SDUI document; SduiRenderer draws
// it natively and the active theme paints it (so the same document is "Open
// Workshop" under that theme and neutral under another, and would feed a React
// Native renderer unchanged).
interface Mount { path: string; totalBytes: string; usedBytes: string }
interface Dashboard {
  activeStreams: number;
  activeTranscodes: number;
  cpuPercent: number;
  memoryPercent: number;
  storage: Mount[];
  totalMediaItems: number;
  totalUsers: number;
}
interface SystemInfo { version: string; ffmpegVersion: string; uptimeSeconds: string; os: string; arch: string }
interface PopularItem { id: string; title: string; type: string; playCount: number }
interface Lib { id: string; name: string; type: string; count: number }
interface Data { dash: Dashboard | null; sys: SystemInfo | null; popular: PopularItem[]; libs: Lib[] }

function shortType(t: string): string {
  return (t || "media").replace("MEDIA_ITEM_TYPE_", "").replace("LIBRARY_TYPE_", "").toLowerCase();
}
function mount(path: string): string {
  const p = path.replace(/\/+$/, "");
  return p.slice(p.lastIndexOf("/") + 1) || p || "/";
}
function human(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0";
  const tb = bytes / 1e12;
  if (tb >= 1) return `${tb.toFixed(1)} TB`;
  const gb = bytes / 1e9;
  return `${gb.toFixed(gb >= 10 ? 0 : 1)} GB`;
}
function fmtUptime(sec: number): string {
  if (!Number.isFinite(sec) || sec <= 0) return "-";
  const d = Math.floor(sec / 86400);
  const h = Math.floor((sec % 86400) / 3600);
  const m = Math.floor((sec % 3600) / 60);
  return d > 0
    ? `${d}d ${String(h).padStart(2, "0")}:${String(m).padStart(2, "0")}`
    : `${h}h ${String(m).padStart(2, "0")}m`;
}
function pct(used: number, total: number): number {
  return total > 0 ? Math.round((used / total) * 100) : 0;
}

// Best-effort fetch: returns null on any error or non-2xx so one missing/forbidden
// endpoint (e.g. an admin-only stat viewed by a non-admin) never blanks the page.
async function tryFetch(url: string): Promise<Record<string, unknown> | null> {
  try {
    const res = await eveyFetchRaw({ url, method: "GET" });
    if (!res.ok) return null;
    return (await res.json()) as Record<string, unknown>;
  } catch {
    return null;
  }
}

function buildDoc(d: Data) {
  const nodes: unknown[] = [];

  // col 1 - system vitals
  if (d.dash || d.sys) {
    const children: unknown[] = [];
    if (d.dash) {
      children.push({
        type: "statBar",
        items: [
          { label: "cpu", value: Math.round(d.dash.cpuPercent), max: 100 },
          { label: "mem", value: Math.round(d.dash.memoryPercent), max: 100 },
        ],
      });
    }
    const pairs: unknown[] = [];
    if (d.sys) {
      pairs.push({ k: "version", v: d.sys.version, tone: "accent" });
      pairs.push({ k: "ffmpeg", v: d.sys.ffmpegVersion, tone: "default" });
      pairs.push({ k: "uptime", v: fmtUptime(Number(d.sys.uptimeSeconds)), tone: "muted" });
      pairs.push({ k: "host", v: `${d.sys.os}/${d.sys.arch}`, tone: "muted" });
    }
    if (d.dash) {
      pairs.push({ k: "streams", v: String(d.dash.activeStreams), tone: d.dash.activeStreams > 0 ? "ok" : "muted" });
      pairs.push({ k: "transcodes", v: String(d.dash.activeTranscodes), tone: d.dash.activeTranscodes > 0 ? "warn" : "muted" });
      pairs.push({ k: "users", v: String(d.dash.totalUsers), tone: "default" });
      pairs.push({ k: "items", v: String(d.dash.totalMediaItems), tone: "default" });
    }
    if (pairs.length) children.push({ type: "kv", pairs });
    nodes.push({ type: "panel", title: "system", subtitle: "// live", children });
  }

  // col 2 - most played (real popular media + play counts)
  if (d.popular.length) {
    nodes.push({
      type: "panel",
      title: "most played",
      subtitle: `// top ${d.popular.length}`,
      children: [
        {
          type: "table",
          columns: ["#", "type", "title", "plays"],
          rows: d.popular.map((m, i) => ({
            cells: [String(i + 1).padStart(2, "0"), shortType(m.type), m.title, String(m.playCount)],
            tone: "accent",
            to: m.id ? `/media/${m.id}` : "",
          })),
        },
      ],
    });
  }

  // col 3 - libraries (real per-library counts), each a clickable link
  if (d.libs.length) {
    nodes.push({
      type: "panel",
      title: "libraries",
      subtitle: `// ${d.libs.length}`,
      children: d.libs.map((l) => ({
        type: "link",
        label: `${shortType(l.type) || l.name} · ${l.count}`,
        to: `/library/${l.id}`,
        tone: "default",
      })),
    });
  }

  // col 1 (row 2) - storage (real disk usage per mount)
  if (d.dash?.storage?.length) {
    nodes.push({
      type: "panel",
      title: "storage",
      subtitle: `// ${d.dash.storage.length} mounts`,
      children: [
        {
          type: "statBar",
          items: d.dash.storage.map((s) => ({
            label: mount(s.path),
            value: pct(Number(s.usedBytes), Number(s.totalBytes)),
            max: 100,
          })),
        },
        {
          type: "kv",
          pairs: d.dash.storage.map((s) => ({
            k: mount(s.path),
            v: `${human(Number(s.usedBytes))} / ${human(Number(s.totalBytes))}`,
            tone: pct(Number(s.usedBytes), Number(s.totalBytes)) >= 90 ? "warn" : "default",
          })),
        },
      ],
    });
  }

  // col 2 (row 2) - now playing (real session state)
  if (d.dash) {
    const live = d.dash.activeStreams > 0;
    nodes.push({
      type: "panel",
      title: "now playing",
      subtitle: "// realtime",
      children: [
        {
          type: "text",
          value: live
            ? `${d.dash.activeStreams} active stream(s) · ${d.dash.activeTranscodes} transcoding`
            : "no active streams - system idle",
          tone: live ? "ok" : "muted",
        },
      ],
    });
  }

  return sanitizeSduiDocument({ layout: "grid", nodes });
}

function ExperiencePoc() {
  const [data, setData] = useState<Data | null>(null);

  useEffect(() => {
    let active = true;
    void (async () => {
      const [dashR, sysR, popR, libsR] = await Promise.all([
        tryFetch("/api/v1/admin/dashboard"),
        tryFetch("/api/v1/admin/system"),
        tryFetch("/api/v1/popular?limit=12"),
        tryFetch("/api/v1/libraries"),
      ]);

      const rawLibs = (libsR?.libraries as Array<Record<string, unknown>> | undefined) ?? [];
      const libs: Lib[] = await Promise.all(
        rawLibs.map(async (lib) => {
          const id = String(lib.id);
          const page = await tryFetch(`/api/v1/media?library_id=${id}&pagination.pageSize=1`);
          const pag = (page?.pagination as Record<string, unknown> | undefined) ?? {};
          return { id, name: String(lib.name ?? ""), type: String(lib.type ?? ""), count: Number(pag.totalItems ?? 0) };
        }),
      );

      const popular: PopularItem[] = ((popR?.items as Array<Record<string, unknown>> | undefined) ?? []).map((m) => ({
        id: String(m.mediaItemId ?? ""),
        title: String(m.title ?? ""),
        type: String(m.type ?? ""),
        playCount: Number(m.playCount ?? 0),
      }));

      if (active) {
        setData({
          dash: (dashR as unknown as Dashboard | null) ?? null,
          sys: (sysR as unknown as SystemInfo | null) ?? null,
          popular,
          libs,
        });
      }
    })();
    return () => {
      active = false;
    };
  }, []);

  if (!data) return <div className="p-6 text-sm text-[var(--text-muted)]">Loading experience…</div>;

  // Native SDUI render - no iframe, no dangerouslySetInnerHTML (see
  // features/sdui/SduiRenderer.test.tsx). The active theme paints it.
  return <SduiRenderer doc={buildDoc(data)} />;
}
