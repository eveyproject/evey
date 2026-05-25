import { render } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { sanitizeSduiDocument, SDUI_LIMITS } from "./schema";
import { SduiRenderer } from "./SduiRenderer";

function renderDoc(input: unknown) {
  return render(<SduiRenderer doc={sanitizeSduiDocument(input)} />);
}

describe("SDUI — adversarial security", () => {
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
});
