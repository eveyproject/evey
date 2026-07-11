// Server-Driven UI schema + sanitizer.
//
// An extension describes UI as DATA (this schema), never as HTML/JS/CSS. The
// host renders it with its own native components (web here; RN renderers would
// consume the exact same document). Security comes from four invariants,
// enforced at the trust boundary `sanitizeSduiDocument`:
//   1. node `type` is a closed allow-list — an unknown/forged type is dropped;
//   2. all user-visible values are plain strings, rendered as text by React
//      (auto-escaped) — never as markup, never `dangerouslySetInnerHTML`;
//   3. `tone` is an enum mapped to fixed classes — the extension cannot inject
//      arbitrary colours/CSS; and sizes/depth are capped (anti-DoS);
//   4. navigation targets (`to`) are clamped by `safePath` to internal app
//      routes only — no scheme, no protocol-relative, no traversal (so a click
//      can open an app route, never an external site and never code).
// The sanitizer never throws: untrusted input is coerced/dropped into a safe
// document.

export type SduiTone = "default" | "accent" | "ok" | "warn" | "muted";
const TONES: readonly SduiTone[] = ["default", "accent", "ok", "warn", "muted"];

interface SduiStat { label: string; value: number; max: number }
interface SduiRow { cells: string[]; tone: SduiTone; to: string }
interface SduiPair { k: string; v: string; tone: SduiTone }

export type SduiNode =
  | { type: "panel"; title: string; subtitle: string; children: SduiNode[] }
  | { type: "statBar"; items: SduiStat[] }
  | { type: "table"; columns: string[]; rows: SduiRow[] }
  | { type: "kv"; pairs: SduiPair[] }
  | { type: "text"; value: string; tone: SduiTone }
  | { type: "link"; label: string; to: string; tone: SduiTone };

export interface SduiDocument {
  version: 1;
  layout: "grid" | "stack";
  nodes: SduiNode[];
}

export const SDUI_LIMITS = {
  maxNodes: 250, // total nodes across the tree
  maxDepth: 5, // panel nesting
  maxString: 500, // chars per string (truncated)
  maxArray: 500, // items/rows/cells/pairs per collection
} as const;

// ---- coercion helpers (never throw) ----
function str(v: unknown): string {
  if (typeof v === "string") return v.length > SDUI_LIMITS.maxString ? v.slice(0, SDUI_LIMITS.maxString) : v;
  if (typeof v === "number" && Number.isFinite(v)) return String(v);
  return "";
}
function num(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}
function tone(v: unknown): SduiTone {
  return typeof v === "string" && (TONES as readonly string[]).includes(v) ? (v as SduiTone) : "default";
}
function arr(v: unknown): unknown[] {
  return Array.isArray(v) ? v.slice(0, SDUI_LIMITS.maxArray) : [];
}

// safePath is the trust boundary for SDUI navigation. A node may carry a `to`
// the renderer will follow, but ONLY an internal absolute app path: one leading
// slash, not protocol-relative (`//`), no scheme (`javascript:`/`http:` — the
// `:` is outside the allowed charset), no `..` traversal, restricted charset.
// Anything else collapses to "" → the renderer treats the node as non-navigable.
function safePath(v: unknown): string {
  if (typeof v !== "string" || v.length === 0 || v.length > SDUI_LIMITS.maxString) return "";
  if (v[0] !== "/" || v[1] === "/") return "";
  if (v.includes("..") || v.includes("\\")) return "";
  return /^\/[A-Za-z0-9/._~-]*$/.test(v) ? v : "";
}

interface Budget { nodes: number }

function sanitizeNode(input: unknown, depth: number, budget: Budget): SduiNode | null {
  if (budget.nodes >= SDUI_LIMITS.maxNodes) return null;
  if (!input || typeof input !== "object") return null;
  const n = input as Record<string, unknown>;
  budget.nodes++;
  switch (n.type) {
    case "panel": {
      const children: SduiNode[] = [];
      if (depth < SDUI_LIMITS.maxDepth) {
        for (const c of arr(n.children)) {
          const sc = sanitizeNode(c, depth + 1, budget);
          if (sc) children.push(sc);
        }
      }
      return { type: "panel", title: str(n.title), subtitle: str(n.subtitle), children };
    }
    case "statBar":
      return {
        type: "statBar",
        items: arr(n.items).map((it) => {
          const o = (it && typeof it === "object" ? it : {}) as Record<string, unknown>;
          return { label: str(o.label), value: num(o.value), max: Math.max(1, num(o.max)) };
        }),
      };
    case "table":
      return {
        type: "table",
        columns: arr(n.columns).map(str),
        rows: arr(n.rows).map((r) => {
          const o = (r && typeof r === "object" ? r : {}) as Record<string, unknown>;
          return { cells: arr(o.cells).map(str), tone: tone(o.tone), to: safePath(o.to) };
        }),
      };
    case "kv":
      return {
        type: "kv",
        pairs: arr(n.pairs).map((p) => {
          const o = (p && typeof p === "object" ? p : {}) as Record<string, unknown>;
          return { k: str(o.k), v: str(o.v), tone: tone(o.tone) };
        }),
      };
    case "text":
      return { type: "text", value: str(n.value), tone: tone(n.tone) };
    case "link":
      return { type: "link", label: str(n.label), to: safePath(n.to), tone: tone(n.tone) };
    default:
      // Unknown/forged type → dropped (and its node budget reclaimed).
      budget.nodes--;
      return null;
  }
}

/**
 * The trust boundary. Takes arbitrary untrusted input (a plugin-provided value)
 * and returns a guaranteed-safe SduiDocument. Never throws.
 */
export function sanitizeSduiDocument(input: unknown): SduiDocument {
  const root = (input && typeof input === "object" ? input : {}) as Record<string, unknown>;
  const budget: Budget = { nodes: 0 };
  const nodes: SduiNode[] = [];
  for (const node of arr(root.nodes)) {
    const sn = sanitizeNode(node, 0, budget);
    if (sn) nodes.push(sn);
  }
  return { version: 1, layout: root.layout === "stack" ? "stack" : "grid", nodes };
}

export const toneClass: Record<SduiTone, string> = {
  default: "text-[var(--text)]",
  accent: "text-[var(--accent)]",
  ok: "text-[var(--color-success)]",
  warn: "text-[var(--color-warning)]",
  muted: "text-[var(--text-muted)]",
};
