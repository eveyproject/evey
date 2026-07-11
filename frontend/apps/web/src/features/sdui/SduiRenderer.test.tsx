import { render, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { sanitizeSduiDocument, SDUI_LIMITS } from "./schema";
import { SduiRenderer } from "./SduiRenderer";

// SduiRenderer calls useNavigate(); mock it so the renderer works without a
// Router and we can assert what navigation a click triggers.
const { navigateMock } = vi.hoisted(() => ({ navigateMock: vi.fn() }));
vi.mock("@tanstack/react-router", () => ({ useNavigate: () => navigateMock }));

beforeEach(() => navigateMock.mockClear());

function renderDoc(input: unknown) {
  return render(<SduiRenderer doc={sanitizeSduiDocument(input)} />);
}

describe("SDUI - adversarial security", () => {
  it("renders injected HTML/JS as inert text, never as markup (XSS)", () => {
    const { container } = renderDoc({
      nodes: [
        { type: "text", value: "<script>alert(1)</script>" },
        { type: "table", columns: ["<b>col</b>"], rows: [{ cells: ['<img src=x onerror="alert(1)">', "y"] }] },
        { type: "panel", title: "<svg onload=alert(1)>", children: [{ type: "text", value: "<iframe src=javascript:alert(1)>" }] },
      ],
    });
    // No live markup was created from plugin strings.
    expect(container.querySelector("script")).toBeNull();
    expect(container.querySelector("img")).toBeNull();
    expect(container.querySelector("svg")).toBeNull();
    expect(container.querySelector("iframe")).toBeNull();
    expect(container.querySelector("[onerror]")).toBeNull();
    expect(container.querySelector("[onload]")).toBeNull();
    // The payloads survive only as escaped text.
    expect(container.textContent).toContain("<script>alert(1)</script>");
    expect(container.textContent).toContain('<img src=x onerror="alert(1)">');
  });

  it("drops unknown / forged node types", () => {
    const doc = sanitizeSduiDocument({
      nodes: [{ type: "evil", html: "<x>" }, { type: "iframe", src: "//evil" }, { type: "script" }, { type: "text", value: "ok" }],
    });
    expect(doc.nodes).toHaveLength(1);
    expect(doc.nodes[0].type).toBe("text");
  });

  it("coerces an invalid tone to a fixed enum value (no CSS injection)", () => {
    const doc = sanitizeSduiDocument({ nodes: [{ type: "text", value: "x", tone: "url(javascript:alert(1))" }] });
    const n = doc.nodes[0] as { type: "text"; tone: string };
    expect(n.tone).toBe("default");
  });

  it("caps nesting depth", () => {
    let node: unknown = { type: "text", value: "deep" };
    for (let i = 0; i < 80; i++) node = { type: "panel", children: [node] };
    const doc = sanitizeSduiDocument({ nodes: [node] });
    let depth = 0;
    let cur = doc.nodes[0] as { type: string; children?: unknown[] };
    while (cur && cur.type === "panel") {
      depth++;
      cur = (cur.children?.[0] as typeof cur) ?? ({} as typeof cur);
    }
    expect(depth).toBeLessThanOrEqual(SDUI_LIMITS.maxDepth + 1);
  });

  it("caps node count and string length (anti-DoS)", () => {
    const many = Array.from({ length: 5000 }, () => ({ type: "text", value: "x" }));
    const doc = sanitizeSduiDocument({ nodes: many });
    expect(doc.nodes.length).toBeLessThanOrEqual(SDUI_LIMITS.maxNodes);

    const long = sanitizeSduiDocument({ nodes: [{ type: "text", value: "a".repeat(100000) }] });
    const t = long.nodes[0] as { value: string };
    expect(t.value.length).toBeLessThanOrEqual(SDUI_LIMITS.maxString);
  });

  it("never throws on garbage input", () => {
    const garbage: unknown[] = [null, undefined, 42, "str", [], true, { nodes: "nope" }, { nodes: [null, 1, "x", {}] }];
    for (const g of garbage) expect(() => render(<SduiRenderer doc={sanitizeSduiDocument(g)} />)).not.toThrow();
  });

  it("ignores prototype-pollution payloads", () => {
    const doc = sanitizeSduiDocument(
      JSON.parse('{"nodes":[{"type":"text","value":"x","__proto__":{"polluted":true}}],"__proto__":{"x":1}}'),
    );
    expect(({} as Record<string, unknown>).polluted).toBeUndefined();
    expect(doc.nodes).toHaveLength(1);
  });

  it("strips unsafe navigation targets (external, javascript:, protocol-relative, traversal)", () => {
    const doc = sanitizeSduiDocument({
      nodes: [
        { type: "link", label: "ext", to: "https://evil.com", tone: "default" },
        { type: "link", label: "js", to: "javascript:alert(1)", tone: "default" },
        { type: "link", label: "proto", to: "//evil.com/x", tone: "default" },
        { type: "link", label: "trav", to: "/../../etc/passwd", tone: "default" },
        { type: "link", label: "ok", to: "/media/4a6f-123", tone: "default" },
        {
          type: "table",
          columns: ["t"],
          rows: [
            { cells: ["evil"], to: "javascript:alert(1)" },
            { cells: ["ok"], to: "/library/lib1" },
          ],
        },
      ],
    });
    const links = doc.nodes.filter((n) => n.type === "link") as Array<{ label: string; to: string }>;
    const to = (label: string) => links.find((l) => l.label === label)!.to;
    expect(to("ext")).toBe("");
    expect(to("js")).toBe("");
    expect(to("proto")).toBe("");
    expect(to("trav")).toBe("");
    expect(to("ok")).toBe("/media/4a6f-123");
    const table = doc.nodes.find((n) => n.type === "table") as { rows: Array<{ to: string }> };
    expect(table.rows[0].to).toBe(""); // javascript: stripped
    expect(table.rows[1].to).toBe("/library/lib1");
  });

  it("navigates internally on click of a safe link, never for a dropped target", () => {
    const { getByText } = renderDoc({
      nodes: [
        { type: "link", label: "go", to: "/media/abc-123", tone: "default" },
        { type: "link", label: "nope", to: "javascript:alert(1)", tone: "default" },
      ],
    });
    fireEvent.click(getByText("go"));
    expect(navigateMock).toHaveBeenCalledWith({ to: "/media/abc-123" });

    navigateMock.mockClear();
    fireEvent.click(getByText("nope")); // sanitized to "" → rendered as inert <div>
    expect(navigateMock).not.toHaveBeenCalled();
  });
});
