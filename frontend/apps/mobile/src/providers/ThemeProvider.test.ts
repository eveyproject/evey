import { describe, it, expect } from "vitest";
import { defaultTheme } from "@evey/theme";
import { mergeCSSVars } from "./ThemeProvider";

describe("mergeCSSVars - plugin theme overrides", () => {
  // The base theme's success/warning are bright enough that onColor() picks
  // near-black. A plugin theme with DARK fills must flip them to white - if
  // the merge kept the base value it would paint #0a0a0a on #0b3d0b.
  it("recomputes onSuccess/onWarning from the plugin's own fill", () => {
    expect(defaultTheme.colors.onSuccess).toBe("#0a0a0a");
    expect(defaultTheme.colors.onWarning).toBe("#0a0a0a");

    const merged = mergeCSSVars(defaultTheme, {
      "--success": "#0b3d0b",
      "--warning": "#4a2c00",
    });
    expect(merged.colors.success).toBe("#0b3d0b");
    expect(merged.colors.warning).toBe("#4a2c00");
    // Canonical 6-digit form: onColor emits a public token, and the 3-digit
    // one silently turned a contrast assertion into NaN.
    expect(merged.colors.onSuccess).toBe("#ffffff");
    expect(merged.colors.onWarning).toBe("#ffffff");
  });

  it("keeps the base on-colours when the plugin overrides nothing", () => {
    const merged = mergeCSSVars(defaultTheme, {});
    expect(merged.colors.onSuccess).toBe(defaultTheme.colors.onSuccess);
    expect(merged.colors.onWarning).toBe(defaultTheme.colors.onWarning);
    expect(merged.colors.onDanger).toBe(defaultTheme.colors.onDanger);
  });

  it("honours an explicit --on-danger from the plugin", () => {
    const merged = mergeCSSVars(defaultTheme, { "--on-danger": "#123456" });
    expect(merged.colors.onDanger).toBe("#123456");
  });
});
