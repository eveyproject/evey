import type { SduiDocument, SduiNode } from "./schema";
import { toneClass } from "./schema";

// Renders a (sanitized) SduiDocument with native Evey components. The plugin
// describes STRUCTURE; the active theme paints it (so the same document looks
// "Open Workshop" under that theme, neutral under another). All values go
// through {text} → React escapes them. No dangerouslySetInnerHTML anywhere.

function asciiBar(value: number, max: number, width = 16): string {
  const filled = Math.max(0, Math.min(width, Math.round((value / max) * width)));
  return "█".repeat(filled) + "░".repeat(width - filled);
}

function SduiNodeView({ node }: { node: SduiNode }) {
  switch (node.type) {
    case "panel":
      return (
        <section className="bg-[var(--surface)] border border-[var(--border)] rounded-[var(--radius-md)] overflow-hidden">
          {(node.title || node.subtitle) && (
            <div className="px-3 py-2 border-b border-[var(--border)] text-xs uppercase tracking-widest text-[var(--accent)]">
              {node.title}
              {node.subtitle && <span className="text-[var(--text-muted)] ml-2 normal-case tracking-normal">{node.subtitle}</span>}
            </div>
          )}
          <div className="flex flex-col">
            {node.children.map((c, i) => (
              <SduiNodeView key={i} node={c} />
            ))}
          </div>
        </section>
      );
    case "statBar":
      return (
        <div className="flex flex-col">
          {node.items.map((s, i) => (
            <div key={i} className="flex items-center gap-2 px-3 py-1.5 text-xs">
              <span className="w-16 uppercase tracking-wide text-[var(--text-muted)] truncate">{s.label}</span>
              <span className="flex-1 text-[var(--accent)] tracking-tighter">{asciiBar(s.value, s.max)}</span>
              <span className="w-12 text-right text-[var(--text)]">{s.value}</span>
            </div>
          ))}
        </div>
      );
    case "table":
      return (
        <div className="flex flex-col">
          <div className="flex gap-3 px-3 py-1.5 text-[10px] uppercase tracking-wide text-[var(--text-muted)] border-b border-[var(--border)]">
            {node.columns.map((c, i) => (
              <span key={i} className={i === node.columns.length - 1 ? "ml-auto" : ""}>{c}</span>
            ))}
          </div>
          {node.rows.map((r, i) => (
            <div key={i} className="flex gap-3 px-3 py-1 text-xs border-b border-[var(--border)] last:border-0 hover:bg-[var(--surface-raised)]">
              {r.cells.map((cell, j) => (
                <span key={j} className={`${j === 0 ? "text-[var(--text-muted)] w-8" : j === r.cells.length - 1 ? "ml-auto text-[var(--text-muted)]" : "flex-1 truncate"} ${j === 1 ? toneClass[r.tone] : ""}`}>{cell}</span>
              ))}
            </div>
          ))}
        </div>
      );
    case "kv":
      return (
        <div className="flex flex-col">
          {node.pairs.map((p, i) => (
            <div key={i} className="flex justify-between px-3 py-1.5 text-xs border-t border-[var(--border)] first:border-0">
              <span className="text-[var(--text-muted)]">{p.k}</span>
              <span className={toneClass[p.tone]}>{p.v}</span>
            </div>
          ))}
        </div>
      );
    case "text":
      return <p className={`px-3 py-2 text-sm ${toneClass[node.tone]}`}>{node.value}</p>;
  }
}

export function SduiRenderer({ doc }: { doc: SduiDocument }) {
  return (
    <div
      className={
        doc.layout === "grid"
          ? "grid grid-cols-1 md:grid-cols-[260px_1fr_240px] gap-3 p-3"
          : "flex flex-col gap-3 p-3"
      }
      data-sdui-root
    >
      {doc.nodes.map((n, i) => (
        <SduiNodeView key={i} node={n} />
      ))}
    </div>
  );
}
