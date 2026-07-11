import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { describe, it, expect } from "vitest";
import { THEME_NAMES, THEME_TITLES, themeCSS } from "@evey/theme";

// Drift guard: the committed public/themes/*.css files are generated from
// themeVars.ts (the single source of truth for both the web CSS and, via
// deriveTokens, the mobile ThemeTokens). This test fails the moment a CSS file
// disagrees with the source - which is exactly the tokens<->CSS drift that used
// to creep in (two different "danger" reds). It compares the parsed var->value
// maps, not the text, so formatting is free to differ but a VALUE can never
// silently diverge.

const themesDir = join(dirname(fileURLToPath(import.meta.url)), "../../public/themes");

function parseVars(css: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of css.split("\n")) {
    const m = line.match(/^\s*(--[a-z0-9-]+):\s*(.+?);\s*$/i);
    if (m) out[m[1]] = m[2].trim();
  }
  return out;
}

describe("theme CSS is generated from themeVars (no drift)", () => {
  for (const name of THEME_NAMES) {
    it(`${name}.css matches themeVars`, () => {
      const committed = parseVars(readFileSync(join(themesDir, `${name}.css`), "utf8"));
      const generated = parseVars(themeCSS(name, THEME_TITLES[name]));
      expect(committed).toEqual(generated);
    });
  }
});
