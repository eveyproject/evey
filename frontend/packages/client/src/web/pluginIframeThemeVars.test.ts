import { describe, it, expect, afterEach } from "vitest";
import { collectThemeVars } from "./PluginIframe";

describe("collectThemeVars", () => {
  afterEach(() => {
    document.documentElement.removeAttribute("style");
    document.head.querySelectorAll("style[data-test]").forEach((el) => el.remove());
  });

  // The browser television paints a theme by setting each property on the
  // root element: a plugin frame given only the stylesheet's names drew
  // itself in its fallback colours, whatever the theme.
  it("hands the frame the properties a theme set on the root element", () => {
    document.documentElement.style.setProperty("--bg", "#0a0a0a");
    document.documentElement.style.setProperty("--accent", "#ffb84d");
    const css = collectThemeVars();
    expect(css).toContain("--bg: #0a0a0a;");
    expect(css).toContain("--accent: #ffb84d;");
  });

  it("still hands over the properties declared by a :root rule, once each", () => {
    const style = document.createElement("style");
    style.setAttribute("data-test", "");
    style.textContent = ":root { --text: #f1f5f9; --bg: #0c1220; }";
    document.head.appendChild(style);
    document.documentElement.style.setProperty("--bg", "#0a0a0a");
    const css = collectThemeVars();
    expect(css).toContain("--text: #f1f5f9;");
    expect(css.match(/--bg:/g)).toHaveLength(1);
    expect(css).toContain("--bg: #0a0a0a;");
  });
});
