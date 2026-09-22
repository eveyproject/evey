import { describe, it, expect } from "vitest";
import { themes, defaultTheme } from "@evey/theme";
import { resolveThemeTokens, mergeCSSVars, pluginThemeKey, isPluginThemeName } from "./pluginTheme";

// The same stored choice must paint the same on the phone and the television:
// a built-in by name, a plugin's sheet merged over the default, and the
// default while that sheet is still on its way.
describe("resolveThemeTokens", () => {
  it("returns the built-in for its name and the default for an unknown one", () => {
    expect(resolveThemeTokens("amoled", null, undefined)).toBe(themes.amoled);
    expect(resolveThemeTokens("dark-red", null, [])).toBe(themes["dark-red"]);
    expect(resolveThemeTokens("no-such-theme", null, [])).toBe(defaultTheme);
  });

  it("merges a plugin's sheet over the default once it is known", () => {
    const tokens = resolveThemeTokens(pluginThemeKey("Ocean"), "ocean", [
      { id: "ocean", name: "Ocean", tokens: { "--accent": "#00aaff", "--bg": "#001122" } },
    ]);
    expect(tokens.colors.accent).toBe("#00aaff");
    expect(tokens.colors.bg).toBe("#001122");
    expect(tokens.colors.text).toBe(defaultTheme.colors.text);
  });

  it("holds the default while the plugin's sheet has not arrived, and for a sheet-less plugin", () => {
    expect(resolveThemeTokens(pluginThemeKey("Ocean"), "ocean", undefined)).toBe(defaultTheme);
    expect(resolveThemeTokens(pluginThemeKey("Bare"), "bare", [{ id: "bare", name: "Bare" }])).toBe(defaultTheme);
  });

  it("recomputes the on-fill colours from the plugin's own fills", () => {
    const light = mergeCSSVars(defaultTheme, { "--success": "#ffffff", "--warning": "#000000" });
    expect(light.colors.onSuccess).not.toBe(light.colors.onWarning);
    expect(isPluginThemeName("plugin:x")).toBe(true);
    expect(isPluginThemeName("default")).toBe(false);
  });
});
