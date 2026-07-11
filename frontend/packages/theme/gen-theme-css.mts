// Regenerates apps/web/public/themes/*.css from the single source of truth
// packages/theme/src/themeVars.ts. Run from the frontend workspace:
//   npx tsx scripts/gen-theme-css.mts
// The theme-vars.test.ts drift guard fails CI if a committed CSS ever diverges
// from this output, so hand-editing a CSS file is caught immediately.
import { writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";
import { THEME_NAMES, THEME_TITLES, themeCSS } from "./src/themeVars.ts";

const themesDir = join(dirname(fileURLToPath(import.meta.url)), "../../apps/web/public/themes");
for (const name of THEME_NAMES) {
  writeFileSync(join(themesDir, `${name}.css`), themeCSS(name, THEME_TITLES[name]));
  console.log("generated", `${name}.css`);
}
