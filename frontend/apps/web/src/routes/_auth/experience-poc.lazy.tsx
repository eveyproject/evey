import { createLazyFileRoute } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import { eveyFetchRaw } from "@evey/api";
import { sanitizeSduiDocument } from "@/features/sdui/schema";
import { SduiRenderer } from "@/features/sdui/SduiRenderer";

export const Route = createLazyFileRoute("/_auth/experience-poc")({
  component: ExperiencePoc,
});

interface Lib { id: string; name: string; type: string; count: number }
interface Item { title: string; type: string; year: number }
interface Payload { libs: Lib[]; items: Item[] }

function shortType(t: string): string {
  return (t || "media").replace("MEDIA_ITEM_TYPE_", "").replace("LIBRARY_TYPE_", "").toLowerCase();
}

async function fetchJSON(url: string): Promise<Record<string, unknown>> {
  const res = await eveyFetchRaw({ url, method: "GET" });
  if (!res.ok) throw new Error(`HTTP ${res.status} on ${url}`);
  return (await res.json()) as Record<string, unknown>;
}

// Build the experience as a pure DATA document (no HTML/JS). The host renders
// it natively via SduiRenderer; the active theme paints it. The same document
// would feed a React Native renderer unchanged → cross-platform by design.
function buildDoc(p: Payload) {
  return sanitizeSduiDocument({
    layout: "grid",
    nodes: [
      {
        type: "panel",
        title: "system",
        subtitle: "// libraries · live",
        children: [
          { type: "statBar", items: p.libs.map((l) => ({ label: shortType(l.type) || l.name, value: l.count, max: 0 })) },
          {
            type: "kv",
            pairs: [
              { k: "transcode", v: "1/4 slots", tone: "ok" },
              { k: "cache", v: "12.4 / 50 GB", tone: "warn" },
              { k: "scanner", v: "idle", tone: "ok" },
              { k: "uptime", v: "3d 04:21", tone: "muted" },
            ],
          },
        ],
      },
      {
        type: "panel",
        title: "library",
        subtitle: `// ${p.items.length} rows`,
        children: [
          {
            type: "table",
            columns: ["#", "type", "title", "year"],
            rows: p.items.map((it, i) => ({
              cells: [String(i + 1).padStart(3, "0"), shortType(it.type), it.title, String(it.year || "----")],
              tone: "accent",
            })),
          },
        ],
      },
      {
        type: "panel",
        title: "activity",
        subtitle: "// recent",
        children: [{ type: "kv", pairs: p.items.slice(0, 9).map((it) => ({ k: "▸", v: it.title, tone: "default" })) }],
      },
    ],
  });
}

function ExperiencePoc() {
  const [payload, setPayload] = useState<Payload | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    let active = true;
    (async () => {
      try {
        const libsData = await fetchJSON("/api/v1/libraries");
        const rawLibs = (libsData.libraries as Array<Record<string, unknown>>) ?? [];
        const libs: Lib[] = [];
        let items: Item[] = [];
        for (const lib of rawLibs) {
          const id = String(lib.id);
          const page = await fetchJSON(`/api/v1/media?library_id=${id}&pagination.pageSize=40`);
          const pag = (page.pagination as Record<string, unknown>) ?? {};
          libs.push({ id, name: String(lib.name), type: String(lib.type ?? ""), count: Number(pag.totalItems ?? 0) });
          const its = ((page.items as Array<Record<string, unknown>>) ?? []).map((it) => ({
            title: String(it.title), type: String(it.type ?? ""), year: Number(it.year ?? 0),
          }));
          items = items.concat(its);
        }
        if (active) setPayload({ libs, items: items.slice(0, 45) });
      } catch (e) {
        if (active) setErr(e instanceof Error ? e.message : String(e));
      }
    })();
    return () => { active = false; };
  }, []);

  if (err) return <div className="p-6 text-sm text-[var(--danger)]">Experience POC error: {err}</div>;
  if (!payload) return <div className="p-6 text-sm text-[var(--text-muted)]">Loading experience…</div>;

  // Native SDUI render — no iframe. Safe because it's data, not code (see
  // features/sdui/SduiRenderer.test.tsx). Cross-platform: same doc → RN renderer.
  return <SduiRenderer doc={buildDoc(payload)} />;
}
